package connector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/conductorone/baton-newrelic/pkg/newrelic"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	rs "github.com/conductorone/baton-sdk/pkg/types/resource"
)

func newTestGroupBuilder(t *testing.T, serverURL string) *groupBuilder {
	t.Helper()
	client, err := newrelic.NewClient(context.Background(), http.DefaultClient, "test-key", serverURL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return newGroupBuilder(client)
}

// TestGroupList_NoGroupsAnywhereTerminatesPagination guards against a regression
// where, once every authentication domain reports zero groups, the pagination Bag
// ends up empty (no domain-continuation phase, no group phase) but NextToken("")
// still seeds a non-nil empty page state — returning a non-empty NextPageToken
// instead of "". The SDK would then call List again with that token, which
// matches neither switch case below and fails with "invalid resource type: ".
func TestGroupList_NoGroupsAnywhereTerminatesPagination(t *testing.T) {
	domainsHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"actor":{"organization":{"authorizationManagement":` +
			`{"authenticationDomains":{"nextCursor":"","totalCount":1,` +
			`"authenticationDomains":[{"id":"d1","name":"D1","groups":{"totalCount":0}}]}}}}}}`))
	})
	srv := httptest.NewServer(accountsHandler(domainsHandler))
	defer srv.Close()

	gb := newTestGroupBuilder(t, srv.URL)
	parentID := &v2.ResourceId{ResourceType: orgResourceType.Id, Resource: "org-1"}

	_, results, err := gb.List(context.Background(), parentID, rs.SyncOpAttrs{})
	if err != nil {
		t.Fatalf("List: unexpected error: %v", err)
	}
	if results.NextPageToken != "" {
		t.Errorf("NextPageToken = %q, want empty string to terminate pagination when no domain has any groups", results.NextPageToken)
	}
}
