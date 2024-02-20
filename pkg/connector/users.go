package connector

import (
	"context"
	"fmt"

	"github.com/conductorone/baton-newrelic/pkg/newrelic"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/helpers"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	"github.com/conductorone/baton-sdk/pkg/types/resource"
)

type userBuilder struct {
	resourceType *v2.ResourceType
	client       *newrelic.Client
}

func (u *userBuilder) ResourceType(ctx context.Context) *v2.ResourceType {
	return userResourceType
}

func userResource(ctx context.Context, pId *v2.ResourceId, user *newrelic.User) (*v2.Resource, error) {
	firstName, lastName := helpers.SplitFullName(user.Name)
	profile := map[string]interface{}{
		"email":      user.Email,
		"user_id":    user.ID,
		"first_name": firstName,
		"last_name":  lastName,
	}

	resource, err := resource.NewUserResource(
		user.Name,
		userResourceType,
		user.ID,
		[]resource.UserTraitOption{
			resource.WithUserProfile(profile),
			resource.WithEmail(user.Email, true),
			resource.WithUserLogin(user.Email),
			resource.WithStatus(v2.UserTrait_Status_STATUS_ENABLED),
		},
		resource.WithParentResourceID(pId),
	)

	if err != nil {
		return nil, err
	}

	return resource, nil
}

// List returns all the users from the database as resource objects.
// Users include a UserTrait because they are the 'shape' of a standard user.
func (u *userBuilder) List(ctx context.Context, parentResourceID *v2.ResourceId, pToken *pagination.Token) ([]*v2.Resource, string, annotations.Annotations, error) {
	var (
		nextCursor string
		users      []newrelic.User
	)
	if parentResourceID == nil {
		return nil, "", nil, nil
	}

	// parse the token
	bag, err := parsePageToken(pToken.Token, &v2.ResourceId{ResourceType: userResourceType.Id})
	if err != nil {
		return nil, "", nil, err
	}

	domains, _, err := u.client.ListDomains(ctx, bag.PageToken())
	if err != nil {
		return nil, "", nil, err
	}

	if len(domains) == 0 {
		return nil, "", nil, fmt.Errorf("domain not found: %v", domains)
	}

	if len(domains) > 1 {
		return nil, "", nil, fmt.Errorf("found more domains")
	}

	for _, domain := range domains {
		users, nextCursor, err = u.client.ListUsers(ctx, domain.ID, bag.PageToken())
		if err != nil {
			return nil, "", nil, err
		}
	}

	// add next cursor to bag
	next, err := bag.NextToken(nextCursor)
	if err != nil {
		return nil, "", nil, err
	}

	var rv []*v2.Resource
	for _, user := range users {
		userCopy := user
		ur, err := userResource(ctx, parentResourceID, &userCopy)
		if err != nil {
			return nil, "", nil, err
		}

		rv = append(rv, ur)
	}

	return rv, next, nil, nil
}

// Entitlements always returns an empty slice for users.
func (u *userBuilder) Entitlements(_ context.Context, resource *v2.Resource, _ *pagination.Token) ([]*v2.Entitlement, string, annotations.Annotations, error) {
	return nil, "", nil, nil
}

// Grants always returns an empty slice for users since they don't have any entitlements.
func (u *userBuilder) Grants(ctx context.Context, resource *v2.Resource, pToken *pagination.Token) ([]*v2.Grant, string, annotations.Annotations, error) {
	return nil, "", nil, nil
}

func newUserBuilder(client *newrelic.Client) *userBuilder {
	return &userBuilder{
		resourceType: userResourceType,
		client:       client,
	}
}
