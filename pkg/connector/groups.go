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
	"github.com/conductorone/baton-sdk/pkg/types/resource"
)

const (
	groupMembership = "member"
)

type groupBuilder struct {
	resourceType *v2.ResourceType
	client       *newrelic.Client
}

func (g *groupBuilder) ResourceType(ctx context.Context) *v2.ResourceType {
	return groupResourceType
}

func groupResource(ctx context.Context, parentId *v2.ResourceId, group *newrelic.Group) (*v2.Resource, error) {
	resource, err := resource.NewGroupResource(
		group.Name,
		groupResourceType,
		group.ID,
		nil,
		resource.WithParentResourceID(parentId),
	)

	if err != nil {
		return nil, err
	}

	return resource, nil
}

var domainResourceType = "domain"

// List returns all the groups from the database as resource objects.
// Groups include a GroupTrait because they are the 'shape' of a standard group.
func (g *groupBuilder) List(ctx context.Context, parentResourceID *v2.ResourceId, pToken *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error) {
	if parentResourceID == nil {
		return nil, "", nil, nil
	}

	// parse the token
	bag, err := parsePageToken(pToken.Token, &v2.ResourceId{ResourceType: domainResourceType})
	if err != nil {
		return nil, "", nil, err
	}

	// TODO: test this properly
	// flow of this function is:
	// 1. query all domains and groups within those domains
	// 2. obtain cursors for paginating over them
	// 3. paginate through domains
	//   3.1. paginate through groups within domain
	//   3.2. append new page tokens to bag (or return) to be used in next iteration
	//   3.3. return resources, groups, and cursor for next groups call
	// 4. at the end of iterating groups within domains, return resources, groups, and cursor for next domains call

	// take in consideration that this function is the only that will handle both domains and groups
	// so we need to know if we are iterating over domains or groups
	// we obtained token to iterate over domains
	// we need to query all domains and groups within those domains
	groups, domainsCursor, groupsCursors, err := g.client.ListGroups(ctx, bag.PageToken())
	if err != nil {
		return nil, "", nil, err
	}

	// first push the cursor for next domains
	bag.Pop()

	if domainsCursor != "" {
		bag.Push(
			pagination.PageState{
				ResourceTypeID: domainResourceType,
				Token:          composeCursor(domainsCursor, ""),
			},
		)
	}

	// then push cursors for paginating groups within domains
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

	var rv []*v2.Resource
	for _, g := range groups {
		for _, group := range g {
			groupCopy := group
			gr, err := groupResource(ctx, parentResourceID, &groupCopy)
			if err != nil {
				return nil, "", nil, err
			}

			rv = append(rv, gr)
		}
	}

	return rv, bag.PageToken(), nil, nil

}

// Entitlements always returns an empty slice for groups.
func (g *groupBuilder) Entitlements(_ context.Context, resource *v2.Resource, _ *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	var rv []*v2.Entitlement

	permissionOptions := []ent.EntitlementOption{
		ent.WithGrantableTo(userResourceType),
		ent.WithDisplayName(fmt.Sprintf("%s Group %s", resource.DisplayName, groupMembership)),
		ent.WithDescription(fmt.Sprintf("%s access to %s group in DockerHub", groupMembership, resource.DisplayName)),
	}

	rv = append(rv, ent.NewAssignmentEntitlement(resource, groupMembership, permissionOptions...))

	return rv, "", nil, nil
}

// Grants always returns an empty slice for groups since they don't have any entitlements.
func (g *groupBuilder) Grants(ctx context.Context, resource *v2.Resource, pToken *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
	bag, err := parsePageToken(pToken.Token, &v2.ResourceId{ResourceType: domainResourceType})
	if err != nil {
		return nil, "", nil, err
	}

	members, nextDomainsCursor, err := g.client.ListGroupMembers(ctx, resource.Id.Resource, bag.PageToken())
	if err != nil {
		return nil, "", nil, err
	}

	next, err := bag.NextToken(nextDomainsCursor)
	if err != nil {
		return nil, "", nil, err
	}

	var rv []*v2.Grant
	for _, uId := range members {
		rv = append(rv, grant.NewGrant(
			resource,
			groupMembership,
			&v2.ResourceId{
				ResourceType: userResourceType.Id,
				Resource:     uId,
			},
		))

	}

	return rv, next, nil, nil
}

func newGroupBuilder(client *newrelic.Client) *groupBuilder {
	return &groupBuilder{
		resourceType: groupResourceType,
		client:       client,
	}
}
