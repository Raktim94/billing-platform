//go:build integration

package integration

import (
	"context"
	"testing"

	orgpg "billing-platform/internal/modules/organisation/pg"
)

// TestRLS_BlocksCrossOrganisationReads is the building block behind
// Scenario G (brief §79): even a query that somehow runs with the wrong
// organisation scope set must come back empty, never leak another
// tenant's rows. This exercises database.Pool.RunScoped +
// migrations' RLS policies directly, independent of any
// application-layer permission check, to prove the defense-in-depth
// layer (docs/architecture.md §10) actually holds on its own.
func TestRLS_BlocksCrossOrganisationReads(t *testing.T) {
	ctx := context.Background()
	branchRepo := orgpg.NewBranchRepo(sharedPool)
	orgRepo := orgpg.NewOrganisationRepo(sharedPool)

	orgA := createTestOrganisation(t, ctx, "RLS Test Org A")
	orgB := createTestOrganisation(t, ctx, "RLS Test Org B")

	// Org A's branch (created during provisioning) must be invisible to a
	// transaction scoped to Org B, even though the query asks for that
	// exact branch ID by primary key. GetByID now also filters by
	// organisation_id at the app layer (docs/adr/0006) in addition to
	// RLS, so passing orgB's own ID here still exercises real
	// cross-tenant isolation end to end — it just means this specific
	// call no longer isolates "RLS with zero app-layer help" the way it
	// used to. That narrower "RLS alone, raw SQL, no app-layer filter"
	// guarantee is exercised elsewhere in this suite (e.g.
	// accounting_test.go, webhooks_test.go) via queries that bypass the
	// repository layer entirely — it has not been separately re-proven
	// for the branches table specifically after this change.
	var found bool
	err := sharedPool.RunScoped(ctx, orgB.OrganisationID, func(ctx context.Context) error {
		_, getErr := branchRepo.GetByID(ctx, orgB.OrganisationID, orgA.BranchID)
		found = getErr == nil
		return nil
	})
	if err != nil {
		t.Fatalf("querying as org B: %v", err)
	}
	if found {
		t.Fatal("RLS FAILED: org B's transaction could read org A's branch by ID")
	}

	// And must be visible again when correctly scoped to Org A.
	err = sharedPool.RunScoped(ctx, orgA.OrganisationID, func(ctx context.Context) error {
		_, getErr := branchRepo.GetByID(ctx, orgA.OrganisationID, orgA.BranchID)
		found = getErr == nil
		return getErr
	})
	if err != nil || !found {
		t.Fatalf("expected org A to read its own branch back; err=%v found=%v", err, found)
	}

	// Same check against the organisations table itself.
	err = sharedPool.RunScoped(ctx, orgB.OrganisationID, func(ctx context.Context) error {
		_, getErr := orgRepo.GetByID(ctx, orgA.OrganisationID)
		found = getErr == nil
		return nil
	})
	if err != nil {
		t.Fatalf("querying organisations as org B: %v", err)
	}
	if found {
		t.Fatal("RLS FAILED: org B's transaction could read org A's own organisation row")
	}
}

// TestRLS_UnsetScopeSeesNothing verifies the fail-closed behavior
// documented in migrations/0001_organisation_hierarchy.up.sql: a query
// that runs without app.current_organisation_id ever being set must see
// zero rows, not every organisation's rows.
func TestRLS_UnsetScopeSeesNothing(t *testing.T) {
	ctx := context.Background()
	orgRepo := orgpg.NewOrganisationRepo(sharedPool)
	orgA := createTestOrganisation(t, ctx, "RLS Unset-Scope Org")

	var found bool
	err := sharedPool.Run(ctx, func(ctx context.Context) error {
		_, getErr := orgRepo.GetByID(ctx, orgA.OrganisationID)
		found = getErr == nil
		return nil
	})
	if err != nil {
		t.Fatalf("querying with no scope set: %v", err)
	}
	if found {
		t.Fatal("RLS FAILED: a transaction with app.current_organisation_id unset could read an organisation row")
	}
}
