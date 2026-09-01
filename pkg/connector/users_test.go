package connector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/conductorone/baton-newrelic/pkg/newrelic"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	rs "github.com/conductorone/baton-sdk/pkg/types/resource"
	"google.golang.org/protobuf/types/known/structpb"
)

// --- userResource status mapping based on emailVerificationState ---
//
// NerdGraph's UserManagement schema documents emailVerificationState as one of
// "Not Verifiable", "Verified", or "Pending". A user is Pending until they
// accept their invite, so they can't do anything with the account yet.

func TestUserResource_PendingEmailVerificationIsDisabled(t *testing.T) {
	user := &newrelic.User{
		ID:                     "user-1",
		Email:                  "pending@example.com",
		Name:                   "Pending User",
		EmailVerificationState: "Pending",
	}

	r, err := userResource(context.Background(), nil, user)
	if err != nil {
		t.Fatalf("userResource: %v", err)
	}

	status := rs.GetStatus(r)
	if status == nil || status.GetStatus() != v2.Status_RESOURCE_STATUS_DISABLED {
		got := v2.Status_RESOURCE_STATUS_UNSPECIFIED
		if status != nil {
			got = status.GetStatus()
		}
		t.Errorf("status = %v, want RESOURCE_STATUS_DISABLED for Pending user", got)
	}
}

func TestUserResource_VerifiedEmailIsEnabled(t *testing.T) {
	user := &newrelic.User{
		ID:                     "user-2",
		Email:                  "verified@example.com",
		Name:                   "Verified User",
		EmailVerificationState: "Verified",
	}

	r, err := userResource(context.Background(), nil, user)
	if err != nil {
		t.Fatalf("userResource: %v", err)
	}

	status := rs.GetStatus(r)
	if status == nil || status.GetStatus() != v2.Status_RESOURCE_STATUS_ENABLED {
		got := v2.Status_RESOURCE_STATUS_UNSPECIFIED
		if status != nil {
			got = status.GetStatus()
		}
		t.Errorf("status = %v, want RESOURCE_STATUS_ENABLED for Verified user", got)
	}
}

func TestUserResource_NotVerifiableEmailIsEnabled(t *testing.T) {
	user := &newrelic.User{
		ID:                     "user-3",
		Email:                  "notverifiable@example.com",
		Name:                   "Not Verifiable User",
		EmailVerificationState: "Not Verifiable",
	}

	r, err := userResource(context.Background(), nil, user)
	if err != nil {
		t.Fatalf("userResource: %v", err)
	}

	status := rs.GetStatus(r)
	if status == nil || status.GetStatus() != v2.Status_RESOURCE_STATUS_ENABLED {
		got := v2.Status_RESOURCE_STATUS_UNSPECIFIED
		if status != nil {
			got = status.GetStatus()
		}
		t.Errorf("status = %v, want RESOURCE_STATUS_ENABLED for Not Verifiable user", got)
	}
}

// --- CreateAccount: user_type is a required field, not a silent default ---
//
// user_type is marked Required in the account-creation schema (see
// connector.go), and its siblings (email, authentication_domain_id) both
// hard-fail when empty. Silently defaulting an omitted, Required user_type to
// FULL_USER_TIER (New Relic's most-privileged tier) would fail open to the
// most powerful option, so an empty user_type must error like its siblings.

func TestCreateAccount_MissingUserTypeErrors(t *testing.T) {
	profile, err := structpb.NewStruct(map[string]interface{}{
		profileEmail:        "new.user@example.com",
		profileAuthDomainID: "domain-1",
	})
	if err != nil {
		t.Fatalf("structpb.NewStruct: %v", err)
	}
	accountInfo := v2.AccountInfo_builder{Profile: profile}.Build()

	u := &userBuilder{resourceType: userResourceType}
	_, _, _, err = u.CreateAccount(context.Background(), accountInfo, nil)
	if err == nil {
		t.Fatal("expected error when user_type is omitted, got nil")
	}
	if !strings.Contains(err.Error(), profileUserType) {
		t.Errorf("error = %q, want it to mention %s", err.Error(), profileUserType)
	}
}

