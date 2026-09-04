//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	catalogueapp "rechvix/internal/modules/catalogue/app"
	cataloguedomain "rechvix/internal/modules/catalogue/domain"
	cataloguepg "rechvix/internal/modules/catalogue/pg"
	"rechvix/internal/platform/audit"
	"rechvix/internal/platform/permissions"
)

func newTestCatalogueService(t *testing.T) *catalogueapp.Service {
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

// bootstrapOwnerPrincipal provisions a fresh tenant via the real
// identity.Bootstrap flow (not a shortcut) and returns an authenticated
// owner Principal for it — the owner role holds every permission by
// bootstrap design, so tests using this can exercise a module's real
// RBAC-checked application layer, not a permission-bypassing fake.
func bootstrapOwnerPrincipal(t *testing.T, ctx context.Context) permissions.Principal {
	t.Helper()
	identitySvc, _ := newTestIdentityService(t)
	email := "catalogue-" + uuid.NewString()[:8] + "@example.com"
	password := "correct horse battery staple 42"
	boot := bootstrapTestTenant(t, ctx, identitySvc, email, password)
	return permissions.Principal{UserID: boot.OwnerUserID, OrganisationID: boot.OrganisationID}
}

func TestCatalogue_UnitConversion_And_ProductVariant_CRUD(t *testing.T) {
	ctx := context.Background()
	svc := newTestCatalogueService(t)
	principal := bootstrapOwnerPrincipal(t, ctx)

	box, err := svc.CreateUnitOfMeasure(ctx, principal, catalogueapp.CreateUnitOfMeasureParams{Code: "BOX", Name: "Box"})
	if err != nil {
		t.Fatalf("CreateUnitOfMeasure(BOX): %v", err)
	}
	pcs, err := svc.CreateUnitOfMeasure(ctx, principal, catalogueapp.CreateUnitOfMeasureParams{Code: "PCS", Name: "Pieces"})
	if err != nil {
		t.Fatalf("CreateUnitOfMeasure(PCS): %v", err)
	}

	conv, err := svc.CreateUnitConversion(ctx, principal, catalogueapp.CreateUnitConversionParams{
		FromUnitID: box.ID, ToUnitID: pcs.ID, Factor: mustDecimal(t, "25"),
	})
	if err != nil {
		t.Fatalf("CreateUnitConversion: %v", err)
	}
	if !conv.Factor.Equal(mustDecimal(t, "25")) {
		t.Fatalf("stored factor = %s, want 25", conv.Factor)
	}

	product, err := svc.CreateProduct(ctx, principal, catalogueapp.CreateProductParams{
		BaseUOMID: pcs.ID, Name: "Integration Test Widget", HSNSACCode: "8471",
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if product.HSNSACCode != "8471" {
		t.Fatalf("HSNSACCode = %q, want 8471", product.HSNSACCode)
	}

	variant, err := svc.CreateVariant(ctx, principal, catalogueapp.CreateVariantParams{
		ProductID: product.ID, SKUCode: "WIDGET-" + uuid.NewString()[:8], Attributes: map[string]any{"colour": "blue"},
	})
	if err != nil {
		t.Fatalf("CreateVariant: %v", err)
	}
	if variant.Attributes["colour"] != "blue" {
		t.Fatalf("variant attributes did not round-trip through jsonb: %+v", variant.Attributes)
	}

	barcode, err := svc.AddBarcode(ctx, principal, catalogueapp.AddBarcodeParams{
		VariantID: variant.ID, UnitID: pcs.ID, Barcode: "890" + uuid.NewString()[:10],
	})
	if err != nil {
		t.Fatalf("AddBarcode: %v", err)
	}

	looked, err := svc.LookupBarcode(ctx, principal, barcode.Barcode)
	if err != nil {
		t.Fatalf("LookupBarcode: %v", err)
	}
	if looked.VariantID != variant.ID {
		t.Fatalf("LookupBarcode returned variant %s, want %s", looked.VariantID, variant.ID)
	}
}

func TestCatalogue_SearchByName(t *testing.T) {
	ctx := context.Background()
	svc := newTestCatalogueService(t)
	principal := bootstrapOwnerPrincipal(t, ctx)

	pcs, err := svc.CreateUnitOfMeasure(ctx, principal, catalogueapp.CreateUnitOfMeasureParams{Code: "PCS", Name: "Pieces"})
	if err != nil {
		t.Fatalf("CreateUnitOfMeasure: %v", err)
	}

	uniqueName := "SearchableWidget" + uuid.NewString()[:8]
	if _, err := svc.CreateProduct(ctx, principal, catalogueapp.CreateProductParams{BaseUOMID: pcs.ID, Name: uniqueName}); err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	results, err := svc.SearchProducts(ctx, principal, uniqueName, 10)
	if err != nil {
		t.Fatalf("SearchProducts: %v", err)
	}
	found := false
	for _, p := range results {
		if p.Name == uniqueName {
			found = true
		}
	}
	if !found {
		t.Fatalf("SearchProducts(%q) did not return the matching product; got %d results", uniqueName, len(results))
	}
}

// TestCatalogue_RLS_BlocksCrossOrganisationProductRead is the catalogue
// module's building block for Scenario G, exercised through the real
// application layer (not raw RunScoped like rls_test.go's generic check)
// — Organisation B's principal must not be able to read Organisation A's
// product even by its exact primary key.
func TestCatalogue_RLS_BlocksCrossOrganisationProductRead(t *testing.T) {
	ctx := context.Background()
	svc := newTestCatalogueService(t)
	principalA := bootstrapOwnerPrincipal(t, ctx)
	principalB := bootstrapOwnerPrincipal(t, ctx)

	pcs, err := svc.CreateUnitOfMeasure(ctx, principalA, catalogueapp.CreateUnitOfMeasureParams{Code: "PCS", Name: "Pieces"})
	if err != nil {
		t.Fatalf("CreateUnitOfMeasure as A: %v", err)
	}
	product, err := svc.CreateProduct(ctx, principalA, catalogueapp.CreateProductParams{BaseUOMID: pcs.ID, Name: "Org A Private Product"})
	if err != nil {
		t.Fatalf("CreateProduct as A: %v", err)
	}

	if _, err := svc.GetProduct(ctx, principalB, product.ID); !errors.Is(err, cataloguedomain.ErrNotFound) {
		t.Fatalf("GetProduct as B for A's product: got err=%v, want ErrNotFound", err)
	}

	// Sanity check: A can still read its own product (proves the failure
	// above is RLS, not a bug that blocks everyone).
	if _, err := svc.GetProduct(ctx, principalA, product.ID); err != nil {
		t.Fatalf("GetProduct as A for its own product: %v", err)
	}
}
