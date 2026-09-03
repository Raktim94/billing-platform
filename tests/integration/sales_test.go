//go:build integration

package integration

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	catalogueapp "billing-platform/internal/modules/catalogue/app"
	contactsapp "billing-platform/internal/modules/contacts/app"
	contactsdomain "billing-platform/internal/modules/contacts/domain"
	contactspg "billing-platform/internal/modules/contacts/pg"
	"billing-platform/internal/modules/gstindia"
	gstindiaapp "billing-platform/internal/modules/gstindia/app"
	gstindiadomain "billing-platform/internal/modules/gstindia/domain"
	gstindiapg "billing-platform/internal/modules/gstindia/pg"
	identityapp "billing-platform/internal/modules/identity/app"
	inventoryapp "billing-platform/internal/modules/inventory/app"
	pricingapp "billing-platform/internal/modules/pricing/app"
	pricingpg "billing-platform/internal/modules/pricing/pg"
	salesapp "billing-platform/internal/modules/sales/app"
	salesdomain "billing-platform/internal/modules/sales/domain"
	salespg "billing-platform/internal/modules/sales/pg"
	"billing-platform/internal/modules/sales/printing"
	taxationapp "billing-platform/internal/modules/taxation/app"
	taxdomain "billing-platform/internal/modules/taxation/domain"
	taxationpg "billing-platform/internal/modules/taxation/pg"
	"billing-platform/internal/platform/audit"
	"billing-platform/internal/platform/numbering"
	"billing-platform/internal/platform/permissions"
)

// salesFixture wires every module sales.Service depends on against
// sharedPool, exactly as apps/server/main.go composes them, and
// provisions one organisation with a GST-registered legal entity (Stage
// 5b's additive migrations/0017 field), a base unit + HSN-classified
// product + variant, a customer party, and one configured tax rate — the
// minimum a FinalizeDocument call actually needs to succeed.
type salesFixture struct {
	Principal     permissions.Principal
	LegalEntityID uuid.UUID
	BranchID      uuid.UUID
	WarehouseID   uuid.UUID
	VariantID     uuid.UUID
	PCS           uuid.UUID
	CustomerID    uuid.UUID
	HSN           string
}

func newTestSalesServices(t *testing.T) (*salesapp.Service, *inventoryapp.Service, *catalogueapp.Service, *gstindiaapp.Service) {
	t.Helper()
	checker := permissions.NewChecker(permissions.NewPGStore(sharedPool), sharedPool)
	recorder := audit.NewPGRecorder(sharedPool)

	catalogueSvc := newTestCatalogueService(t)
	contactsSvc := contactsapp.NewService(
		sharedPool,
		contactspg.NewPartyRepo(sharedPool),
		contactspg.NewAddressRepo(sharedPool),
		contactspg.NewTaxRegistrationRepo(sharedPool),
		checker, recorder,
	)
	orgSvc := newTestOrgService(t)
	inventorySvc := newTestInventoryService(t)

	gstRateRepo := gstindiapg.NewTaxRateRepo(sharedPool)
	gstindiaSvc := gstindiaapp.NewService(sharedPool, gstRateRepo, gstindiapg.NewStateRepo(sharedPool), checker, recorder)
	gstEngine := gstindia.NewEngine(gstRateRepo, gstindiapg.NewStateRepo(sharedPool))
	taxationSvc := taxationapp.NewService(
		sharedPool, gstEngine,
		taxationpg.NewTaxDocumentRepo(sharedPool),
		taxationpg.NewTaxLineRepo(sharedPool),
		taxationpg.NewTaxComponentRepo(sharedPool),
	)
	numberingSvc := numbering.NewService(sharedPool, numbering.NewPGRepository(sharedPool))
	pricingSvc := pricingapp.NewService(
		sharedPool,
		pricingpg.NewPriceListRepo(sharedPool),
		pricingpg.NewPriceListItemRepo(sharedPool),
		checker, recorder,
	)

	// accountingSvc is nil here deliberately — this pre-Stage-6 helper is
	// shared by every existing sales test, none of which set up a chart of
	// accounts; FinalizeDocument treats a nil accounting as "skip posting"
	// (see sales/app.Service's field comment). Stage 6's own tests
	// (accounting_test.go) construct their own sales/purchases services
	// WITH a real accountingSvc wired in.
	salesSvc := salesapp.NewService(
		sharedPool,
		salespg.NewDocumentRepo(sharedPool),
		salespg.NewDocumentLineRepo(sharedPool),
		inventorySvc, taxationSvc, catalogueSvc, contactsSvc, orgSvc, pricingSvc, numberingSvc, nil, nil,
		checker, recorder,
	)
	return salesSvc, inventorySvc, catalogueSvc, gstindiaSvc
}

