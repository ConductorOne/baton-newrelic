package connector

import (
	"context"
	"fmt"
	"strings"

	"github.com/conductorone/baton-newrelic/pkg/newrelic"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	ent "github.com/conductorone/baton-sdk/pkg/types/entitlement"
	"github.com/conductorone/baton-sdk/pkg/types/grant"
	rs "github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
)

const (
	roleMembership = "member"
)

type roleBuilder struct {
	resourceType *v2.ResourceType
	client       *newrelic.Client
}

func (r *roleBuilder) ResourceType(ctx context.Context) *v2.ResourceType {
	return roleResourceType
}

func roleResource(ctx context.Context, pId *v2.ResourceId, role *newrelic.Role) (*v2.Resource, error) {
	profile := map[string]interface{}{
		"role_scope": role.Scope,
		"role_name":  role.Name,
	}

	resource, err := rs.NewRoleResource(
		role.DisplayName,
		roleResourceType,
		role.ID,
		[]rs.RoleTraitOption{
			rs.WithRoleProfile(profile),
		},
		rs.WithParentResourceID(pId),
	)

	if err != nil {
		return nil, err
	}

	return resource, nil
}

// List returns all the roles from the database as resource objects.
// Roles include a RoleTrait because they are the 'shape' of a standard role.
func (r *roleBuilder) List(ctx context.Context, parentResourceID *v2.ResourceId, pToken *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error) {
	if parentResourceID == nil {
		return nil, "", nil, nil
	}

	// parse the token
	bag, err := parsePageToken(pToken.Token, &v2.ResourceId{ResourceType: roleResourceType.Id})
	if err != nil {
		return nil, "", nil, err
	}

	roles, nextCursor, err := r.client.ListRoles(ctx, bag.PageToken())
	if err != nil {
		return nil, "", nil, err
	}

	// add next cursor to bag
	next, err := bag.NextToken(nextCursor)
	if err != nil {
		return nil, "", nil, err
	}

	var rv []*v2.Resource
	for _, role := range roles {
		roleCopy := role
		rr, err := roleResource(ctx, parentResourceID, &roleCopy)
		if err != nil {
			return nil, "", nil, err
		}

		rv = append(rv, rr)
	}

	return rv, next, nil, nil
}

// Entitlements always returns an empty slice for roles.
func (r *roleBuilder) Entitlements(_ context.Context, resource *v2.Resource, _ *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	var rv []*v2.Entitlement

	rolesTrait, err := rs.GetRoleTrait(resource)
	if err != nil {
		return nil, "", nil, err
	}

	// get role name
	roleName, ok := rs.GetProfileStringValue(rolesTrait.Profile, "role_name")
	if !ok {
		return nil, "", nil, fmt.Errorf("unable to get role name from role trait profile")
	}

	permissionOptions := []ent.EntitlementOption{
		ent.WithGrantableTo(groupResourceType),
		ent.WithDisplayName(fmt.Sprintf("%s Role", resource.DisplayName)),
		ent.WithDescription(fmt.Sprintf("%s access to %s group in DockerHub", roleMembership, resource.DisplayName)),
	}

	rv = append(rv, ent.NewAssignmentEntitlement(resource, roleName, permissionOptions...))

	return rv, "", nil, nil
}

