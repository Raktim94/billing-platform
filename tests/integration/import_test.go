//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/google/uuid"

	catalogueapp "rechvix/internal/modules/catalogue/app"
	cataloguepg "rechvix/internal/modules/catalogue/pg"
	contactsapp "rechvix/internal/modules/contacts/app"
	contactsdomain "rechvix/internal/modules/contacts/domain"
	contactspg "rechvix/internal/modules/contacts/pg"
	"rechvix/internal/platform/audit"
	"rechvix/internal/platform/importer"
	"rechvix/internal/platform/permissions"
)

func newTestCatalogueServiceForImport(t *testing.T) *catalogueapp.Service {
	t.Helper()
	return catalogueapp.NewService(
		sharedPool,
		cataloguepg.NewUnitOfMeasureRepo(sharedPool),
		cataloguepg.NewUnitConversionRepo(sharedPool),
		cataloguepg.NewCategoryRepo(sharedPool),
		cataloguepg.NewBrandRepo(sharedPool),
		cataloguepg.NewProductRepo(sharedPool),
		cataloguepg.NewProductVariantRepo(sharedPool),
		cataloguepg.NewBarcodeRepo(sharedPool),
		permissions.NewChecker(permissions.NewPGStore(sharedPool), sharedPool),
		audit.NewPGRecorder(sharedPool),
	)
}

func TestCatalogue_ImportProducts_ValidatesDedupesAndCommits(t *testing.T) {
	ctx := context.Background()
	svc := newTestCatalogueServiceForImport(t)
	principal := bootstrapOwnerPrincipal(t, ctx)

	if _, err := svc.CreateUnitOfMeasure(ctx, principal, catalogueapp.CreateUnitOfMeasureParams{Code: "PCS", Name: "Pieces"}); err != nil {
		t.Fatalf("CreateUnitOfMeasure: %v", err)
	}
	existingName := "PreExisting Widget " + uuid.NewString()[:8]
	if _, err := svc.CreateProduct(ctx, principal, catalogueapp.CreateProductParams{
		Name: existingName, BaseUOMID: mustGetUnitID(t, ctx, svc, principal, "PCS"),
	}); err != nil {
		t.Fatalf("CreateProduct (pre-existing): %v", err)
	}

	newName := "Imported Widget " + uuid.NewString()[:8]
	rows := []importer.Row{
		{Number: 1, Fields: map[string]string{"name": newName, "hsn_sac_code": "8471", "base_uom_code": "PCS"}}, // valid, new
		{Number: 2, Fields: map[string]string{"name": existingName, "base_uom_code": "PCS"}},                    // duplicate
		{Number: 3, Fields: map[string]string{"name": "", "base_uom_code": "PCS"}},                              // missing name
		{Number: 4, Fields: map[string]string{"name": "Bad Unit Widget", "base_uom_code": "NOSUCHUNIT"}},        // bad unit
	}

	// Dry run first: nothing committed, but the report must already show
	// the correct outcome per row.
	dryReport, err := svc.ImportProducts(ctx, principal, rows, true)
	if err != nil {
		t.Fatalf("ImportProducts(dryRun): %v", err)
	}
	if dryReport.Committed != 0 {
		t.Fatalf("dry run Committed = %d, want 0", dryReport.Committed)
	}
	if dryReport.Valid != 1 || dryReport.Duplicates != 1 || dryReport.Errors != 2 {
		t.Fatalf("dry run counts = %+v, want Valid=1 Duplicates=1 Errors=2", dryReport)
	}

	list, err := svc.ListProducts(ctx, principal)
	if err != nil {
		t.Fatalf("ListProducts after dry run: %v", err)
	}
	for _, p := range list {
		if p.Name == newName {
			t.Fatalf("dry run must not have committed %q, but it exists", newName)
		}
	}

	// Real run: same rows, dryRun=false.
	report, err := svc.ImportProducts(ctx, principal, rows, false)
	if err != nil {
		t.Fatalf("ImportProducts: %v", err)
	}
	if report.Committed != 1 || report.Duplicates != 1 || report.Errors != 2 {
		t.Fatalf("real run counts = %+v, want Committed=1 Duplicates=1 Errors=2", report)
	}

	list, err = svc.ListProducts(ctx, principal)
	if err != nil {
		t.Fatalf("ListProducts: %v", err)
	}
	found := false
	for _, p := range list {
		if p.Name == newName {
			found = true
		}
	}
	if !found {
		t.Fatalf("imported product %q not found after commit", newName)
	}
}

func TestContacts_ImportParties_ValidatesDedupesAndCommits(t *testing.T) {
	ctx := context.Background()
	svc := contactsapp.NewService(
		sharedPool,
		contactspg.NewPartyRepo(sharedPool),
		contactspg.NewAddressRepo(sharedPool),
		contactspg.NewTaxRegistrationRepo(sharedPool),
		permissions.NewChecker(permissions.NewPGStore(sharedPool), sharedPool),
		audit.NewPGRecorder(sharedPool),
	)
	principal := bootstrapOwnerPrincipal(t, ctx)

	existingName := "PreExisting Traders " + uuid.NewString()[:8]
	if _, err := svc.CreateParty(ctx, principal, contactsapp.CreatePartyParams{
		PartyType: contactsdomain.PartyCustomer, LegalName: existingName, CurrencyCode: "INR",
	}); err != nil {
		t.Fatalf("CreateParty (pre-existing): %v", err)
	}

	newName := "Imported Traders " + uuid.NewString()[:8]
	rows := []importer.Row{
		{Number: 1, Fields: map[string]string{"party_type": "SUPPLIER", "legal_name": newName, "currency_code": "INR"}},      // valid, new
		{Number: 2, Fields: map[string]string{"party_type": "CUSTOMER", "legal_name": existingName, "currency_code": "INR"}}, // duplicate (same type+name)
		{Number: 3, Fields: map[string]string{"party_type": "NOT_A_TYPE", "legal_name": "Whatever", "currency_code": "INR"}}, // bad party_type
		{Number: 4, Fields: map[string]string{"party_type": "CUSTOMER", "legal_name": "", "currency_code": "INR"}},           // missing name
	}

	report, err := svc.ImportParties(ctx, principal, rows, false)
	if err != nil {
		t.Fatalf("ImportParties: %v", err)
	}
	if report.Committed != 1 || report.Duplicates != 1 || report.Errors != 2 {
		t.Fatalf("counts = %+v, want Committed=1 Duplicates=1 Errors=2", report)
	}

	list, err := svc.ListParties(ctx, principal)
	if err != nil {
		t.Fatalf("ListParties: %v", err)
	}
	found := false
	for _, p := range list {
		if p.LegalName == newName {
			found = true
		}
	}
	if !found {
		t.Fatalf("imported party %q not found after commit", newName)
	}
}

func mustGetUnitID(t *testing.T, ctx context.Context, svc *catalogueapp.Service, principal permissions.Principal, code string) uuid.UUID {
	t.Helper()
	units, err := svc.ListUnitsOfMeasure(ctx, principal)
	if err != nil {
		t.Fatalf("ListUnitsOfMeasure: %v", err)
	}
	for _, u := range units {
		if u.Code == code {
			return u.ID
		}
	}
	t.Fatalf("unit %q not found", code)
	return uuid.UUID{}
}
