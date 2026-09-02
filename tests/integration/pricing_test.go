//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	catalogueapp "billing-platform/internal/modules/catalogue/app"
	pricingapp "billing-platform/internal/modules/pricing/app"
	pricingdomain "billing-platform/internal/modules/pricing/domain"
	pricingpg "billing-platform/internal/modules/pricing/pg"
	"billing-platform/internal/platform/audit"
	"billing-platform/internal/platform/money"
	"billing-platform/internal/platform/permissions"
)

func newTestPricingService(t *testing.T) *pricingapp.Service {
	t.Helper()
	return pricingapp.NewService(
		sharedPool,
		pricingpg.NewPriceListRepo(sharedPool),
		pricingpg.NewPriceListItemRepo(sharedPool),
		permissions.NewChecker(permissions.NewPGStore(sharedPool), sharedPool),
		audit.NewPGRecorder(sharedPool),
	)
}

// seedVariant creates a minimal unit + product + variant in the given
// tenant, for tests that only need a valid product_variant_id/unit_id to
// satisfy price_list_items' foreign keys — the catalogue data itself
// isn't what's under test here.
func seedVariant(t *testing.T, ctx context.Context, principal permissions.Principal) (unitID, variantID uuid.UUID) {
	t.Helper()
	catSvc := newTestCatalogueService(t)
	unit, err := catSvc.CreateUnitOfMeasure(ctx, principal, catalogueapp.CreateUnitOfMeasureParams{Code: "PCS", Name: "Pieces"})
	if err != nil {
		t.Fatalf("seedVariant: CreateUnitOfMeasure: %v", err)
	}
	product, err := catSvc.CreateProduct(ctx, principal, catalogueapp.CreateProductParams{BaseUOMID: unit.ID, Name: "Priced Widget"})
	if err != nil {
		t.Fatalf("seedVariant: CreateProduct: %v", err)
	}
	variant, err := catSvc.CreateVariant(ctx, principal, catalogueapp.CreateVariantParams{ProductID: product.ID, SKUCode: "PRICED-" + uuid.NewString()[:8]})
	if err != nil {
		t.Fatalf("seedVariant: CreateVariant: %v", err)
	}
	return unit.ID, variant.ID
}

func TestPricing_PriceList_SetAndResolve(t *testing.T) {
	ctx := context.Background()
	svc := newTestPricingService(t)
	principal := bootstrapOwnerPrincipal(t, ctx)
	unitID, variantID := seedVariant(t, ctx, principal)

	priceList, err := svc.CreatePriceList(ctx, principal, pricingapp.CreatePriceListParams{Name: "Retail", CurrencyCode: "INR", IsDefault: true})
	if err != nil {
		t.Fatalf("CreatePriceList: %v", err)
	}

	price, err := money.Parse("76.271186", "INR")
	if err != nil {
		t.Fatalf("money.Parse: %v", err)
	}
	if _, err := svc.SetPrice(ctx, principal, pricingapp.SetPriceParams{PriceListID: priceList.ID, ProductVariantID: variantID, UnitID: unitID, Price: price}); err != nil {
		t.Fatalf("SetPrice: %v", err)
	}

	resolved, err := svc.ResolvePrice(ctx, principal, priceList.ID, variantID, unitID)
	if err != nil {
		t.Fatalf("ResolvePrice: %v", err)
	}
	if !resolved.Price.Decimal().Equal(price.Decimal()) {
		t.Fatalf("resolved price = %s, want %s (full precision must survive NUMERIC round trip)", resolved.Price, price)
	}
	if resolved.Price.Currency() != "INR" {
		t.Fatalf("resolved currency = %s, want INR", resolved.Price.Currency())
	}

	// Revising the price (same price list + variant + unit) must replace
	// the existing row, not accumulate a second one — pricingpg.Upsert's
	// whole reason to exist (migrations/0010_pricing.up.sql UNIQUE
	// constraint).
	revised, err := money.Parse("80.00", "INR")
	if err != nil {
		t.Fatalf("money.Parse: %v", err)
	}
	if _, err := svc.SetPrice(ctx, principal, pricingapp.SetPriceParams{PriceListID: priceList.ID, ProductVariantID: variantID, UnitID: unitID, Price: revised}); err != nil {
		t.Fatalf("SetPrice (revision): %v", err)
	}
	items, err := svc.ListPrices(ctx, principal, priceList.ID)
	if err != nil {
		t.Fatalf("ListPrices: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected exactly one price_list_item after revising the same variant+unit, got %d", len(items))
	}
	if !items[0].Price.Decimal().Equal(revised.Decimal()) {
		t.Fatalf("after revision, price = %s, want %s", items[0].Price, revised)
	}
}

func TestPricing_ResolvePrice_NotFoundWhenUnset(t *testing.T) {
	ctx := context.Background()
	svc := newTestPricingService(t)
	principal := bootstrapOwnerPrincipal(t, ctx)
	unitID, variantID := seedVariant(t, ctx, principal)

	priceList, err := svc.CreatePriceList(ctx, principal, pricingapp.CreatePriceListParams{Name: "Empty List", CurrencyCode: "INR"})
	if err != nil {
		t.Fatalf("CreatePriceList: %v", err)
	}

	if _, err := svc.ResolvePrice(ctx, principal, priceList.ID, variantID, unitID); !errors.Is(err, pricingdomain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound resolving an unset price, got %v", err)
	}
}

// TestPricing_RLS_BlocksCrossOrganisationPriceListRead is the pricing
// module's Scenario G building block.
func TestPricing_RLS_BlocksCrossOrganisationPriceListRead(t *testing.T) {
	ctx := context.Background()
	svc := newTestPricingService(t)
	principalA := bootstrapOwnerPrincipal(t, ctx)
	principalB := bootstrapOwnerPrincipal(t, ctx)

	priceList, err := svc.CreatePriceList(ctx, principalA, pricingapp.CreatePriceListParams{Name: "Org A Private List", CurrencyCode: "INR"})
	if err != nil {
		t.Fatalf("CreatePriceList as A: %v", err)
	}

	if _, err := svc.GetPriceList(ctx, principalB, priceList.ID); !errors.Is(err, pricingdomain.ErrNotFound) {
		t.Fatalf("GetPriceList as B for A's price list: got err=%v, want ErrNotFound", err)
	}
	if _, err := svc.GetPriceList(ctx, principalA, priceList.ID); err != nil {
		t.Fatalf("GetPriceList as A for its own price list: %v", err)
	}
}
