//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/google/uuid"

	orgapp "billing-platform/internal/modules/organisation/app"
	orgpg "billing-platform/internal/modules/organisation/pg"
	"billing-platform/internal/platform/audit"
	"billing-platform/internal/platform/permissions"
)

// noopAuditRecorder discards audit entries — used where a test doesn't
// care about audit output. Tests that DO care (see audit_test.go) supply
// audit.NewPGRecorder(sharedPool) instead and query audit_log directly.
type noopAuditRecorder struct{}

func (noopAuditRecorder) Record(ctx context.Context, e audit.Entry) error { return nil }

func newTestOrgService(t *testing.T) *orgapp.Service {
	t.Helper()
	return orgapp.NewService(
		sharedPool,
		orgpg.NewOrganisationRepo(sharedPool),
		orgpg.NewLegalEntityRepo(sharedPool),
		orgpg.NewBranchRepo(sharedPool),
		orgpg.NewWarehouseRepo(sharedPool),
		permissions.NewChecker(permissions.NewPGStore(sharedPool), sharedPool),
		noopAuditRecorder{},
	)
}

// createTestOrganisation provisions a full organisation (org + legal
// entity + branch + warehouse) via the same Provision path production
// bootstrap uses, and returns the full result so tests can reference any
// level of the hierarchy.
func createTestOrganisation(t *testing.T, ctx context.Context, name string) orgapp.ProvisionResult {
	t.Helper()
	svc := newTestOrgService(t)
	unique := uuid.NewString()[:8]
	result, err := svc.Provision(ctx, orgapp.ProvisionParams{
		OrganisationName:    name,
		DefaultCurrencyCode: "INR",
		DefaultTimezone:     "Asia/Kolkata",
		LegalEntityName:     name + " Pvt Ltd",
		CountryCode:         "IN",
		BranchCode:          "BR-" + unique,
		BranchName:          name + " Main Branch",
		WarehouseCode:       "WH-" + unique,
		WarehouseName:       name + " Main Warehouse",
	})
	if err != nil {
		t.Fatalf("provisioning test organisation: %v", err)
	}
	return result
}
