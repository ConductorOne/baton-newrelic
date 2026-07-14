package newrelic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		// no-op'd before the doRequest strict-error check was in place.
		_, _ = w.Write([]byte(`{"errors":[{"message":"user already in group","extensions":{"errorClass":"BAD_USER_INPUT"}}]}`))
	})
	srv := httptest.NewServer(accountsHandler(mutHandler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	if err := client.AddUserToGroup(context.Background(), "group-1", "user-1"); err == nil {
		t.Fatal("expected error from AddUserToGroup when NerdGraph returns HTTP 200 + errors array, got nil")
	}
}

// --- T6: GetUserByEmail returns v2 identity id via filtered query ---

func TestGetUserByEmail_ReturnsV2ID(t *testing.T) {
	const wantID = "v2-identity-id-abc"
	const wantEmail = "alice@example.com"

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

	user, err := client.GetUserByEmail(context.Background(), wantEmail)
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

	user, err := client.GetUserByEmail(context.Background(), "nobody@example.com")
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

// --- T7: RemoveUserFromGroup treats not-found as success (idempotent revoke) ---

func TestRemoveUserFromGroup_NotFoundIsSuccess(t *testing.T) {
	notFoundHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":[{"message":"could not find the target or you are unauthorized.","extensions":{"errorClass":"NOT_FOUND"}}]}`))
	})
	srv := httptest.NewServer(accountsHandler(notFoundHandler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	if err := client.RemoveUserFromGroup(context.Background(), "group-1", "missing-user"); err != nil {
		t.Errorf("RemoveUserFromGroup should return nil for not-found, got: %v", err)
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

// --- Add*Role idempotency: already-granted treated as success ---

func TestAddGroupRole_AlreadyGrantedIsSuccess(t *testing.T) {
	// NerdGraph returns HTTP 200 with an errors array carrying an already-granted signal.
	// AddGroupRole must treat this as success (nil error) for idempotency.
	alreadyGrantedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":[{"message":"role already added to group","extensions":{"errorClass":"DUPLICATE"}}]}`))
	})
	srv := httptest.NewServer(accountsHandler(alreadyGrantedHandler))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	if err := client.AddGroupRole(context.Background(), "role-1", "group-1"); err != nil {
		t.Errorf("AddGroupRole should return nil for already-granted role, got: %v", err)
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

