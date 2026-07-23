package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/conductorone/baton-newrelic/pkg/newrelic"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	"github.com/conductorone/baton-sdk/pkg/annotations"
	"github.com/conductorone/baton-sdk/pkg/connectorbuilder"
	"github.com/conductorone/baton-sdk/pkg/types/resource"
	"google.golang.org/protobuf/types/known/structpb"
)

// stringField returns the string value of a profile field, or "" if the
// profile or the field is unset. GetFields/GetStringValue are nil-safe, so
// this is safe to call with a nil profile.
func stringField(profile *structpb.Struct, key string) string {
	return profile.GetFields()[key].GetStringValue()
}

const (
	profileEmail  = "email"
	profileUserID = "user_id"
)

type userBuilder struct {
	resourceType           *v2.ResourceType
	client                 *newrelic.Client
	authenticationDomainID string
	orgParentIDCache       atomic.Pointer[v2.ResourceId]
}

// multiDomainState is JSON-encoded as the pagination cursor when an org has more
// than one authentication domain. Cursors in NerdGraph are domain-specific, so
// we must paginate each domain's users independently.
type multiDomainState struct {
	DomainIDs  []string `json:"dids"`
	DomainIdx  int      `json:"didx"`
	UserCursor string   `json:"uc,omitempty"`
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
func (u *userBuilder) List(ctx context.Context, parentResourceID *v2.ResourceId, opts resource.SyncOpAttrs) ([]*v2.Resource, *resource.SyncOpResults, error) {
	if parentResourceID == nil {
		return nil, nil, nil
	}

	bag, err := parsePageToken(opts.PageToken.Token, &v2.ResourceId{ResourceType: userResourceType.Id})
	if err != nil {
		return nil, nil, err
	}

	rawCursor := bag.PageToken()

	// Pagination state is always carried as a JSON multiDomainState: on the first
	// call we enumerate every authentication domain once and cache their IDs in the
	// cursor. Caching means ListDomains runs a single time per sync (not per user
	// page), and each domain is paged independently because NerdGraph user cursors
	// are domain-specific.
	var mds *multiDomainState
	if len(rawCursor) > 0 && rawCursor[0] == '{' {
		var s multiDomainState
		if err := json.Unmarshal([]byte(rawCursor), &s); err != nil {
			return nil, nil, fmt.Errorf("baton-newrelic: invalid pagination cursor: %w", err)
		}
		mds = &s
	} else {
		// First call: enumerate ALL authentication domains, following the domain
		// cursor so orgs with more domains than fit on one page aren't dropped.
		domains, err := u.client.ListAllDomains(ctx)
		if err != nil {
			return nil, nil, err
		}
		if len(domains) == 0 {
			return nil, nil, nil
		}
		ids := make([]string, len(domains))
		for i, d := range domains {
			ids[i] = d.ID
		}
		mds = &multiDomainState{DomainIDs: ids}
	}

	// DomainIdx >= len is a defensive guard; the cursor-marshal logic below never
	// encodes an out-of-range index, so this branch is unreachable in normal flow.
	if mds.DomainIdx >= len(mds.DomainIDs) {
		next, err := bag.NextToken("")
		if err != nil {
			return nil, nil, err
		}
		return nil, &resource.SyncOpResults{NextPageToken: next}, nil
	}
	domainID := mds.DomainIDs[mds.DomainIdx]
	userCursor := mds.UserCursor

	users, nextUserCursor, err := u.client.ListUsers(ctx, domainID, userCursor)
	if err != nil {
		return nil, nil, err
	}

	var nextState *multiDomainState
	switch {
	case nextUserCursor != "":
		nextState = &multiDomainState{
			DomainIDs:  mds.DomainIDs,
			DomainIdx:  mds.DomainIdx,
			UserCursor: nextUserCursor,
		}
	case mds.DomainIdx+1 < len(mds.DomainIDs):
		nextState = &multiDomainState{
			DomainIDs: mds.DomainIDs,
			DomainIdx: mds.DomainIdx + 1,
		}
		// default: all domains exhausted → nextState stays nil, nextCursorStr = "" signals completion
	}

	var nextCursorStr string
	if nextState != nil {
		b, merr := json.Marshal(nextState)
		if merr != nil {
			return nil, nil, fmt.Errorf("baton-newrelic: failed to marshal pagination cursor: %w", merr)
		}
		nextCursorStr = string(b)
	}

	next, err := bag.NextToken(nextCursorStr)
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

	email := stringField(profile, "email")
	if email == "" {
		email = accountInfo.GetLogin()
	}
	if email == "" {
		return nil, nil, nil, fmt.Errorf("baton-newrelic: create account requires an email address")
	}

	name := stringField(profile, "name")
	if name == "" {
		name = email
	}

	userType := stringField(profile, "user_type")
	if userType == "" {
		userType = "FULL_USER_TIER"
	}

	authDomainID := stringField(profile, "authentication_domain_id")
	if authDomainID == "" {
		authDomainID = u.authenticationDomainID
	}
	if authDomainID == "" {
		return nil, nil, nil, fmt.Errorf("baton-newrelic: create account requires authentication_domain_id")
	}

	// Best-effort parent link: users are child resources of the org (see
	// organizations.go), so mirror the parent the same user would receive from a
	// normal sync. If the org lookup fails we still return the created/existing user
	// without the parent link, which the next full sync repairs.
	parentID := u.orgParentID(ctx)

	// Step 1: check if user already exists.
	existing, err := u.client.GetUserByEmail(ctx, email)
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
			if existing, gErr := u.client.GetUserByEmail(ctx, email); gErr == nil && existing != nil {
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

// Delete permanently removes a user. Returns success if the user is not found (idempotent).
func (u *userBuilder) Delete(ctx context.Context, resourceId *v2.ResourceId, _ *v2.ResourceId) (annotations.Annotations, error) {
	if err := u.client.DeleteUser(ctx, resourceId.GetResource()); err != nil {
		return nil, fmt.Errorf("baton-newrelic: failed to delete user %s: %w", resourceId.GetResource(), err)
	}
	return nil, nil
}

func newUserBuilder(client *newrelic.Client, authDomainID string) *userBuilder {
	return &userBuilder{
		resourceType:           userResourceType,
		client:                 client,
		authenticationDomainID: authDomainID,
	}
}