// Grants always returns an empty slice for roles since they don't have any entitlements.
func (r *roleBuilder) Grants(ctx context.Context, resource *v2.Resource, pToken *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
	// parse the token
	bag, err := parsePageToken(pToken.Token, &v2.ResourceId{ResourceType: domainResourceType})
	if err != nil {
		return nil, "", nil, err
	}

	switch bag.ResourceTypeID() {
	case domainResourceType:
		// list and paginate through all domains
		domains, nextDomainsCursor, err := r.client.ListDomains(ctx, bag.PageToken())
		if err != nil {
			return nil, "", nil, err
		}

		// remove old cursors from bag
		bag.Pop()

		// push next cursor for paginating domains
		if nextDomainsCursor != "" {
			bag.Push(
				pagination.PageState{
					ResourceTypeID: domainResourceType,
					Token:          nextDomainsCursor,
				},
			)
		}

		for _, d := range domains {
			if d.Total == 0 {
				continue
			}

			// push cursor for paginating groups under domain
			bag.Push(
				pagination.PageState{
					ResourceTypeID: groupResourceType.Id,
					Token:          fmt.Sprintf("%s:", d.ID),
				},
			)
		}

		next, err := bag.NextToken(bag.PageToken())
		if err != nil {
			return nil, "", nil, err
		}

		return nil, next, nil, nil

	case groupResourceType.Id:
		// list and paginate through groups under specific domain
		parts := strings.Split(bag.PageToken(), ":")
		if len(parts) != 2 {
			return nil, "", nil, fmt.Errorf("invalid page token: %s (type: %s)", bag.PageToken(), bag.ResourceTypeID())
		}

		domainId := parts[0]
		cursor := parts[1]

		// get role trait
		rolesTrait, err := rs.GetRoleTrait(resource)
		if err != nil {
			return nil, "", nil, err
		}

		// get role name for entitlement id
		roleName, ok := rs.GetProfileStringValue(rolesTrait.Profile, "role_name")
		if !ok {
			return nil, "", nil, fmt.Errorf("unable to get role name from role trait profile")
		}

		// list all groups within all domains with specific role
		groups, nextGroupsCursor, err := r.client.ListGroupsWithRole(ctx, domainId, resource.Id.Resource, cursor)
		if err != nil {
			return nil, "", nil, err
		}

		c, err := composeCursor(domainId, nextGroupsCursor)
		if err != nil {
			return nil, "", nil, err
		}

		next, err := bag.NextToken(c)
		if err != nil {
			return nil, "", nil, err
		}

		var rv []*v2.Grant
		for _, g := range groups {
			// skip groups without roles
			if g.Roles.TotalCount == 0 {
				continue
			}

			rv = append(rv, grant.NewGrant(
				resource,
				roleName,
				&v2.ResourceId{
					ResourceType: groupResourceType.Id,
					Resource:     g.ID,
				},
				grant.WithAnnotation(
					&v2.GrantExpandable{
						EntitlementIds: []string{fmt.Sprintf("group:%s:%s", g.ID, groupMembership)},
					},
				),
			))
		}

		return rv, next, nil, nil

	default:
		return nil, "", nil, fmt.Errorf("invalid resource type: %s", bag.ResourceTypeID())
	}
}

func (r *roleBuilder) Grant(ctx context.Context, principal *v2.Resource, entitlement *v2.Entitlement) (annotations.Annotations, error) {
	l := ctxzap.Extract(ctx)

	if principal.Id.ResourceType != groupResourceType.Id {
		l.Warn(
			"newrelic-connector: only groups can be granted role membership",
			zap.String("principal_id", principal.Id.String()),
			zap.String("principal_type", principal.Id.ResourceType),
		)

		return nil, fmt.Errorf("newrelic-connector: only groups can be granted role membership")
	}

	roleId, groupId := entitlement.Resource.Id.Resource, principal.Id.Resource
	err := r.client.AddRoleToGroup(ctx, roleId, groupId)
	if err != nil {
		return nil, fmt.Errorf("newrelic-connector: failed to add role to group: %w", err)
	}

	return nil, nil
}

func (r *roleBuilder) Revoke(ctx context.Context, grant *v2.Grant) (annotations.Annotations, error) {
	l := ctxzap.Extract(ctx)

	principal := grant.Principal
	entitlement := grant.Entitlement

	if principal.Id.ResourceType != groupResourceType.Id {
		l.Warn(
			"newrelic-connector: only groups can have role membership revoked",
			zap.String("principal_id", principal.Id.String()),
			zap.String("principal_type", principal.Id.ResourceType),
		)

		return nil, fmt.Errorf("newrelic-connector: only groups can have role membership revoked")
	}

	roleId, groupId := entitlement.Resource.Id.Resource, principal.Id.Resource
	err := r.client.RemoveRoleFromGroup(ctx, roleId, groupId)
	if err != nil {
		return nil, fmt.Errorf("newrelic-connector: failed to remove role from group: %w", err)
	}

	return nil, nil
}

func newRoleBuilder(client *newrelic.Client) *roleBuilder {
	return &roleBuilder{
		resourceType: roleResourceType,
		client:       client,
	}
}
