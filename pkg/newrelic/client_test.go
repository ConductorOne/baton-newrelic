package newrelic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestClient builds a Client pointed at the given test-server URL.
// NewClient skips GetAccountId when httpClient is nil; use http.DefaultClient
// so that the stored httpClient field is non-nil for subsequent calls.
func newTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	c, err := NewClient(context.Background(), http.DefaultClient, "test-key", serverURL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

// --- T5: isNotFoundErr ---

func TestIsNotFoundErr_NRLiteralMessage(t *testing.T) {
	// NR's real not-found message as observed in production responses.
	e := GraphqlError{Message: "could not find the target or you are unauthorized."}
	if !isNotFoundErr(e) {
		t.Errorf("expected isNotFoundErr to return true for NR literal message")
	}
}

func TestIsNotFoundErr_ErrorClassExtension(t *testing.T) {
	e := GraphqlError{
		Message:    "some other wording",
		Extensions: map[string]interface{}{"errorClass": "NOT_FOUND"},
	}
	if !isNotFoundErr(e) {
		t.Errorf("expected isNotFoundErr to return true for NOT_FOUND errorClass")
	}
}

func TestIsNotFoundErr_OtherError(t *testing.T) {
	e := GraphqlError{Message: "permission denied", Extensions: map[string]interface{}{"errorClass": "FORBIDDEN"}}
	if isNotFoundErr(e) {
		t.Errorf("expected isNotFoundErr to return false for unrelated error")
	}
}

// --- T3: mutation error path (HTTP 200 + top-level errors array) ---

// accountsHandler is a minimal handler that satisfies the GetAccountId call made
// by NewClient, then delegates all subsequent requests to next.
func accountsHandler(next http.Handler) http.HandlerFunc {
	called := false
	return func(w http.ResponseWriter, r *http.Request) {
		if !called {
			called = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"actor":{"accounts":[{"id":1}]}}}`))
			return
		}
		next.ServeHTTP(w, r)
	}
}

func TestAddUserToGroup_GraphQLErrorHTTP200(t *testing.T) {
	mutHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// NerdGraph returns HTTP 200 but with a top-level errors array — mutation silently
		// no-op'd before the explicit error-array check was in place.
		_, _ = w.Write([]byte(`{"errors":[{"message":"user already in group","extensions":{"errorClass":"BAD_USER_INPUT"}}]}`))
	})
	srv := httptest.NewServer(accountsHandler(mutHandler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	if err := client.AddUserToGroup(context.Background(), "group-1", "user-1"); err == nil {
		t.Fatal("expected error from AddUserToGroup when NerdGraph returns HTTP 200 + errors array, got nil")
	}
}

// TestAddUserToGroup_AlreadyMemberIsErrAlreadyMember uses NerdGraph's verbatim
// duplicate-membership response — errorClass SERVER_ERROR, message "Validation
// failed: Group has already been taken" — as observed in production (CXH-2333).
// A fixture using invented wording or errorClass would pass without exercising
// the real matcher path.
func TestAddUserToGroup_AlreadyMemberIsErrAlreadyMember(t *testing.T) {
	mutHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":[{"message":"Validation failed: Group has already been taken","extensions":{"errorClass":"SERVER_ERROR"}}]}`))
	})
	srv := httptest.NewServer(accountsHandler(mutHandler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	err := client.AddUserToGroup(context.Background(), "group-1", "user-1")
	if !errors.Is(err, ErrAlreadyMember) {
		t.Fatalf("expected ErrAlreadyMember, got %v", err)
	}
}

// --- T6: GetUserByEmail returns v2 identity id via filtered query ---

func TestGetUserByEmail_ReturnsV2ID(t *testing.T) {
	const wantID = "v2-identity-id-abc"
	const wantEmail = "alice@example.com"
	const wantDomainID = "domain-1"

	queryHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body GraphqlBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		// Verify this is the filtered email query (not a ListUsers paginated scan).
		if body.Variables["email"] != wantEmail {
			http.Error(w, "unexpected email variable", http.StatusBadRequest)
			return
		}
		if body.Variables["domainId"] != wantDomainID {
			http.Error(w, "unexpected domainId variable", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"actor": map[string]interface{}{
					"organization": map[string]interface{}{
						"userManagement": map[string]interface{}{
							"authenticationDomains": map[string]interface{}{
								"authenticationDomains": []interface{}{
									map[string]interface{}{
										"users": map[string]interface{}{
											"users": []interface{}{
												map[string]interface{}{
													"id":    wantID,
													"email": wantEmail,
													"name":  "Alice",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		})
	})
	srv := httptest.NewServer(accountsHandler(queryHandler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	user, err := client.GetUserByEmail(context.Background(), wantDomainID, wantEmail)
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if user == nil {
		t.Fatal("expected user, got nil")
	}
	if user.ID != wantID {
		t.Errorf("ID = %q, want %q (v2 identity id)", user.ID, wantID)
	}
}

func TestGetUserByEmail_NotFound(t *testing.T) {
	emptyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"actor": map[string]interface{}{
					"organization": map[string]interface{}{
						"userManagement": map[string]interface{}{
							"authenticationDomains": map[string]interface{}{
								"authenticationDomains": []interface{}{
									map[string]interface{}{
										"users": map[string]interface{}{
											"users": []interface{}{},
										},
									},
								},
							},
						},
					},
				},
			},
		})
	})
	srv := httptest.NewServer(accountsHandler(emptyHandler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	user, err := client.GetUserByEmail(context.Background(), "domain-1", "nobody@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: unexpected error: %v", err)
	}
	if user != nil {
		t.Errorf("expected nil user for missing email, got %+v", user)
	}
}

// --- T5: DeleteUser treats NR not-found as success ---

func TestDeleteUser_NotFoundIsSuccess(t *testing.T) {
	notFoundHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":[{"message":"could not find the target or you are unauthorized.","extensions":{"errorClass":"NOT_FOUND"}}]}`))
	})
	srv := httptest.NewServer(accountsHandler(notFoundHandler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	if err := client.DeleteUser(context.Background(), "missing-id"); err != nil {
		t.Errorf("DeleteUser should return nil for not-found, got: %v", err)
	}
}

func TestDeleteUser_ForbiddenSurfacesAsError(t *testing.T) {
	// NerdGraph returns FORBIDDEN even when the message uses NR's ambiguous literal.
	// DeleteUser must surface this as an error — not silently succeed — so C1 does
	// not mark an account deprovisioned when it still has access.
	forbiddenHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":[{"message":"could not find the target or you are unauthorized.","extensions":{"errorClass":"FORBIDDEN"}}]}`))
	})
	srv := httptest.NewServer(accountsHandler(forbiddenHandler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	if err := client.DeleteUser(context.Background(), "user-1"); err == nil {
		t.Error("DeleteUser should return error for FORBIDDEN, got nil")
	}
}

// emptyUsersResponse is the body NerdGraph returns for a users(filter: {id: {eq: ...}})
// query that matches nobody: HTTP 200, empty users[] list, no errors.
const emptyUsersResponse = `{"data":{"actor":{"organization":{"userManagement":{"authenticationDomains":{"authenticationDomains":[{"users":{"users":[]}}]}}}}}}`

func userByIDResponse(id, email, name string) string {
	return fmt.Sprintf(
		`{"data":{"actor":{"organization":{"userManagement":{"authenticationDomains":{"authenticationDomains":[{"users":{"users":[{"id":%q,"email":%q,"name":%q}]}}]}}}}}}`,
		id, email, name)
}

// --- T5b: GetUserByID ---

func TestGetUserByID_Found(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(userByIDResponse("user-1", "user1@example.com", "User One")))
	})
	srv := httptest.NewServer(accountsHandler(handler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	user, err := client.GetUserByID(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if user == nil || user.ID != "user-1" {
		t.Errorf("expected user-1, got %+v", user)
	}
}

func TestGetUserByID_NotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(emptyUsersResponse))
	})
	srv := httptest.NewServer(accountsHandler(handler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	user, err := client.GetUserByID(context.Background(), "missing-id")
	if err != nil {
		t.Fatalf("GetUserByID: unexpected error: %v", err)
	}
	if user != nil {
		t.Errorf("expected nil user for missing id, got %+v", user)
	}
}

func TestGetUserByID_FollowsDomainCursor(t *testing.T) {
	// The user lives in the second page of authentication domains. A GetUserByID that
	// only reads the first page would wrongly report this user as not found.
	const wantID = "user-on-page-2"
	var requests int

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body GraphqlBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		requests++
		w.Header().Set("Content-Type", "application/json")

		if body.Variables["domainCursor"] == nil {
			_, _ = w.Write([]byte(`{"data":{"actor":{"organization":{"userManagement":{"authenticationDomains":{"nextCursor":"page-2","authenticationDomains":[{"users":{"users":[]}}]}}}}}}`))
			return
		}
		_, _ = w.Write([]byte(userByIDResponse(wantID, "user2@example.com", "User Two")))
	})
	srv := httptest.NewServer(accountsHandler(handler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	user, err := client.GetUserByID(context.Background(), wantID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if user == nil || user.ID != wantID {
		t.Errorf("expected %s from the second domain page, got %+v", wantID, user)
	}
	if requests != 2 {
		t.Errorf("expected GetUserByID to follow the domain cursor across 2 requests, got %d", requests)
	}
}

// --- T7: RemoveUserFromGroup treats not-found as success (idempotent revoke) ---
//
// RemoveUserFromGroup uses isNotFoundErrStrict (errorClass == NOT_FOUND only), not
// the loose isNotFoundErr message-substring fallback, so a permission-denied removal
// (FORBIDDEN, even carrying NR's ambiguous "could not find the target" message)
// surfaces as a real error rather than a silent success.

func TestRemoveUserFromGroup_NotFoundIsErrNotMember(t *testing.T) {
	notFoundHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":[{"message":"could not find the target or you are unauthorized.","extensions":{"errorClass":"NOT_FOUND"}}]}`))
	})
	srv := httptest.NewServer(accountsHandler(notFoundHandler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	err := client.RemoveUserFromGroup(context.Background(), "group-1", "missing-user")
	if !errors.Is(err, ErrNotMember) {
		t.Errorf("RemoveUserFromGroup should return ErrNotMember for not-found, got: %v", err)
	}
}

func TestRemoveUserFromGroup_OtherGraphQLErrorSurfaces(t *testing.T) {
	errHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":[{"message":"permission denied","extensions":{"errorClass":"FORBIDDEN"}}]}`))
	})
	srv := httptest.NewServer(accountsHandler(errHandler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	if err := client.RemoveUserFromGroup(context.Background(), "group-1", "user-1"); err == nil {
		t.Error("RemoveUserFromGroup should return error for non-not-found GraphQL errors, got nil")
	}
}

func TestRemoveUserFromGroup_ForbiddenWithAmbiguousMessageSurfacesAsError(t *testing.T) {
	// NerdGraph returns FORBIDDEN even when the message uses NR's ambiguous literal
	// "could not find the target or you are unauthorized." RemoveUserFromGroup must
	// surface this as an error — not silently succeed as ErrNotMember — so a
	// permission-denied removal is not reported as a completed revoke.
	forbiddenHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":[{"message":"could not find the target or you are unauthorized.","extensions":{"errorClass":"FORBIDDEN"}}]}`))
	})
	srv := httptest.NewServer(accountsHandler(forbiddenHandler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	err := client.RemoveUserFromGroup(context.Background(), "group-1", "user-1")
	if err == nil {
		t.Error("RemoveUserFromGroup should return error for FORBIDDEN, got nil")
	}
	if errors.Is(err, ErrNotMember) {
		t.Error("RemoveUserFromGroup should not classify FORBIDDEN as ErrNotMember")
	}
}

// --- Add*Role idempotency: already-granted surfaces as ErrRoleAlreadyAssigned ---
//
// Unlike group membership, role grant/revoke sentinels must survive the client
// layer (not collapse to a bare nil) so roleBuilder.Grant/Revoke in
// pkg/connector/roles.go can map them to the GrantAlreadyExists annotation,
// same pattern as groups.go's ErrAlreadyMember/ErrNotMember.

func TestAddGroupRole_AlreadyGrantedIsErrRoleAlreadyAssigned(t *testing.T) {
	alreadyGrantedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":[{"message":"role already added to group","extensions":{"errorClass":"DUPLICATE"}}]}`))
	})
	srv := httptest.NewServer(accountsHandler(alreadyGrantedHandler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	err := client.AddGroupRole(context.Background(), "role-1", "group-1")
	if !errors.Is(err, ErrRoleAlreadyAssigned) {
		t.Errorf("AddGroupRole should return ErrRoleAlreadyAssigned for already-granted role, got: %v", err)
	}
}

func TestAddAccountRole_AlreadyGrantedIsErrRoleAlreadyAssigned(t *testing.T) {
	alreadyGrantedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":[{"message":"role already added to group","extensions":{"errorClass":"DUPLICATE"}}]}`))
	})
	srv := httptest.NewServer(accountsHandler(alreadyGrantedHandler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	err := client.AddAccountRole(context.Background(), "role-1", "group-1", 1)
	if !errors.Is(err, ErrRoleAlreadyAssigned) {
		t.Errorf("AddAccountRole should return ErrRoleAlreadyAssigned for already-granted role, got: %v", err)
	}
}

func TestAddOrgRole_AlreadyGrantedIsErrRoleAlreadyAssigned(t *testing.T) {
	alreadyGrantedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":[{"message":"role already added to group","extensions":{"errorClass":"DUPLICATE"}}]}`))
	})
	srv := httptest.NewServer(accountsHandler(alreadyGrantedHandler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	err := client.AddOrgRole(context.Background(), "role-1", "group-1")
	if !errors.Is(err, ErrRoleAlreadyAssigned) {
		t.Errorf("AddOrgRole should return ErrRoleAlreadyAssigned for already-granted role, got: %v", err)
	}
}

func TestAddGroupRole_OtherGraphQLErrorSurfaces(t *testing.T) {
	errHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":[{"message":"permission denied","extensions":{"errorClass":"FORBIDDEN"}}]}`))
	})
	srv := httptest.NewServer(accountsHandler(errHandler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	if err := client.AddGroupRole(context.Background(), "role-1", "group-1"); err == nil {
		t.Error("AddGroupRole should return error for non-already-granted GraphQL errors, got nil")
	}
}

// --- Remove*Role idempotency: strict not-found surfaces as ErrRoleNotAssigned ---
//
// Remove*Role uses isNotFoundErrStrict (errorClass == NOT_FOUND only), not the
// loose isNotFoundErr message-substring fallback, so a permission-denied revoke
// (FORBIDDEN, even carrying NR's ambiguous "could not find the target" message)
// surfaces as a real error rather than a silent success.

func TestRemoveGroupRole_StrictNotFoundIsErrRoleNotAssigned(t *testing.T) {
	notFoundHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":[{"message":"could not find the target or you are unauthorized.","extensions":{"errorClass":"NOT_FOUND"}}]}`))
	})
	srv := httptest.NewServer(accountsHandler(notFoundHandler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	err := client.RemoveGroupRole(context.Background(), "role-1", "group-1")
	if !errors.Is(err, ErrRoleNotAssigned) {
		t.Errorf("RemoveGroupRole should return ErrRoleNotAssigned for NOT_FOUND, got: %v", err)
	}
}

func TestRemoveGroupRole_ForbiddenSurfacesAsError(t *testing.T) {
	forbiddenHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":[{"message":"could not find the target or you are unauthorized.","extensions":{"errorClass":"FORBIDDEN"}}]}`))
	})
	srv := httptest.NewServer(accountsHandler(forbiddenHandler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	err := client.RemoveGroupRole(context.Background(), "role-1", "group-1")
	if err == nil {
		t.Error("RemoveGroupRole should return error for FORBIDDEN, got nil")
	}
	if errors.Is(err, ErrRoleNotAssigned) {
		t.Error("RemoveGroupRole should not classify FORBIDDEN as ErrRoleNotAssigned")
	}
}

func TestRemoveAccountRole_StrictNotFoundIsErrRoleNotAssigned(t *testing.T) {
	notFoundHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":[{"message":"could not find the target or you are unauthorized.","extensions":{"errorClass":"NOT_FOUND"}}]}`))
	})
	srv := httptest.NewServer(accountsHandler(notFoundHandler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	err := client.RemoveAccountRole(context.Background(), "role-1", "group-1", 1)
	if !errors.Is(err, ErrRoleNotAssigned) {
		t.Errorf("RemoveAccountRole should return ErrRoleNotAssigned for NOT_FOUND, got: %v", err)
	}
}

func TestRemoveAccountRole_ForbiddenSurfacesAsError(t *testing.T) {
	forbiddenHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":[{"message":"could not find the target or you are unauthorized.","extensions":{"errorClass":"FORBIDDEN"}}]}`))
	})
	srv := httptest.NewServer(accountsHandler(forbiddenHandler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	err := client.RemoveAccountRole(context.Background(), "role-1", "group-1", 1)
	if err == nil {
		t.Error("RemoveAccountRole should return error for FORBIDDEN, got nil")
	}
}

func TestRemoveOrgRole_StrictNotFoundIsErrRoleNotAssigned(t *testing.T) {
	notFoundHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":[{"message":"could not find the target or you are unauthorized.","extensions":{"errorClass":"NOT_FOUND"}}]}`))
	})
	srv := httptest.NewServer(accountsHandler(notFoundHandler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	err := client.RemoveOrgRole(context.Background(), "role-1", "group-1")
	if !errors.Is(err, ErrRoleNotAssigned) {
		t.Errorf("RemoveOrgRole should return ErrRoleNotAssigned for NOT_FOUND, got: %v", err)
	}
}

func TestRemoveOrgRole_ForbiddenSurfacesAsError(t *testing.T) {
	forbiddenHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":[{"message":"could not find the target or you are unauthorized.","extensions":{"errorClass":"FORBIDDEN"}}]}`))
	})
	srv := httptest.NewServer(accountsHandler(forbiddenHandler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	err := client.RemoveOrgRole(context.Background(), "role-1", "group-1")
	if err == nil {
		t.Error("RemoveOrgRole should return error for FORBIDDEN, got nil")
	}
}

// --- ListUsers always returns v2 identity ids (no v1 fallback) ---

func TestListUsers_ReturnsV2IDs(t *testing.T) {
	const wantID = "v2-identity-xyz"
	queryHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"actor": map[string]interface{}{
					"organization": map[string]interface{}{
						"userManagement": map[string]interface{}{
							"authenticationDomains": map[string]interface{}{
								"authenticationDomains": []interface{}{
									map[string]interface{}{
										"users": map[string]interface{}{
											"users": []interface{}{
												map[string]interface{}{
													"id":    wantID,
													"email": "bob@example.com",
													"name":  "Bob",
												},
											},
											"nextCursor": nil,
										},
									},
								},
							},
						},
					},
				},
			},
		})
	})
	srv := httptest.NewServer(accountsHandler(queryHandler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	users, _, err := client.ListUsers(context.Background(), "", "")
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	if users[0].ID != wantID {
		t.Errorf("ID = %q, want %q (v2 identity id, not v1 userId)", users[0].ID, wantID)
	}
}

// --- ListUsers threads emailVerificationState through to the User struct ---

func TestListUsers_ThreadsEmailVerificationState(t *testing.T) {
	queryHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"actor": map[string]interface{}{
					"organization": map[string]interface{}{
						"userManagement": map[string]interface{}{
							"authenticationDomains": map[string]interface{}{
								"authenticationDomains": []interface{}{
									map[string]interface{}{
										"users": map[string]interface{}{
											"users": []interface{}{
												map[string]interface{}{
													"id":                     "v2-pending",
													"email":                  "pending@example.com",
													"name":                   "Pending",
													"emailVerificationState": "Pending",
												},
											},
											"nextCursor": nil,
										},
									},
								},
							},
						},
					},
				},
			},
		})
	})
	srv := httptest.NewServer(accountsHandler(queryHandler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	users, _, err := client.ListUsers(context.Background(), "", "")
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	if users[0].EmailVerificationState != "Pending" {
		t.Errorf("EmailVerificationState = %q, want %q", users[0].EmailVerificationState, "Pending")
	}
}

// --- CreateUser: duplicate email is classified as ErrUserAlreadyExists ---

func TestCreateUser_DuplicateEmailReturnsErrUserAlreadyExists(t *testing.T) {
	mutHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// NerdGraph rejects a duplicate email with HTTP 200 + a top-level errors array.
		_, _ = w.Write([]byte(`{"errors":[{"message":"user with email bob@example.com already exists"}]}`))
	})
	srv := httptest.NewServer(accountsHandler(mutHandler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.CreateUser(context.Background(), "domain-1", "bob@example.com", "Bob", "FULL_USER_TIER")
	if !errors.Is(err, ErrUserAlreadyExists) {
		t.Fatalf("expected ErrUserAlreadyExists, got %v", err)
	}
}

func TestCreateUser_OtherGraphQLErrorSurfaces(t *testing.T) {
	mutHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":[{"message":"invalid user type","extensions":{"errorClass":"BAD_USER_INPUT"}}]}`))
	})
	srv := httptest.NewServer(accountsHandler(mutHandler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.CreateUser(context.Background(), "domain-1", "bob@example.com", "Bob", "NOPE")
	if err == nil {
		t.Fatal("expected error for non-duplicate GraphQL error")
	}
	if errors.Is(err, ErrUserAlreadyExists) {
		t.Fatal("non-duplicate error must not be classified as ErrUserAlreadyExists")
	}
}

// --- userType enum name matches the live NerdGraph schema ---
//
// The mock server does not validate GraphQL types, so a wrong enum name passes
// every other test here and only fails against the real API with
// `Unknown type "..."`. NerdGraph names this enum UserManagementRequestedTierName;
// UserManagementRequestedTier does not exist.

func TestUserMutations_UseSchemaUserTypeEnum(t *testing.T) {
	const want = "UserManagementRequestedTierName"

	mutations := map[string]string{
		"CreateUser": composeCreateUserMutation(),
		"UpdateUser": composeUpdateUserMutation([]string{"userType"}),
	}

	for name, mutation := range mutations {
		if !strings.Contains(mutation, want) {
			t.Errorf("%s mutation must declare $userType as %s, got:\n%s", name, want, mutation)
		}
		// Any remaining mention after stripping the valid name is the bad name.
		if strings.Contains(strings.ReplaceAll(mutation, want, ""), "UserManagementRequestedTier") {
			t.Errorf("%s mutation declares nonexistent type UserManagementRequestedTier, got:\n%s", name, mutation)
		}
	}
}

// TestGetUserByEmailQuery_UsesFilterArgument guards against regressing to the
// nonexistent "search" argument on UserManagementAuthenticationDomain.users.
// Real NerdGraph schema only exposes filter/cursor/id/sort on that field;
// introspection confirms UserManagementUserFilterInput.email is a
// UserManagementEmailInput with eq/contains string fields.
func TestGetUserByEmailQuery_UsesFilterArgument(t *testing.T) {
	query := composeGetUserByEmailQuery()

	const want = "filter: {email: {eq: $email}}"
	if !strings.Contains(query, want) {
		t.Errorf("GetUserByEmail query must use %q, got:\n%s", want, query)
	}
	if strings.Contains(query, "search:") {
		t.Errorf("GetUserByEmail query must not use nonexistent \"search\" argument, got:\n%s", query)
	}
}

// TestGetUserByEmailQuery_ScopedToDomain guards against regressing to an
// unfiltered scan across every authentication domain in the org — GetUserByEmail
// must search only within the domain CreateAccount is provisioning into, the
// same pattern used by usersQueryV2 and groupMembersQuery.
func TestGetUserByEmailQuery_ScopedToDomain(t *testing.T) {
	query := composeGetUserByEmailQuery()

	const wantArg = "authenticationDomains(id: $domainId)"
	if !strings.Contains(query, wantArg) {
		t.Errorf("GetUserByEmail query must scope authenticationDomains with %q, got:\n%s", wantArg, query)
	}
	const wantDecl = "$domainId: [ID!]"
	if !strings.Contains(query, wantDecl) {
		t.Errorf("GetUserByEmail query must declare %q, got:\n%s", wantDecl, query)
	}
}

func TestIsAlreadyExistsErr(t *testing.T) {
	yes := []GraphqlError{
		{Message: "email already exists"},
		{Message: "that address is already registered"},
		{Message: "duplicate key"},
		{Message: "whatever", Extensions: map[string]interface{}{"errorClass": "DUPLICATE"}},
		{Message: "whatever", Extensions: map[string]interface{}{"errorClass": "ALREADY_EXISTS"}},
	}
	for _, e := range yes {
		if !isAlreadyExistsErr(e) {
			t.Errorf("expected isAlreadyExistsErr true for %+v", e)
		}
	}
	no := GraphqlError{Message: "permission denied", Extensions: map[string]interface{}{"errorClass": "FORBIDDEN"}}
	if isAlreadyExistsErr(no) {
		t.Errorf("expected isAlreadyExistsErr false for %+v", no)
	}
}

// --- ListGroupMembers reads member ids from the userManagement API ---
//
// Grants() in groups.go uses these ids as grant-principal resource ids, so they
// must come from the same API as ListUsers for principals to resolve to user
// resources.

func TestListGroupMembers_ReturnsV2IDs(t *testing.T) {
	const wantUserID = "v2-identity-abc"

	memberHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"actor": map[string]interface{}{
					"organization": map[string]interface{}{
						"userManagement": map[string]interface{}{
							"authenticationDomains": map[string]interface{}{
								"authenticationDomains": []interface{}{
									map[string]interface{}{
										"id":   "dom-1",
										"name": "Domain 1",
										"groups": map[string]interface{}{
											"nextCursor": nil,
											"totalCount": 1,
											"groups": []interface{}{
												map[string]interface{}{
													"id":          "grp-1",
													"displayName": "Group 1",
													"users": map[string]interface{}{
														"nextCursor": nil,
														"totalCount": 1,
														"users": []interface{}{
															// id field is the v2 identity id — same namespace
															// as what ListUsers returns via usersQueryV2.
															map[string]interface{}{"id": wantUserID},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		})
	})
	srv := httptest.NewServer(accountsHandler(memberHandler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	members, _, err := client.ListGroupMembers(context.Background(), "dom-1", "grp-1", "")
	if err != nil {
		t.Fatalf("ListGroupMembers: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(members))
	}
	if members[0] != wantUserID {
		t.Errorf("member ID = %q, want %q (v2 identity id, same namespace as ListUsers)", members[0], wantUserID)
	}
}

// --- ListAllDomains follows the domain cursor across pages ---

func TestListAllDomains_FollowsCursor(t *testing.T) {
	page := 0
	domHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if page == 0 {
			page++
			_, _ = w.Write([]byte(`{"data":{"actor":{"organization":{"authorizationManagement":` +
				`{"authenticationDomains":{"nextCursor":"c2","totalCount":2,` +
				`"authenticationDomains":[{"id":"d1","name":"D1","groups":{"totalCount":0}}]}}}}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"actor":{"organization":{"authorizationManagement":` +
			`{"authenticationDomains":{"nextCursor":"","totalCount":2,` +
			`"authenticationDomains":[{"id":"d2","name":"D2","groups":{"totalCount":0}}]}}}}}}`))
	})
	srv := httptest.NewServer(accountsHandler(domHandler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	domains, err := client.ListAllDomains(context.Background())
	if err != nil {
		t.Fatalf("ListAllDomains: %v", err)
	}
	if len(domains) != 2 {
		t.Fatalf("expected 2 domains across 2 pages, got %d", len(domains))
	}
	if domains[0].ID != "d1" || domains[1].ID != "d2" {
		t.Errorf("unexpected domain ids: %q, %q", domains[0].ID, domains[1].ID)
	}
}
