package domain

import "testing"

func TestValidDocumentType(t *testing.T) {
	valid := []DocumentType{
		DocQuotation, DocProformaInvoice, DocSalesOrder, DocDeliveryChallan, DocTaxInvoice,
		DocPOSInvoice, DocCreditNote, DocDebitNote, DocSalesReturn, DocRecurringInvoice,
	}
	for _, v := range valid {
		if !ValidDocumentType(v) {
			t.Errorf("ValidDocumentType(%s) = false, want true", v)
		}
	}
	if ValidDocumentType("NOT_A_REAL_TYPE") {
		t.Error("ValidDocumentType(bogus) = true, want false")
	}
}

func TestStockAffecting(t *testing.T) {
	cases := map[DocumentType]bool{
		DocTaxInvoice:       true,
		DocPOSInvoice:       true,
		DocDeliveryChallan:  true,
		DocSalesReturn:      true,
		DocQuotation:        false,
		DocProformaInvoice:  false,
		DocSalesOrder:       false,
		DocRecurringInvoice: false,
		DocCreditNote:       false,
		DocDebitNote:        false,
	}
	for docType, want := range cases {
		if got := StockAffecting(docType); got != want {
			t.Errorf("StockAffecting(%s) = %v, want %v", docType, got, want)
		}
	}
}

func TestMovementTypeFor(t *testing.T) {
	if got := MovementTypeFor(DocSalesReturn); got != "SALE_RETURN" {
		t.Errorf("MovementTypeFor(SALES_RETURN) = %q, want SALE_RETURN", got)
	}
	for _, docType := range []DocumentType{DocTaxInvoice, DocPOSInvoice, DocDeliveryChallan} {
		if got := MovementTypeFor(docType); got != "SALE" {
			t.Errorf("MovementTypeFor(%s) = %q, want SALE", docType, got)
		}
	}
}
