package connector

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/conductorone/baton-newrelic/pkg/newrelic"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sdk/pkg/pagination"
	"github.com/conductorone/baton-sdk/pkg/types/resource"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/structpb"
)

// stringField returns the string value of a profile field, or "" if the
// profile or the field is unset. GetFields/GetStringValue are nil-safe, so
// this is safe to call with a nil profile.
func stringField(profile *structpb.Struct, key string) string {
	return profile.GetFields()[key].GetStringValue()
}

const (
	profileEmail        = "email"
	profileUserID       = "user_id"
	profileName         = "name"
	profileUserType     = "user_type"
	profileAuthDomainID = "authentication_domain_id"

	// emailVerificationStatePending is NerdGraph's emailVerificationState value for a
	// user whose invite hasn't been accepted yet (per New Relic's UserManagement
	// schema: one of "Not Verifiable", "Verified", or "Pending").
	emailVerificationStatePending = "Pending"
)

type userBuilder struct {
	resourceType     *v2.ResourceType
	client           *newrelic.Client
	orgParentIDCache atomic.Pointer[v2.ResourceId]
}

func (u *userBuilder) ResourceType(ctx context.Context) *v2.ResourceType {
	return userResourceType
}

func userResource(ctx context.Context, pId *v2.ResourceId, user *newrelic.User) (*v2.Resource, error) {
	firstName, lastName := resource.SplitFullName(user.Name)
	profile := map[string]interface{}{
		profileEmail:  user.Email,
		profileUserID: user.ID,
		"first_name":  firstName,
		"last_name":   lastName,
	}

	// A user whose invite hasn't been accepted yet (emailVerificationState ==
	// "Pending") can't do anything with the account, so it's not yet enabled.
	status := v2.Status_RESOURCE_STATUS_ENABLED
	if user.EmailVerificationState == emailVerificationStatePending {
		status = v2.Status_RESOURCE_STATUS_DISABLED
	}

	resource, err := resource.NewUserResource(
		user.Name,
		userResourceType,
		user.ID,
		[]resource.UserTraitOption{
			resource.WithEmail(user.Email, true),
			resource.WithUserLogin(user.Email),
		},
		resource.WithParentResourceID(pId),
		resource.WithResourceProfile(profile),
		resource.WithResourceStatus(status, ""),
	)

	if err != nil {
		return nil, err
	}

	return resource, nil
}

// List returns all the users from the database as resource objects.
// Users include a UserTrait because they are the 'shape' of a standard user.
//
// Pagination uses the SDK Bag to page each authentication domain as its own
// phase, replacing the previous hand-rolled JSON cursor.
func (u *userBuilder) List(ctx context.Context, parentResourceID *v2.ResourceId, opts resource.SyncOpAttrs) ([]*v2.Resource, *resource.SyncOpResults, error) {
	if parentResourceID == nil {
		return nil, nil, nil
	}

	bag := &pagination.Bag{}
	if err := bag.Unmarshal(opts.PageToken.Token); err != nil {
		return nil, nil, fmt.Errorf("baton-newrelic: invalid pagination cursor: %w", err)
	}

	// First call: enumerate every authentication domain once and push one
	// pagination phase per domain. NerdGraph user cursors are domain-specific, so
	// each domain is paged independently as its own phase; the Bag advances to the
	// next domain automatically once a domain's users are exhausted. The domain IDs
	// live in the marshaled token, so ListAllDomains runs a single time per sync.
	if bag.Current() == nil {
		domains, err := u.client.ListAllDomains(ctx)
		if err != nil {
			return nil, nil, err
		}
		for _, d := range domains {
			bag.Push(pagination.PageState{
				ResourceTypeID: userResourceType.Id,
				ResourceID:     d.ID,
			})
		}
		// No authentication domains → nothing to sync. Can happen for zero-domain
		// orgs or when the API key lacks visibility into any domain.
		if bag.Current() == nil {
			ctxzap.Extract(ctx).Debug("no authentication domains found, skipping user sync")
			return nil, nil, nil
		}
	}

	domainID := bag.ResourceID()
	users, nextUserCursor, err := u.client.ListUsers(ctx, domainID, bag.PageToken())
	if err != nil {
		return nil, nil, err
	}

	// nextUserCursor != "" keeps the current domain on the stack with the new
	// cursor; "" pops it and advances to the next domain (or ends the sync once the
	// last domain is popped).
	next, err := bag.NextToken(nextUserCursor)
	if err != nil {
		return nil, nil, err
	}

	var rv []*v2.Resource
	for _, user := range users {
		userCopy := user
		ur, err := userResource(ctx, parentResourceID, &userCopy)
		if err != nil {
			return nil, nil, err
		}
		rv = append(rv, ur)
	}

	return rv, &resource.SyncOpResults{NextPageToken: next}, nil
}

