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
	groupMembership = "member"
)

type groupBuilder struct {
	resourceType *v2.ResourceType
	client       *newrelic.Client
}

func (g *groupBuilder) ResourceType(ctx context.Context) *v2.ResourceType {
	return groupResourceType
}

func groupResource(ctx context.Context, parentId *v2.ResourceId, domainId string, group *newrelic.Group) (*v2.Resource, error) {
	profile := map[string]interface{}{
		"group_domain": domainId,
	}

	resource, err := rs.NewGroupResource(
		group.Name,
		groupResourceType,
		group.ID,
		[]rs.GroupTraitOption{
			rs.WithGroupProfile(profile),
		},
		rs.WithParentResourceID(parentId),
	)

	if err != nil {
		return nil, err
	}

	return resource, nil
}

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

	switch bag.ResourceTypeID() {
	case domainResourceType:
		// list and paginate through domains
		domains, nextDomainsCursor, err := g.client.ListDomains(ctx, bag.PageToken())
		if err != nil {
			return nil, "", nil, err
		}

		// remove old cursors from bag
		bag.Pop()

		// add cursor for paginating next domains to bag
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

			// add cursors for paginating groups under this domain
			bag.Push(
				pagination.PageState{
					ResourceTypeID: groupResourceType.Id,
					Token:          fmt.Sprintf("%s:", d.ID),
				},
			)
		}

		// if there are no more cursors, return nil
		var token string
		if bag.Current() != nil {
			token = bag.PageToken()
		}

		// handle next iteration
		next, err := bag.NextToken(token)
		if err != nil {
			if err.Error() != "no active page state" {
				return nil, "", nil, err
			}
		}

		return nil, next, nil, nil

	case groupResourceType.Id:
		// list and paginate through groups within a domain
		parts := strings.Split(bag.PageToken(), ":")
		if len(parts) != 2 {
			return nil, "", nil, fmt.Errorf("invalid page token: %s (type: %s)", bag.PageToken(), bag.ResourceTypeID())
		}

		domainId := parts[0]
		cursor := parts[1]

		// list all groups within all domains with specific role
		groups, nextGroupsCursor, err := g.client.ListGroups(ctx, domainId, cursor)
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

		var rv []*v2.Resource
		for _, g := range groups {
			groupCopy := g

			gr, err := groupResource(ctx, parentResourceID, domainId, &groupCopy)
			if err != nil {
				return nil, "", nil, err
			}

			rv = append(rv, gr)
		}

		return rv, next, nil, nil

	default:
		return nil, "", nil, fmt.Errorf("invalid resource type: %s", bag.ResourceTypeID())
	}
}

// Entitlements always returns an empty slice for groups.
func (g *groupBuilder) Entitlements(_ context.Context, resource *v2.Resource, _ *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	var rv []*v2.Entitlement

	permissionOptions := []ent.EntitlementOption{
		ent.WithGrantableTo(userResourceType),
		ent.WithDisplayName(fmt.Sprintf("%s Group %s", resource.DisplayName, groupMembership)),
		ent.WithDescription(fmt.Sprintf("%s access to %s group in NewRelic", groupMembership, resource.DisplayName)),
	}

	rv = append(rv, ent.NewAssignmentEntitlement(resource, groupMembership, permissionOptions...))

	return rv, "", nil, nil
}

// Grants always returns an empty slice for groups since they don't have any entitlements.
func (g *groupBuilder) Grants(ctx context.Context, resource *v2.Resource, pToken *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
	bag, err := parsePageToken(pToken.Token, &v2.ResourceId{ResourceType: userResourceType.Id})
	if err != nil {
		return nil, "", nil, err
	}

	// obtain domain id from group profile
	groupTrait, err := rs.GetGroupTrait(resource)
	if err != nil {
		return nil, "", nil, err
	}

	domainId, ok := rs.GetProfileStringValue(groupTrait.Profile, "group_domain")
	if !ok {
		return nil, "", nil, fmt.Errorf("unable to get domain id from group trait profile")
	}

	members, nextDomainsCursor, err := g.client.ListGroupMembers(ctx, domainId, resource.Id.Resource, bag.PageToken())
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

func (g *groupBuilder) Grant(ctx context.Context, principal *v2.Resource, entitlement *v2.Entitlement) (annotations.Annotations, error) {
	l := ctxzap.Extract(ctx)

	if principal.Id.ResourceType != userResourceType.Id {
		l.Warn(
			"newrelic-connector: only users can be granted group membership",
			zap.String("principal_id", principal.Id.String()),
			zap.String("principal_type", principal.Id.ResourceType),
		)

		return nil, fmt.Errorf("newrelic-connector: only users can be granted group membership")
	}

	groupId, userId := entitlement.Resource.Id.Resource, principal.Id.Resource
	err := g.client.AddUserToGroup(ctx, groupId, userId)
	if err != nil {
		return nil, fmt.Errorf("newrelic-connector: failed to add user to group: %w", err)
	}

	return nil, nil
}

func (g *groupBuilder) Revoke(ctx context.Context, grant *v2.Grant) (annotations.Annotations, error) {
	l := ctxzap.Extract(ctx)

	principal := grant.Principal
	entitlement := grant.Entitlement

	if principal.Id.ResourceType != userResourceType.Id {
		l.Warn(
			"newrelic-connector: only users can have group membership revoked",
			zap.String("principal_id", principal.Id.String()),
			zap.String("principal_type", principal.Id.ResourceType),
		)

		return nil, fmt.Errorf("newrelic-connector: only users can have group membership revoked")
	}

	groupId, userId := entitlement.Resource.Id.Resource, principal.Id.Resource
	err := g.client.RemoveUserFromGroup(ctx, groupId, userId)
	if err != nil {
		return nil, fmt.Errorf("newrelic-connector: failed to remove user from group: %w", err)
	}

	return nil, nil
}

func newGroupBuilder(client *newrelic.Client) *groupBuilder {
	return &groupBuilder{
		resourceType: groupResourceType,
		client:       client,
	}
}
