package connector

import (
	"context"
	"fmt"

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

	rolesTrait, err := rs.GetRoleTrait(resource)
	if err != nil {
		return nil, "", nil, err
	}

	// get role name
	roleName, ok := rs.GetProfileStringValue(rolesTrait.Profile, "role_name")
	if !ok {
		return nil, "", nil, fmt.Errorf("unable to get role name from role trait profile")
	}

	groups, domainsCursor, groupsCursors, err := r.client.ListGroupsWithRole(ctx, resource.Id.Resource, bag.PageToken())
	if err != nil {
		return nil, "", nil, err
	}

	// remove old cursors from bag
	bag.Pop()

	// first add cursor to look through next domains
	if domainsCursor != "" {
		bag.Push(
			pagination.PageState{
				ResourceTypeID: domainResourceType,
				Token:          composeCursor(domainsCursor, ""),
			},
		)
	}

	// then add cursors to look through groups within domains
	if len(groupsCursors) != 0 {
		for _, gc := range groupsCursors {
			bag.Push(
				pagination.PageState{
					ResourceTypeID: groupResourceType.Id,
					Token:          composeCursor("", gc),
				},
			)
		}
	}

	var rv []*v2.Grant
	for _, g := range groups {
		for _, gr := range g {
			if gr.Roles.TotalCount == 0 {
				continue
			}

			rv = append(rv, grant.NewGrant(
				resource,
				roleName,
				&v2.ResourceId{
					ResourceType: groupResourceType.Id,
					Resource:     gr.ID,
				},
			))
		}
	}

	return rv, bag.PageToken(), nil, nil
}

func newRoleBuilder(client *newrelic.Client) *roleBuilder {
	return &roleBuilder{
		resourceType: roleResourceType,
		client:       client,
	}
}