// Entitlements always returns an empty slice for users.
func (u *userBuilder) Entitlements(_ context.Context, _ *v2.Resource, _ resource.SyncOpAttrs) ([]*v2.Entitlement, *resource.SyncOpResults, error) {
	return nil, nil, nil
}

// Grants always returns an empty slice for users since they don't have any entitlements.
func (u *userBuilder) Grants(_ context.Context, _ *v2.Resource, _ resource.SyncOpAttrs) ([]*v2.Grant, *resource.SyncOpResults, error) {
	return nil, nil, nil
}

// CreateAccountCapabilityDetails reports that no special credential options are needed.
func (u *userBuilder) CreateAccountCapabilityDetails(_ context.Context) (*v2.CredentialDetailsAccountProvisioning, annotations.Annotations, error) {
	return v2.CredentialDetailsAccountProvisioning_builder{
		SupportedCredentialOptions: []v2.CapabilityDetailCredentialOption{
			v2.CapabilityDetailCredentialOption_CAPABILITY_DETAIL_CREDENTIAL_OPTION_NO_PASSWORD,
		},
		PreferredCredentialOption: v2.CapabilityDetailCredentialOption_CAPABILITY_DETAIL_CREDENTIAL_OPTION_NO_PASSWORD,
	}.Build(), nil, nil
}