func TestCreateAccount_ExplicitUserTypeSucceeds(t *testing.T) {
	const wantUserID = "new-user-id-123"

	accountCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// NewClient's constructor issues a one-time GetAccountId call before any
		// other request.
		if !accountCalled {
			accountCalled = true
			_, _ = w.Write([]byte(`{"data":{"actor":{"accounts":[{"id":1}]}}}`))
			return
		}

		var body struct {
			Query string `json:"query"`
		}
		if decodeErr := json.NewDecoder(r.Body).Decode(&body); decodeErr != nil {
			t.Fatalf("decode request body: %v", decodeErr)
		}

		switch {
		case strings.Contains(body.Query, "userManagementCreateUser"):
			_, _ = w.Write([]byte(`{"data":{"userManagementCreateUser":{"createdUser":{"id":"` + wantUserID + `","email":"new.user@example.com","name":"New User"}}}}`))
		case strings.Contains(body.Query, "authenticationDomains"):
			// GetUserByEmail pre-check: no existing user in this domain.
			_, _ = w.Write([]byte(`{"data":{"actor":{"organization":{"userManagement":{"authenticationDomains":{"authenticationDomains":[]}}}}}}`))
		default:
			// Best-effort org lookup (orgParentID) or anything else unrelated to
			// this test: return an empty, error-free response.
			_, _ = w.Write([]byte(`{"data":{}}`))
		}
	}))
	defer srv.Close()

	client, err := newrelic.NewClient(context.Background(), http.DefaultClient, "test-key", srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	profile, err := structpb.NewStruct(map[string]interface{}{
		profileEmail:        "new.user@example.com",
		profileName:         "New User",
		profileUserType:     "CORE_USER_TIER",
		profileAuthDomainID: "domain-1",
	})
	if err != nil {
		t.Fatalf("structpb.NewStruct: %v", err)
	}
	accountInfo := v2.AccountInfo_builder{Profile: profile}.Build()

	u := &userBuilder{resourceType: userResourceType, client: client}
	resp, _, _, err := u.CreateAccount(context.Background(), accountInfo, nil)
	if err != nil {
		t.Fatalf("CreateAccount: unexpected error: %v", err)
	}

	success, ok := resp.(*v2.CreateAccountResponse_SuccessResult)
	if !ok {
		t.Fatalf("response type = %T, want *v2.CreateAccountResponse_SuccessResult", resp)
	}
	if got := success.Resource.GetId().GetResource(); got != wantUserID {
		t.Errorf("created user id = %q, want %q", got, wantUserID)
	}
}

// --- Delete: existence check resolves NerdGraph's not-found/forbidden ambiguity ---
//
// userManagementDeleteUser returns the same errorClass and message for a missing
// user as for a user the credential isn't authorized to see, so Delete checks
// existence via GetUserByID first rather than trusting the mutation's own error
// response for that distinction.

func TestDelete_AlreadyGoneNeverCallsMutation(t *testing.T) {
	accountCalled := false
	mutationCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if !accountCalled {
			accountCalled = true
			_, _ = w.Write([]byte(`{"data":{"actor":{"accounts":[{"id":1}]}}}`))
			return
		}

		var body struct {
			Query string `json:"query"`
		}
		if decodeErr := json.NewDecoder(r.Body).Decode(&body); decodeErr != nil {
			t.Fatalf("decode request body: %v", decodeErr)
		}

		switch {
		case strings.Contains(body.Query, "GetUserByID"):
			_, _ = w.Write([]byte(`{"data":{"actor":{"organization":{"userManagement":{"authenticationDomains":{"authenticationDomains":[{"users":{"users":[]}}]}}}}}}`))
		case strings.Contains(body.Query, "userManagementDeleteUser"):
			mutationCalled = true
			_, _ = w.Write([]byte(`{"errors":[{"message":"Could not find the target or you are unauthorized.","extensions":{"errorClass":"CLIENT_ERROR"}}]}`))
		default:
			t.Fatalf("unexpected query: %s", body.Query)
		}
	}))
	defer srv.Close()

	client, err := newrelic.NewClient(context.Background(), http.DefaultClient, "test-key", srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	u := &userBuilder{resourceType: userResourceType, client: client}
	resourceId := v2.ResourceId_builder{ResourceType: userResourceType.Id, Resource: "missing-id"}.Build()

	if _, err := u.Delete(context.Background(), resourceId, nil); err != nil {
		t.Errorf("Delete should return nil when GetUserByID finds no user, got: %v", err)
	}
	if mutationCalled {
		t.Error("Delete should not call the delete mutation once the pre-check finds no user")
	}
}

func TestDelete_ExistsButMutationFailsSurfacesAsError(t *testing.T) {
	// The user exists (pre-check finds them), but the delete mutation itself fails
	// with NerdGraph's verbatim CLIENT_ERROR/"already unauthorized" response — most
	// likely a real permission problem. This must surface as an error, not be
	// swallowed, so C1 never marks an account deprovisioned when it still has access.
	accountCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if !accountCalled {
			accountCalled = true
			_, _ = w.Write([]byte(`{"data":{"actor":{"accounts":[{"id":1}]}}}`))
			return
		}

		var body struct {
			Query string `json:"query"`
		}
		if decodeErr := json.NewDecoder(r.Body).Decode(&body); decodeErr != nil {
			t.Fatalf("decode request body: %v", decodeErr)
		}

		switch {
		case strings.Contains(body.Query, "GetUserByID"):
			_, _ = w.Write([]byte(`{"data":{"actor":{"organization":{"userManagement":{"authenticationDomains":{"authenticationDomains":[{"users":{"users":[{"id":"user-1","email":"user1@example.com","name":"User One"}]}}]}}}}}}`))
		case strings.Contains(body.Query, "userManagementDeleteUser"):
			_, _ = w.Write([]byte(`{"errors":[{"message":"Could not find the target or you are unauthorized.","extensions":{"errorClass":"CLIENT_ERROR"}}]}`))
		default:
			t.Fatalf("unexpected query: %s", body.Query)
		}
	}))
	defer srv.Close()

	client, err := newrelic.NewClient(context.Background(), http.DefaultClient, "test-key", srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	u := &userBuilder{resourceType: userResourceType, client: client}
	resourceId := v2.ResourceId_builder{ResourceType: userResourceType.Id, Resource: "user-1"}.Build()

	if _, err := u.Delete(context.Background(), resourceId, nil); err == nil {
		t.Error("Delete should return error when the mutation fails for a user known to still exist, got nil")
	}
}
