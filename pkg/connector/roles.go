package connector

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/conductorone/baton-newrelic/pkg/newrelic"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	ent "github.com/conductorone/baton-sdk/pkg/types/entitlement"
	"github.com/conductorone/baton-sdk/pkg/types/grant"
	rs "github.com/conductorone/baton-sdk/pkg/types/resource"
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
		nil,
		rs.WithParentResourceID(pId),
		rs.WithResourceProfile(profile),
	)

	if err != nil {
		return nil, err
	}

	return resource, nil
}

// List returns all the roles from the database as resource objects.
// Roles include a RoleTrait because they are the 'shape' of a standard role.
func (r *roleBuilder) List(ctx context.Context, parentResourceID *v2.ResourceId, opts rs.SyncOpAttrs) ([]*v2.Resource, *rs.SyncOpResults, error) {
	if parentResourceID == nil {
		return nil, nil, nil
	}

	bag, err := parsePageToken(opts.PageToken.Token, &v2.ResourceId{ResourceType: roleResourceType.Id})
	if err != nil {
		return nil, nil, err
	}

	roles, nextCursor, err := r.client.ListRoles(ctx, bag.PageToken())
	if err != nil {
		return nil, nil, err
	}

	next, err := bag.NextToken(nextCursor)
	if err != nil {
		return nil, nil, err
	}

	var rv []*v2.Resource
	for _, role := range roles {
		roleCopy := role
		rr, err := roleResource(ctx, parentResourceID, &roleCopy)
		if err != nil {
			return nil, nil, err
		}

		rv = append(rv, rr)
	}

	return rv, &rs.SyncOpResults{NextPageToken: next}, nil
}

// Entitlements returns the member entitlement for a role.
func (r *roleBuilder) Entitlements(_ context.Context, resource *v2.Resource, _ rs.SyncOpAttrs) ([]*v2.Entitlement, *rs.SyncOpResults, error) {
	var rv []*v2.Entitlement

	roleScope, ok := rs.GetProfileStringValue(rs.GetProfile(resource), "role_scope")
	if !ok {
		return nil, nil, fmt.Errorf("unable to get role scope from role trait profile")
	}

	roleName, ok := rs.GetProfileStringValue(rs.GetProfile(resource), "role_name")
	if !ok {
		return nil, nil, fmt.Errorf("unable to get role name from role trait profile")
	}

	permissionOptions := []ent.EntitlementOption{
		ent.WithGrantableTo(groupResourceType),
		ent.WithDisplayName(fmt.Sprintf("%s - %s Role", resource.DisplayName, roleScope)),
		ent.WithDescription(fmt.Sprintf("%s access to %s role in NewRelic", roleMembership, resource.DisplayName)),
	}

	rv = append(rv, ent.NewAssignmentEntitlement(resource, roleName, permissionOptions...))

	return rv, &rs.SyncOpResults{}, nil
}

// Grants returns grants for groups assigned to a role.
func (r *roleBuilder) Grants(ctx context.Context, resource *v2.Resource, opts rs.SyncOpAttrs) ([]*v2.Grant, *rs.SyncOpResults, error) {
	bag, err := parsePageToken(opts.PageToken.Token, &v2.ResourceId{ResourceType: domainResourceType})
	if err != nil {
		return nil, nil, err
	}

	switch bag.ResourceTypeID() {
	case domainResourceType:
		domains, nextDomainsCursor, err := r.client.ListDomains(ctx, bag.PageToken())
		if err != nil {
			return nil, nil, err
		}

		bag.Pop()

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

			bag.Push(
				pagination.PageState{
					ResourceTypeID: groupResourceType.Id,
					Token:          fmt.Sprintf("%s:", d.ID),
				},
			)
		}

		// bag.Current() == nil here means every domain (across every page) had
		// zero groups, so no group phase was ever pushed. NextToken("") would
		// still seed a non-nil empty page state and return a non-empty token,
		// which makes the SDK call back in with a token that doesn't match any
		// resource type below — leaving next as "" instead terminates pagination.
		var next string
		if bag.Current() != nil {
			var err error
			next, err = bag.NextToken(bag.PageToken())
			if err != nil {
				return nil, nil, err
			}
		}

		return nil, &rs.SyncOpResults{NextPageToken: next}, nil

	case groupResourceType.Id:
		parts := strings.Split(bag.PageToken(), ":")
		if len(parts) != 2 {
			return nil, nil, fmt.Errorf("invalid page token: %s (type: %s)", bag.PageToken(), bag.ResourceTypeID())
		}

		domainId := parts[0]
		cursor := parts[1]

		roleName, ok := rs.GetProfileStringValue(rs.GetProfile(resource), "role_name")
		if !ok {
			return nil, nil, fmt.Errorf("unable to get role name from role trait profile")
		}

		groups, nextGroupsCursor, err := r.client.ListGroupsWithRole(ctx, domainId, resource.Id.Resource, cursor)
		if err != nil {
			return nil, nil, err
		}

		c, err := composeCursor(domainId, nextGroupsCursor)
		if err != nil {
			return nil, nil, err
		}

		next, err := bag.NextToken(c)
		if err != nil {
			return nil, nil, err
		}

		var rv []*v2.Grant
		for _, g := range groups {
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

		return rv, &rs.SyncOpResults{NextPageToken: next}, nil

	default:
		return nil, nil, fmt.Errorf("invalid resource type: %s", bag.ResourceTypeID())
	}
}