// CreateAccount provisions a new user:
//  1. Check if user already exists (by email) → return AlreadyExistsResult if so.
//  2. Create the user via userManagementCreateUser.
func (u *userBuilder) CreateAccount(
	ctx context.Context,
	accountInfo *v2.AccountInfo,
	_ *v2.LocalCredentialOptions,
) (connectorbuilder.CreateAccountResponse, []*v2.PlaintextData, annotations.Annotations, error) {
	profile := accountInfo.GetProfile()

	email := stringField(profile, profileEmail)
	if email == "" {
		email = accountInfo.GetLogin()
	}
	if email == "" {
		return nil, nil, nil, fmt.Errorf("baton-newrelic: create account requires an email address")
	}

	name := stringField(profile, profileName)
	if name == "" {
		name = email
	}

	userType := stringField(profile, profileUserType)
	if userType == "" {
		return nil, nil, nil, fmt.Errorf("baton-newrelic: create account requires user_type")
	}

	authDomainID := stringField(profile, profileAuthDomainID)
	if authDomainID == "" {
		return nil, nil, nil, fmt.Errorf("baton-newrelic: create account requires authentication_domain_id")
	}

	// Best-effort parent link: users are child resources of the org (see
	// organizations.go), so mirror the parent the same user would receive from a
	// normal sync. If the org lookup fails we still return the created/existing user
	// without the parent link, which the next full sync repairs.
	parentID := u.orgParentID(ctx)

	// Step 1: check if user already exists.
	existing, err := u.client.GetUserByEmail(ctx, authDomainID, email)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("baton-newrelic: failed to check for existing user: %w", err)
	}
	if existing != nil {
		existingResource, err := userResource(ctx, parentID, existing)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("baton-newrelic: failed to build existing user resource: %w", err)
		}
		return &v2.CreateAccountResponse_AlreadyExistsResult{
			Resource: existingResource,
		}, nil, nil, nil
	}

	// Step 2: create the user.
	newUserID, err := u.client.CreateUser(ctx, authDomainID, email, name, userType)
	if err != nil {
		// A concurrent CreateAccount for the same email (or a user in a domain the
		// pre-check didn't surface) races here: NerdGraph rejects the duplicate.
		// Re-resolve by email and report AlreadyExists instead of a hard error.
		if errors.Is(err, newrelic.ErrUserAlreadyExists) {
			if existing, gErr := u.client.GetUserByEmail(ctx, authDomainID, email); gErr == nil && existing != nil {
				existingResource, rErr := userResource(ctx, parentID, existing)
				if rErr != nil {
					return nil, nil, nil, fmt.Errorf("baton-newrelic: failed to build existing user resource: %w", rErr)
				}
				return &v2.CreateAccountResponse_AlreadyExistsResult{
					Resource: existingResource,
				}, nil, nil, nil
			}
			return &v2.CreateAccountResponse_AlreadyExistsResult{}, nil, nil, nil
		}
		return nil, nil, nil, fmt.Errorf("baton-newrelic: failed to create user: %w", err)
	}

	newUser := &newrelic.User{
		ID:    newUserID,
		Email: email,
		Name:  name,
	}
	newResource, err := userResource(ctx, parentID, newUser)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("baton-newrelic: failed to build new user resource: %w", err)
	}

	return &v2.CreateAccountResponse_SuccessResult{
		Resource: newResource,
	}, nil, nil, nil
}

// orgParentID returns the org resource ID that users hang off of, or nil if the
// org lookup fails (the link is best-effort and self-heals on the next sync).
// The org ID is invariant for the lifetime of the builder, so a successful
// lookup is cached to avoid a redundant GetOrg round-trip on every CreateAccount.
func (u *userBuilder) orgParentID(ctx context.Context) *v2.ResourceId {
	if cached := u.orgParentIDCache.Load(); cached != nil {
		return cached
	}
	org, err := u.client.GetOrg(ctx)
	if err != nil || org == nil {
		return nil
	}
	rid := &v2.ResourceId{ResourceType: orgResourceType.Id, Resource: org.ID}
	u.orgParentIDCache.Store(rid)
	return rid
}

// Delete permanently removes a user. Returns success if the user is not found
// (idempotent). NerdGraph's delete mutation returns the same errorClass and message
// for "user not found" as for "user exists but you lack permission", so that
// ambiguity is resolved here rather than trusting the mutation's own error response:
// check existence first, and skip the mutation only when that check positively
// confirms the user is already gone. Any other outcome — the user still exists, or
// the check itself couldn't reach a conclusion — falls through to calling
// DeleteUser, so its response (not an inconclusive pre-check) is what surfaces as
// the real error if there is one.
func (u *userBuilder) Delete(ctx context.Context, resourceId *v2.ResourceId, _ *v2.ResourceId) (annotations.Annotations, error) {
	existing, checkErr := u.client.GetUserByID(ctx, resourceId.GetResource())
	if checkErr == nil && existing == nil {
		return nil, nil
	}
	if checkErr != nil {
		ctxzap.Extract(ctx).Debug("delete: user existence pre-check inconclusive, falling through to delete",
			zap.String("user_id", resourceId.GetResource()), zap.Error(checkErr))
	}

	if err := u.client.DeleteUser(ctx, resourceId.GetResource()); err != nil {
		return nil, fmt.Errorf("baton-newrelic: failed to delete user %s: %w", resourceId.GetResource(), err)
	}
	return nil, nil
}

func newUserBuilder(client *newrelic.Client) *userBuilder {
	return &userBuilder{
		resourceType: userResourceType,
		client:       client,
	}
}