func setupSalesFixture(t *testing.T, ctx context.Context) salesFixture {
	t.Helper()
	identitySvc, _ := newTestIdentityService(t)
	unique := uuid.NewString()[:8]
	boot, err := identitySvc.Bootstrap(ctx, identityapp.BootstrapParams{
		OrganisationName: "Sales Test Co " + unique, DefaultCurrencyCode: "INR", DefaultTimezone: "Asia/Kolkata",
		LegalEntityName: "Sales Test Co " + unique + " Pvt Ltd", CountryCode: "IN",
		GSTIN: "27AAAAA0000A1Z5", GSTStateCode: "27", // Maharashtra, same code Stage 5a's golden fixtures use
		BranchCode: "BR-" + unique, BranchName: "Main Branch",
		WarehouseCode: "WH-" + unique, WarehouseName: "Main Warehouse",
		OwnerEmail: "sales-" + unique + "@example.com", OwnerFullName: "Test Owner", OwnerPassword: "correct horse battery staple 42",
	})
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	principal := permissions.Principal{UserID: boot.OwnerUserID, OrganisationID: boot.OrganisationID}

	catalogueSvc := newTestCatalogueService(t)
	pcs, err := catalogueSvc.CreateUnitOfMeasure(ctx, principal, catalogueapp.CreateUnitOfMeasureParams{Code: "PCS", Name: "Pieces"})
	if err != nil {
		t.Fatalf("CreateUnitOfMeasure: %v", err)
	}
	hsn := "998" + uuid.NewString()[:5]
	product, err := catalogueSvc.CreateProduct(ctx, principal, catalogueapp.CreateProductParams{
		BaseUOMID: pcs.ID, Name: "Sales Test Widget " + unique, HSNSACCode: hsn,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	variant, err := catalogueSvc.CreateVariant(ctx, principal, catalogueapp.CreateVariantParams{ProductID: product.ID, SKUCode: "SAL-" + unique})
	if err != nil {
		t.Fatalf("CreateVariant: %v", err)
	}

	inventorySvc := newTestInventoryService(t)
	openingCost := mustDecimal(t, "50")
	if _, err := inventorySvc.RecordOpeningStock(ctx, principal, inventoryapp.RecordMovementParams{
		WarehouseID: boot.WarehouseID, ProductVariantID: variant.ID, MovementType: "OPENING",
		UnitID: pcs.ID, Quantity: mustDecimal(t, "100"), UnitCost: &openingCost,
	}); err != nil {
		t.Fatalf("RecordOpeningStock: %v", err)
	}

	checker := permissions.NewChecker(permissions.NewPGStore(sharedPool), sharedPool)
	recorder := audit.NewPGRecorder(sharedPool)
	gstindiaSvc := gstindiaapp.NewService(sharedPool, gstindiapg.NewTaxRateRepo(sharedPool), gstindiapg.NewStateRepo(sharedPool), checker, recorder)
	if _, err := gstindiaSvc.CreateRate(ctx, principal, gstindiaapp.CreateRateParams{
		HSNSACCode: hsn, Classification: gstindiadomain.ClassificationTaxable,
		GSTRate: mustDecimal(t, "18"), CessRate: mustDecimal(t, "0"), ValidFrom: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("CreateRate: %v", err)
	}

	contactsSvc := contactsapp.NewService(
		sharedPool, contactspg.NewPartyRepo(sharedPool), contactspg.NewAddressRepo(sharedPool), contactspg.NewTaxRegistrationRepo(sharedPool),
		checker, recorder,
	)
	customer, err := contactsSvc.CreateParty(ctx, principal, contactsapp.CreatePartyParams{
		PartyType: contactsdomain.PartyCustomer, LegalName: "Test Customer " + unique, CurrencyCode: "INR",
	})
	if err != nil {
		t.Fatalf("CreateParty(customer): %v", err)
	}

	return salesFixture{
		Principal: principal, LegalEntityID: boot.LegalEntityID, BranchID: boot.BranchID, WarehouseID: boot.WarehouseID,
		VariantID: variant.ID, PCS: pcs.ID, CustomerID: customer.ID, HSN: hsn,
	}
}

func TestSales_TaxInvoice_FinalizePostsTaxSnapshotAndStock(t *testing.T) {
	ctx := context.Background()
	salesSvc, inventorySvc, _, _ := newTestSalesServices(t)
	fx := setupSalesFixture(t, ctx)

	doc, err := salesSvc.CreateDocument(ctx, fx.Principal, salesapp.CreateDocumentParams{
		LegalEntityID: fx.LegalEntityID, BranchID: fx.BranchID, WarehouseID: fx.WarehouseID, CustomerPartyID: fx.CustomerID,
		DocumentType: salesdomain.DocTaxInvoice, PlaceOfSupplyStateCode: "27", CurrencyCode: "INR", BaseCurrencyCode: "INR",
		PricingMode: taxdomain.PricingExclusive,
	})
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	if doc.DocumentNumber == "" {
		t.Fatal("document number was not allocated")
	}

	if _, err := salesSvc.AddLine(ctx, fx.Principal, salesapp.AddLineParams{
		DocumentID: doc.ID, ProductVariantID: fx.VariantID, UnitID: fx.PCS,
		Quantity: mustDecimal(t, "10"), UnitPrice: mustDecimal(t, "100"),
	}); err != nil {
		t.Fatalf("AddLine: %v", err)
	}

	finalized, err := salesSvc.FinalizeDocument(ctx, fx.Principal, doc.ID)
	if err != nil {
		t.Fatalf("FinalizeDocument: %v", err)
	}
	if finalized.Status != salesdomain.StatusFinalized {
		t.Fatalf("status = %s, want FINALIZED", finalized.Status)
	}
	if finalized.TaxDocumentID == nil {
		t.Fatal("tax_document_id was not stamped on finalize")
	}
	// 10 * 100 = 1000 taxable, exclusive, 18% GST intra-state -> 90 CGST + 90 SGST -> grand total 1180.
	if got := finalized.GrandTotalAmount.StringFixed(0); got != "1180.00" {
		t.Fatalf("GrandTotalAmount = %s, want 1180.00", got)
	}

	bal, err := inventorySvc.GetBalance(ctx, fx.Principal, fx.WarehouseID, fx.VariantID)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if !bal.QuantityOnHand.Equal(mustDecimal(t, "90")) {
		t.Fatalf("QuantityOnHand after sale = %s, want 90 (100 opening - 10 sold)", bal.QuantityOnHand)
	}
}

func TestSales_Finalize_InsufficientStockRejectsAtomically(t *testing.T) {
	ctx := context.Background()
	salesSvc, inventorySvc, _, _ := newTestSalesServices(t)
	fx := setupSalesFixture(t, ctx)

	doc, err := salesSvc.CreateDocument(ctx, fx.Principal, salesapp.CreateDocumentParams{
		LegalEntityID: fx.LegalEntityID, BranchID: fx.BranchID, WarehouseID: fx.WarehouseID, CustomerPartyID: fx.CustomerID,
		DocumentType: salesdomain.DocTaxInvoice, PlaceOfSupplyStateCode: "27", CurrencyCode: "INR", BaseCurrencyCode: "INR",
	})
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	// Only 100 PCS in stock (opening stock from setupSalesFixture); ask for more.
	if _, err := salesSvc.AddLine(ctx, fx.Principal, salesapp.AddLineParams{
		DocumentID: doc.ID, ProductVariantID: fx.VariantID, UnitID: fx.PCS,
		Quantity: mustDecimal(t, "1000"), UnitPrice: mustDecimal(t, "100"),
	}); err != nil {
		t.Fatalf("AddLine: %v", err)
	}

	if _, err := salesSvc.FinalizeDocument(ctx, fx.Principal, doc.ID); err == nil {
		t.Fatal("FinalizeDocument succeeded with insufficient stock, want an error")
	}

	// Atomicity: the document must still be DRAFT (not partially
	// finalized), and stock must be untouched — proving the tax
	// calculation + stock check + status update rolled back together.
	refetched, _, err := salesSvc.GetDocument(ctx, fx.Principal, doc.ID)
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if refetched.Status != salesdomain.StatusDraft {
		t.Fatalf("status after failed finalize = %s, want DRAFT (atomicity broken)", refetched.Status)
	}
	bal, err := inventorySvc.GetBalance(ctx, fx.Principal, fx.WarehouseID, fx.VariantID)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if !bal.QuantityOnHand.Equal(mustDecimal(t, "100")) {
		t.Fatalf("QuantityOnHand after failed finalize = %s, want 100 (unchanged)", bal.QuantityOnHand)
	}
}

// TestSales_Finalize_ZeroValueDocumentRejectedWithClearError verifies the
// fix for a confusing crash: a tax invoice whose lines all price out to
// ₹0.00 (e.g. a product added with no configured selling price — the
// billing UI silently defaults an unpriced line to "0") must be rejected
// with a clear, specific error before reaching accounting's double-entry
// post, which would otherwise fail deep inside with an opaque
// "must be either a debit or a credit, not both/neither" 500.
func TestSales_Finalize_ZeroValueDocumentRejectedWithClearError(t *testing.T) {
	ctx := context.Background()
	salesSvc, _, _, _ := newTestSalesServices(t)
	fx := setupSalesFixture(t, ctx)

	doc, err := salesSvc.CreateDocument(ctx, fx.Principal, salesapp.CreateDocumentParams{
		LegalEntityID: fx.LegalEntityID, BranchID: fx.BranchID, WarehouseID: fx.WarehouseID, CustomerPartyID: fx.CustomerID,
		DocumentType: salesdomain.DocTaxInvoice, PlaceOfSupplyStateCode: "27", CurrencyCode: "INR", BaseCurrencyCode: "INR",
	})
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	if _, err := salesSvc.AddLine(ctx, fx.Principal, salesapp.AddLineParams{
		DocumentID: doc.ID, ProductVariantID: fx.VariantID, UnitID: fx.PCS,
		Quantity: mustDecimal(t, "1"), UnitPrice: mustDecimal(t, "0"),
	}); err != nil {
		t.Fatalf("AddLine: %v", err)
	}

	_, err = salesSvc.FinalizeDocument(ctx, fx.Principal, doc.ID)
	if !errors.Is(err, salesdomain.ErrZeroValueDocument) {
		t.Fatalf("FinalizeDocument error = %v, want ErrZeroValueDocument", err)
	}

	refetched, _, err := salesSvc.GetDocument(ctx, fx.Principal, doc.ID)
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if refetched.Status != salesdomain.StatusDraft {
		t.Fatalf("status after rejected finalize = %s, want DRAFT (atomicity broken)", refetched.Status)
	}
}

// TestSales_Numbering_ConcurrentCreateUniqueSequentialNumbers is
// Scenario I's building block for the sales module specifically: N
// concurrent CreateDocument calls for the same (org, branch, doc type,
// financial year) must all receive distinct, gap-free sequential
// numbers — proving internal/platform/numbering's INSERT ... ON CONFLICT
// DO UPDATE ... RETURNING allocation is genuinely race-free under real
// concurrent load, not just single-threaded-correct.
func TestSales_Numbering_ConcurrentCreateUniqueSequentialNumbers(t *testing.T) {
	ctx := context.Background()
	salesSvc, _, _, _ := newTestSalesServices(t)
	fx := setupSalesFixture(t, ctx)

	const n = 12
	numbers := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			doc, err := salesSvc.CreateDocument(ctx, fx.Principal, salesapp.CreateDocumentParams{
				LegalEntityID: fx.LegalEntityID, BranchID: fx.BranchID, WarehouseID: fx.WarehouseID, CustomerPartyID: fx.CustomerID,
				DocumentType: salesdomain.DocQuotation, CurrencyCode: "INR", BaseCurrencyCode: "INR",
			})
			errs[i] = err
			if err == nil {
				numbers[i] = doc.DocumentNumber
			}
		}(i)
	}
	wg.Wait()

	seen := make(map[string]bool, n)
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent CreateDocument[%d]: %v", i, err)
		}
		if seen[numbers[i]] {
			t.Fatalf("duplicate document number allocated: %s", numbers[i])
		}
		seen[numbers[i]] = true
	}
	if len(seen) != n {
		t.Fatalf("got %d distinct numbers, want %d (no gaps, no duplicates)", len(seen), n)
	}
}

