package connector

import (
	"context"

	"github.com/conductorone/baton-newrelic/pkg/newrelic"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/pagination"
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
// Orgs include a OrgTrait because they are the 'shape' of a standard org.
func (o *orgBuilder) List(ctx context.Context, parentResourceID *v2.ResourceId, pToken *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error) {
	org, err := o.client.GetOrg(ctx)
	if err != nil {
		return nil, "", nil, err
	}

	var rv []*v2.Resource

	or, err := orgResource(ctx, org)
	if err != nil {
		return nil, "", nil, err
	}

	rv = append(rv, or)

	return rv, "", nil, nil
}

// Entitlements always returns an empty slice for orgs.
func (o *orgBuilder) Entitlements(_ context.Context, resource *v2.Resource, _ *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	return nil, "", nil, nil
}

// Grants always returns an empty slice for orgs since they don't have any entitlements.
func (o *orgBuilder) Grants(ctx context.Context, resource *v2.Resource, pToken *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
	return nil, "", nil, nil
}

func newOrgBuilder(client *newrelic.Client) *orgBuilder {
	return &orgBuilder{
		resourceType: orgResourceType,
		client:       client,
	}
}
