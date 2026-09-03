package v1

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"billing-platform/internal/modules/ewaybill/canonical"
)

func TestFileName_HumanRecognizable(t *testing.T) {
	got := FileName("INV/2026-27/000133", time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC))
	if got != "EWB-INV_2026-27_000133-20260903.json" {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, "INV") || !strings.Contains(got, "133") {
		t.Fatalf("filename %q lost the recognizable invoice number", got)
	}
}

func TestFileName_SanitizesUnsafeCharacters(t *testing.T) {
	got := FileName(`INV\..///weird*name`, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	for _, unsafe := range []string{"\\", "*", "//"} {
		if strings.Contains(got, unsafe) {
			t.Fatalf("filename %q still contains unsafe char %q", got, unsafe)
		}
	}
}

func TestPrepareUpload_ProducesNonEmptyValidJSON(t *testing.T) {
	m := New()
	bill := canonical.CanonicalEWayBill{
		SalesDocumentID: uuid.Must(uuid.NewV7()), InvoiceNumber: "INV-1", InvoiceDate: time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC),
		DocumentType: "INV", Supplier: canonical.Party{LegalName: "Acme", GSTIN: "27AAAAA0000A1Z5", StateCode: "27"},
		Recipient: canonical.Party{LegalName: "Buyer", StateCode: "29"}, ShipTo: canonical.Party{StateCode: "29"},
		// DispatchFrom mirrors Supplier and Recipient has no GSTIN set,
		// same as ShipTo — matching real usage (buildCanonicalFromLiveData
		// always sets DispatchFrom: supplier), so transactionType resolves
		// to "1" (Regular), not a spurious "3"/"2" from an unrealistic
		// fixture.
		DispatchFrom: canonical.Party{LegalName: "Acme", GSTIN: "27AAAAA0000A1Z5", StateCode: "27"},
		Items:        []canonical.Item{{LineRef: "1", HSNSACCode: "998877", Quantity: decimal.NewFromInt(1), TaxableAmount: decimal.NewFromInt(1000), GSTRate: decimal.NewFromInt(18), IGSTRate: decimal.NewFromInt(18)}},
		Tax:          canonical.TaxTotals{TaxableValue: decimal.NewFromInt(1000), IGST: decimal.NewFromInt(180), GrandTotal: decimal.NewFromInt(1180)},
		Transport:    canonical.Transport{VehicleNumber: "KA01AB1234", DistanceKM: decimal.NewFromInt(50)},
	}
	file, err := m.PrepareUpload(context.Background(), bill)
	if err != nil {
		t.Fatalf("PrepareUpload: %v", err)
	}
	if len(file.Content) == 0 {
		t.Fatal("prepared file has no content")
	}
	if !json.Valid(file.Content) {
		t.Fatal("prepared file content is not valid JSON")
	}
	if file.FileName == "" || !strings.HasPrefix(file.FileName, "EWB-") {
		t.Fatalf("unexpected filename %q", file.FileName)
	}
	var decoded map[string]any
	if err := json.Unmarshal(file.Content, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded["docNo"] != "INV-1" {
		t.Fatalf("docNo = %v, want INV-1", decoded["docNo"])
	}
	if decoded["fromGstin"] != "27AAAAA0000A1Z5" {
		t.Fatalf("fromGstin = %v, want the real field flat at root level, not nested under \"from\"", decoded["fromGstin"])
	}
	if decoded["transactionType"] != "1" {
		t.Fatalf("transactionType = %v, want \"1\" (Regular — ShipTo/DispatchFrom match Recipient/Supplier in this fixture)", decoded["transactionType"])
	}
	if decoded["totInvValue"] != "1180.00" {
		t.Fatalf("totInvValue = %v, want 1180.00", decoded["totInvValue"])
	}
	items, ok := decoded["itemList"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("itemList = %v, want exactly 1 item", decoded["itemList"])
	}
	item := items[0].(map[string]any)
	if item["igstRate"] != "18.00" || item["cgstRate"] != "0.00" {
		t.Fatalf("item rates = %+v, want the per-component split preserved (igstRate=18.00, cgstRate=0.00), not collapsed into one combined rate", item)
	}
}

func TestPrepareUpload_TransactionType_DetectsBillToShipTo(t *testing.T) {
	m := New()
	bill := canonical.CanonicalEWayBill{
		InvoiceNumber: "INV-2", InvoiceDate: time.Now(),
		Supplier:  canonical.Party{GSTIN: "27AAAAA0000A1Z5", StateCode: "27"},
		Recipient: canonical.Party{GSTIN: "29BBBBB0000B1Z5", StateCode: "29"},
		// ShipTo has a DIFFERENT GSTIN than Recipient — a genuine
		// bill-to/ship-to split, which must produce transactionType "2".
		ShipTo:       canonical.Party{GSTIN: "07CCCCC0000C1Z5", StateCode: "07"},
		DispatchFrom: canonical.Party{GSTIN: "27AAAAA0000A1Z5", StateCode: "27"},
		Tax:          canonical.TaxTotals{GrandTotal: decimal.NewFromInt(1000)},
	}
	file, err := m.PrepareUpload(context.Background(), bill)
	if err != nil {
		t.Fatalf("PrepareUpload: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(file.Content, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded["transactionType"] != "2" {
		t.Fatalf("transactionType = %v, want \"2\" (Bill To-Ship To)", decoded["transactionType"])
	}
	if decoded["shipToGSTIN"] != "07CCCCC0000C1Z5" {
		t.Fatalf("shipToGSTIN = %v, want 07CCCCC0000C1Z5", decoded["shipToGSTIN"])
	}
}

func TestSplitBatch_KeepsEachFileUnderLimit(t *testing.T) {
	// 5 documents of ~40 bytes each, a tiny 100-byte cap forces multiple
	// batch files.
	var docs [][]byte
	for i := 0; i < 5; i++ {
		docs = append(docs, []byte(`{"doc_no":"INV-0000000`+string(rune('0'+i))+`","padding":"xxxxxxxxxxxxxxxxxxxx"}`))
	}
	batches, err := SplitBatch(docs, 100)
	if err != nil {
		t.Fatalf("SplitBatch: %v", err)
	}
	if len(batches) < 2 {
		t.Fatalf("expected multiple batch files under a 100-byte cap, got %d", len(batches))
	}
	for i, b := range batches {
		if len(b.Content) > 100 {
			t.Fatalf("batch %d is %d bytes, exceeds the 100-byte cap", i, len(b.Content))
		}
		if !json.Valid(b.Content) {
			t.Fatalf("batch %d content is not valid JSON", i)
		}
		if b.FileName != BatchFileName(i+1) {
			t.Fatalf("batch %d filename = %q, want %q", i, b.FileName, BatchFileName(i+1))
		}
	}
}

func TestSplitBatch_SingleDocumentExceedingLimit_ErrorsRatherThanTruncates(t *testing.T) {
	huge := make([]byte, 200)
	for i := range huge {
		huge[i] = 'x'
	}
	_, err := SplitBatch([][]byte{huge}, 100)
	if err == nil {
		t.Fatal("expected an error for a single document exceeding the batch limit, got nil")
	}
}