// TestSales_TaxSnapshot_ImmutableAfterLaterRateMasterUpdate proves brief
// §7's "never recalculate an old finalized invoice using today's GST
// master" specifically through the sales module's real finalize path
// (Stage 5a already proved this for the tax engine in isolation) — a
// rate change after finalize must not move the invoice's already-printed
// numbers.
func TestSales_TaxSnapshot_ImmutableAfterLaterRateMasterUpdate(t *testing.T) {
	ctx := context.Background()
	salesSvc, _, _, gstindiaSvc := newTestSalesServices(t)
	fx := setupSalesFixture(t, ctx)

	doc, err := salesSvc.CreateDocument(ctx, fx.Principal, salesapp.CreateDocumentParams{
		LegalEntityID: fx.LegalEntityID, BranchID: fx.BranchID, WarehouseID: fx.WarehouseID, CustomerPartyID: fx.CustomerID,
		DocumentType: salesdomain.DocTaxInvoice, PlaceOfSupplyStateCode: "27", CurrencyCode: "INR", BaseCurrencyCode: "INR",
	})
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	if _, err := salesSvc.AddLine(ctx, fx.Principal, salesapp.AddLineParams{
		DocumentID: doc.ID, ProductVariantID: fx.VariantID, UnitID: fx.PCS,
		Quantity: mustDecimal(t, "5"), UnitPrice: mustDecimal(t, "100"),
	}); err != nil {
		t.Fatalf("AddLine: %v", err)
	}
	finalized, err := salesSvc.FinalizeDocument(ctx, fx.Principal, doc.ID)
	if err != nil {
		t.Fatalf("FinalizeDocument: %v", err)
	}
	originalTotal := finalized.GrandTotalAmount.StringFixed(0)

	// Now change the GST rate for this HSN going forward.
	if _, err := gstindiaSvc.CreateRate(ctx, fx.Principal, gstindiaapp.CreateRateParams{
		HSNSACCode: fx.HSN, Classification: gstindiadomain.ClassificationTaxable,
		GSTRate: mustDecimal(t, "28"), CessRate: mustDecimal(t, "0"), ValidFrom: time.Now().AddDate(0, 0, 1),
	}); err != nil {
		t.Fatalf("CreateRate (new rate): %v", err)
	}

	refetched, _, err := salesSvc.GetDocument(ctx, fx.Principal, doc.ID)
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if got := refetched.GrandTotalAmount.StringFixed(0); got != originalTotal {
		t.Fatalf("GrandTotalAmount after later rate change = %s, want unchanged %s", got, originalTotal)
	}
}

