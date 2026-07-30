package connector

import (
	"context"
	"testing"

	"github.com/conductorone/baton-newrelic/pkg/newrelic"
	v2 "github.com/conductorone/baton-sdk/pb/c1/connector/v2"
	rs "github.com/conductorone/baton-sdk/pkg/types/resource"
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

	trait, err := rs.GetUserTrait(r)
	if err != nil {
		t.Fatalf("GetUserTrait: %v", err)
	}
	if trait.GetStatus().GetStatus() != v2.UserTrait_Status_STATUS_DISABLED {
		t.Errorf("status = %v, want STATUS_DISABLED for Pending user", trait.GetStatus().GetStatus())
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

	trait, err := rs.GetUserTrait(r)
	if err != nil {
		t.Fatalf("GetUserTrait: %v", err)
	}
	if trait.GetStatus().GetStatus() != v2.UserTrait_Status_STATUS_ENABLED {
		t.Errorf("status = %v, want STATUS_ENABLED for Verified user", trait.GetStatus().GetStatus())
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

	trait, err := rs.GetUserTrait(r)
	if err != nil {
		t.Fatalf("GetUserTrait: %v", err)
	}
	if trait.GetStatus().GetStatus() != v2.UserTrait_Status_STATUS_ENABLED {
		t.Errorf("status = %v, want STATUS_ENABLED for Not Verifiable user", trait.GetStatus().GetStatus())
	}
}
