package connector

import (
	"context"

	"github.com/conductorone/baton-newrelic/pkg/newrelic"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	rs "github.com/conductorone/baton-sdk/pkg/types/resource"
)

type orgBuilder struct {
	resourceType *v2.ResourceType
	client       *newrelic.Client
}

func (o *orgBuilder) ResourceType(ctx context.Context) *v2.ResourceType {
	return orgResourceType
}

func orgResource(ctx context.Context, org *newrelic.Org) (*v2.Resource, error) {
	resource, err := rs.NewResource(
		org.Name,
		orgResourceType,
		org.ID,
		rs.WithAnnotation(
			&v2.ChildResourceType{ResourceTypeId: groupResourceType.Id},
			&v2.ChildResourceType{ResourceTypeId: roleResourceType.Id},
			&v2.ChildResourceType{ResourceTypeId: userResourceType.Id},
		),
	)

	if err != nil {
		return nil, err
	}

	return resource, nil
}

// List returns all the orgs from the database as resource objects.
func (o *orgBuilder) List(ctx context.Context, _ *v2.ResourceId, _ rs.SyncOpAttrs) ([]*v2.Resource, *rs.SyncOpResults, error) {
	org, err := o.client.GetOrg(ctx)
	if err != nil {
		return nil, nil, err
	}

	var rv []*v2.Resource

	or, err := orgResource(ctx, org)
	if err != nil {
		return nil, nil, err
	}

	rv = append(rv, or)

	return rv, &rs.SyncOpResults{}, nil
}

// Entitlements always returns an empty slice for orgs.
func (o *orgBuilder) Entitlements(_ context.Context, _ *v2.Resource, _ rs.SyncOpAttrs) ([]*v2.Entitlement, *rs.SyncOpResults, error) {
	return nil, nil, nil
}

// Grants always returns an empty slice for orgs since they don't have any entitlements.
func (o *orgBuilder) Grants(_ context.Context, _ *v2.Resource, _ rs.SyncOpAttrs) ([]*v2.Grant, *rs.SyncOpResults, error) {
	return nil, nil, nil
}

func newOrgBuilder(client *newrelic.Client) *orgBuilder {
	return &orgBuilder{
		resourceType: orgResourceType,
		client:       client,
	}
}
