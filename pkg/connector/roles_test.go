package connector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/conductorone/baton-newrelic/pkg/newrelic"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	ent "github.com/conductorone/baton-sdk/pkg/types/entitlement"
	"github.com/conductorone/baton-sdk/pkg/types/grant"
)

// accountsHandler satisfies the GetAccountId call NewClient makes on construction,
// then delegates all subsequent requests to next. Mirrors pkg/newrelic's test helper.
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

func newTestRoleBuilder(t *testing.T, serverURL string) *roleBuilder {
	t.Helper()
	client, err := newrelic.NewClient(context.Background(), http.DefaultClient, "test-key", serverURL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return newRoleBuilder(client)
}

// groupScopedGrantEntitlement builds the (principal, entitlement) pair Grant/Revoke
// expect for a group-scoped role: a group resource as principal and a role resource
// (with role_scope=group) as the entitlement's target.
func groupScopedGrantEntitlement(t *testing.T) (*v2.Resource, *v2.Entitlement) {
	t.Helper()

	principal, err := groupResource(context.Background(), nil, "domain-1", &newrelic.Group{
		BaseResource: newrelic.BaseResource{ID: "group-1"},
		Name:         "Admins",
	})
	if err != nil {
		t.Fatalf("groupResource: %v", err)
	}

	roleRes, err := roleResource(context.Background(), nil, &newrelic.Role{
		BaseResource: newrelic.BaseResource{ID: "role-1"},
		DisplayName:  "Manager",
		Name:         "member",
		Scope:        groupScope,
	})
	if err != nil {
		t.Fatalf("roleResource: %v", err)
	}

	entitlement := ent.NewAssignmentEntitlement(roleRes, "member")
	return principal, entitlement
}

func TestRoleGrant_AlreadyAssignedReturnsGrantAlreadyExists(t *testing.T) {
	alreadyGrantedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":[{"message":"role already added to group","extensions":{"errorClass":"DUPLICATE"}}]}`))
	})
	srv := httptest.NewServer(accountsHandler(alreadyGrantedHandler))
	defer srv.Close()

	rb := newTestRoleBuilder(t, srv.URL)
	principal, entitlement := groupScopedGrantEntitlement(t)

	annos, err := rb.Grant(context.Background(), principal, entitlement)
	if err != nil {
		t.Fatalf("Grant: unexpected error: %v", err)
	}
	if !annos.Contains(&v2.GrantAlreadyExists{}) {
		t.Errorf("expected GrantAlreadyExists annotation, got: %+v", annos)
	}
}

func TestRoleGrant_OtherErrorSurfaces(t *testing.T) {
	errHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":[{"message":"permission denied","extensions":{"errorClass":"FORBIDDEN"}}]}`))
	})
	srv := httptest.NewServer(accountsHandler(errHandler))
	defer srv.Close()

	rb := newTestRoleBuilder(t, srv.URL)
	principal, entitlement := groupScopedGrantEntitlement(t)

	if _, err := rb.Grant(context.Background(), principal, entitlement); err == nil {
		t.Error("expected error for non-already-assigned GraphQL error, got nil")
	}
}

func TestRoleRevoke_StrictNotFoundReturnsGrantAlreadyRevoked(t *testing.T) {
	notFoundHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":[{"message":"could not find the target or you are unauthorized.","extensions":{"errorClass":"NOT_FOUND"}}]}`))
	})
	srv := httptest.NewServer(accountsHandler(notFoundHandler))
	defer srv.Close()

	rb := newTestRoleBuilder(t, srv.URL)
	principal, entitlement := groupScopedGrantEntitlement(t)
	g := grant.NewGrant(entitlement.Resource, "member", principal.Id)

	annos, err := rb.Revoke(context.Background(), g)
	if err != nil {
		t.Fatalf("Revoke: unexpected error: %v", err)
	}
	if !annos.Contains(&v2.GrantAlreadyRevoked{}) {
		t.Errorf("expected GrantAlreadyRevoked annotation, got: %+v", annos)
	}
}

func TestRoleRevoke_ForbiddenSurfacesAsError(t *testing.T) {
	forbiddenHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errors":[{"message":"could not find the target or you are unauthorized.","extensions":{"errorClass":"FORBIDDEN"}}]}`))
	})
	srv := httptest.NewServer(accountsHandler(forbiddenHandler))
	defer srv.Close()

	rb := newTestRoleBuilder(t, srv.URL)
	principal, entitlement := groupScopedGrantEntitlement(t)
	g := grant.NewGrant(entitlement.Resource, "member", principal.Id)

	if _, err := rb.Revoke(context.Background(), g); err == nil {
		t.Error("Revoke should surface FORBIDDEN as an error, not silent success")
	}
}