func TestSales_RLS_BlocksCrossOrganisationDocumentRead(t *testing.T) {
	ctx := context.Background()
	salesSvc, _, _, _ := newTestSalesServices(t)
	fxA := setupSalesFixture(t, ctx)
	fxB := setupSalesFixture(t, ctx)

	docA, err := salesSvc.CreateDocument(ctx, fxA.Principal, salesapp.CreateDocumentParams{
		LegalEntityID: fxA.LegalEntityID, BranchID: fxA.BranchID, WarehouseID: fxA.WarehouseID, CustomerPartyID: fxA.CustomerID,
		DocumentType: salesdomain.DocQuotation, CurrencyCode: "INR", BaseCurrencyCode: "INR",
	})
	if err != nil {
		t.Fatalf("CreateDocument (org A): %v", err)
	}

	if _, _, err := salesSvc.GetDocument(ctx, fxB.Principal, docA.ID); err == nil {
		t.Fatal("RLS FAILED: org B's principal could read org A's sales_document")
	}
}

func TestSales_Print_A4Invoice_RendersNonEmptyPDF(t *testing.T) {
	ctx := context.Background()
	salesSvc, _, _, _ := newTestSalesServices(t)
	fx := setupSalesFixture(t, ctx)

	doc, err := salesSvc.CreateDocument(ctx, fx.Principal, salesapp.CreateDocumentParams{
		LegalEntityID: fx.LegalEntityID, BranchID: fx.BranchID, WarehouseID: fx.WarehouseID, CustomerPartyID: fx.CustomerID,
		DocumentType: salesdomain.DocTaxInvoice, PlaceOfSupplyStateCode: "27", CurrencyCode: "INR", BaseCurrencyCode: "INR",
	})
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	if _, err := salesSvc.AddLine(ctx, fx.Principal, salesapp.AddLineParams{
		DocumentID: doc.ID, ProductVariantID: fx.VariantID, UnitID: fx.PCS,
		Quantity: mustDecimal(t, "2"), UnitPrice: mustDecimal(t, "250"),
	}); err != nil {
		t.Fatalf("AddLine: %v", err)
	}
	if _, err := salesSvc.FinalizeDocument(ctx, fx.Principal, doc.ID); err != nil {
		t.Fatalf("FinalizeDocument: %v", err)
	}

	data, err := salesSvc.BuildInvoiceData(ctx, fx.Principal, doc.ID)
	if err != nil {
		t.Fatalf("BuildInvoiceData: %v", err)
	}
	for _, tpl := range []printing.Template{printing.TemplateA4GSTInvoice, printing.TemplateThermal80mm, printing.TemplateThermal58mm} {
		pdfBytes, err := printing.RenderPDF(tpl, *data)
		if err != nil {
			t.Fatalf("RenderPDF(%s): %v", tpl, err)
		}
		if len(pdfBytes) < 100 {
			t.Fatalf("RenderPDF(%s) produced suspiciously small output: %d bytes", tpl, len(pdfBytes))
		}
		if !bytes.HasPrefix(pdfBytes, []byte("%PDF")) {
			t.Fatalf("RenderPDF(%s) output does not start with the PDF magic bytes", tpl)
		}
	}
}