const (
	orgScope   = "organization"
	accScope   = "account"
	groupScope = "group"
)

func (r *roleBuilder) Grant(ctx context.Context, principal *v2.Resource, entitlement *v2.Entitlement) (annotations.Annotations, error) {
	if principal.Id.ResourceType != groupResourceType.Id {
		return nil, fmt.Errorf("baton-newrelic: only groups can be granted role membership")
	}

	roleScope, ok := rs.GetProfileStringValue(rs.GetProfile(entitlement.Resource), "role_scope")
	if !ok {
		return nil, fmt.Errorf("unable to get role scope from role trait profile")
	}

	roleId, groupId := entitlement.Resource.Id.Resource, principal.Id.Resource
	var err error
	switch roleScope {
	case orgScope:
		err = r.client.AddOrgRole(ctx, roleId, groupId)
	case accScope:
		err = r.client.AddAccountRole(ctx, roleId, groupId, r.client.AccountId)
	case groupScope:
		err = r.client.AddGroupRole(ctx, roleId, groupId)
	default:
		return nil, fmt.Errorf("baton-newrelic: role scope %s is not supported", roleScope)
	}

	if err != nil {
		if errors.Is(err, newrelic.ErrRoleAlreadyAssigned) {
			return annotations.New(&v2.GrantAlreadyExists{}), nil
		}
		return nil, fmt.Errorf("baton-newrelic: failed to add role to group: %w", err)
	}

	return nil, nil
}

func (r *roleBuilder) Revoke(ctx context.Context, grant *v2.Grant) (annotations.Annotations, error) {
	principal := grant.Principal
	entitlement := grant.Entitlement

	if principal.Id.ResourceType != groupResourceType.Id {
		return nil, fmt.Errorf("baton-newrelic: only groups can have role membership revoked")
	}

	roleScope, ok := rs.GetProfileStringValue(rs.GetProfile(entitlement.Resource), "role_scope")
	if !ok {
		return nil, fmt.Errorf("unable to get role scope from role trait profile")
	}

	roleId, groupId := entitlement.Resource.Id.Resource, principal.Id.Resource
	var err error
	switch roleScope {
	case orgScope:
		err = r.client.RemoveOrgRole(ctx, roleId, groupId)
	case accScope:
		err = r.client.RemoveAccountRole(ctx, roleId, groupId, r.client.AccountId)
	case groupScope:
		err = r.client.RemoveGroupRole(ctx, roleId, groupId)
	default:
		return nil, fmt.Errorf("baton-newrelic: role scope %s is not supported", roleScope)
	}

	if err != nil {
		if errors.Is(err, newrelic.ErrRoleNotAssigned) {
			return annotations.New(&v2.GrantAlreadyRevoked{}), nil
		}
		return nil, fmt.Errorf("baton-newrelic: failed to remove role from group: %w", err)
	}

	return nil, nil
}

func newRoleBuilder(client *newrelic.Client) *roleBuilder {
	return &roleBuilder{
		resourceType: roleResourceType,
		client:       client,
	}
}
