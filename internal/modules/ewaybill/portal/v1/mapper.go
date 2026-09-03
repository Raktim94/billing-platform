// Package v1 is the first (and, as of Stage 8c, only) FREE_PORTAL export
// mapper version.
//
// HONEST CAVEAT, do not remove this comment without re-verifying against
// current official documentation: the JSON shape produced here is a
// structurally reasonable placeholder for the government e-Way Bill
// portal's bulk-upload format — a top-level version tag plus an itemized
// array, matching the general shape NIC's own JSON-based bulk tools use —
// but it has NOT been verified byte-field-for-byte-field against a live
// official sample file or current NIC bulk-upload documentation as part
// of this stage's work. Per brief Rule 2 ("never invent tax rules") and
// the same caution docs/research.md applied to government API versions:
// treat every field name below as provisional until checked against the
// real portal's current "Prepare JSON" / bulk-upload tool output, and
// update ewaybill_portal_schema_versions with a new dated row (never edit
// this file's field names in place) when it's verified or when the
// government changes it.
package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"billing-platform/internal/modules/ewaybill/canonical"
	"billing-platform/internal/modules/ewaybill/portal"
)

const SchemaVersion = "v1-placeholder"

// portalParty/portalItem/portalUploadDocument are the placeholder wire
// shapes — deliberately kept separate from canonical.Party/Item (never
// serialize the canonical types directly) so a future real-schema mapper
// can reshape freely without touching the canonical model other modules
// depend on.
type portalParty struct {
	GSTIN     string `json:"gstin,omitempty"`
	LegalName string `json:"legal_name"`
	Address1  string `json:"addr1,omitempty"`
	Address2  string `json:"addr2,omitempty"`
	Place     string `json:"place,omitempty"`
	Pincode   string `json:"pincode,omitempty"`
	StateCode string `json:"state_code"`
}

type portalItem struct {
	HSNCode       string `json:"hsn_code"`
	Description   string `json:"product_desc,omitempty"`
	Quantity      string `json:"quantity"`
	Unit          string `json:"unit,omitempty"`
	TaxableAmount string `json:"taxable_amount"`
	GSTRate       string `json:"gst_rate"`
	CessRate      string `json:"cess_rate,omitempty"`
}

type portalUploadDocument struct {
	Version          string       `json:"version"`
	DocumentNumber   string       `json:"doc_no"`
	DocumentDate     string       `json:"doc_date"` // DD/MM/YYYY, matching NIC's documented date convention
	DocumentType     string       `json:"doc_type"`
	SupplierDetails  portalParty  `json:"from"`
	RecipientDetails portalParty  `json:"to"`
	ShipToDetails    portalParty  `json:"ship_to"`
	Items            []portalItem `json:"item_list"`
	TotalTaxable     string       `json:"total_taxable_amount"`
	TotalCGST        string       `json:"total_cgst"`
	TotalSGST        string       `json:"total_sgst"`
	TotalIGST        string       `json:"total_igst"`
	TotalCess        string       `json:"total_cess"`
	TransportMode    string       `json:"trans_mode,omitempty"`
	VehicleNumber    string       `json:"vehicle_no,omitempty"`
	TransporterID    string       `json:"transporter_id,omitempty"`
	TransporterName  string       `json:"transporter_name,omitempty"`
	DistanceKM       string       `json:"distance,omitempty"`
}

// MaxFileSizeBytes is a documented, configurable placeholder ceiling
// (docs/architecture.md §9b — "enforce the portal's actual file-size
// limit"); 5MB is a reasonable conservative default pending verification
// of the portal's current real limit.
const MaxFileSizeBytes = 5 * 1024 * 1024

type Mapper struct{}

func New() *Mapper { return &Mapper{} }

var _ portal.Exporter = (*Mapper)(nil)

func (m *Mapper) SchemaVersion() string { return SchemaVersion }

func (m *Mapper) PrepareUpload(_ context.Context, bill canonical.CanonicalEWayBill) (portal.PreparedFile, error) {
	doc := portalUploadDocument{
		Version:          SchemaVersion,
		DocumentNumber:   bill.InvoiceNumber,
		DocumentDate:     bill.InvoiceDate.Format("02/01/2006"),
		DocumentType:     bill.DocumentType,
		SupplierDetails:  partyToPortal(bill.Supplier),
		RecipientDetails: partyToPortal(bill.Recipient),
		ShipToDetails:    partyToPortal(bill.ShipTo),
		TotalTaxable:     bill.Tax.TaxableValue.StringFixed(2),
		TotalCGST:        bill.Tax.CGST.StringFixed(2),
		TotalSGST:        bill.Tax.SGST.StringFixed(2),
		TotalIGST:        bill.Tax.IGST.StringFixed(2),
		TotalCess:        bill.Tax.CESS.StringFixed(2),
		TransportMode:    bill.Transport.Mode,
		VehicleNumber:    bill.Transport.VehicleNumber,
		TransporterID:    bill.Transport.TransporterID,
		TransporterName:  bill.Transport.TransporterName,
		DistanceKM:       bill.Transport.DistanceKM.StringFixed(2),
	}
	for _, it := range bill.Items {
		doc.Items = append(doc.Items, portalItem{
			HSNCode: it.HSNSACCode, Description: it.Description,
			Quantity: it.Quantity.StringFixed(3), Unit: it.UnitCode,
			TaxableAmount: it.TaxableAmount.StringFixed(2),
			GSTRate:       it.GSTRate.StringFixed(2),
			CessRate:      it.CessRate.StringFixed(2),
		})
	}

	content, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return portal.PreparedFile{}, fmt.Errorf("ewaybill/portal/v1: marshaling upload document: %w", err)
	}
	if len(content) > MaxFileSizeBytes {
		// A single invoice exceeding the portal's file-size ceiling would
		// be extraordinary (thousands of line items) — this is a hard
		// stop rather than a silent truncation, since truncating a
		// government filing document is far worse than failing loudly.
		// Batch-splitting (see SplitBatch) is for the *bulk, multiple-
		// invoice* case (docs/architecture.md §9b), not a single
		// oversized document.
		return portal.PreparedFile{}, fmt.Errorf("ewaybill/portal/v1: prepared file is %d bytes, exceeds the %d byte limit for a single document", len(content), MaxFileSizeBytes)
	}

	return portal.PreparedFile{FileName: FileName(bill.InvoiceNumber, bill.InvoiceDate), Content: content}, nil
}

func partyToPortal(p canonical.Party) portalParty {
	return portalParty{
		GSTIN: p.GSTIN, LegalName: p.LegalName, Address1: p.AddressLine1, Address2: p.AddressLine2,
		Place: p.City, Pincode: p.PostalCode, StateCode: p.StateCode,
	}
}

var filenameUnsafe = regexp.MustCompile(`[^A-Za-z0-9_.-]`)

// FileName produces the human-recognizable naming convention
// docs/architecture.md §9b specifies: EWB-<invoice-number>-<date>.json.
// Illegal filesystem characters in the invoice number are sanitized to
// underscores rather than silently dropped, so the number stays
// recognizable.
func FileName(invoiceNumber string, invoiceDate time.Time) string {
	safe := filenameUnsafe.ReplaceAllString(invoiceNumber, "_")
	safe = strings.Trim(safe, "_")
	return fmt.Sprintf("EWB-%s-%s.json", safe, invoiceDate.Format("20060102"))
}

// BatchFileName is the numbered-batch variant for a multi-invoice bulk
// export exceeding the size limit (docs/architecture.md §9b:
// "EWB-BATCH-001.json, EWB-BATCH-002.json, ...").
func BatchFileName(batchNumber int) string {
	return fmt.Sprintf("EWB-BATCH-%03d.json", batchNumber)
}

// SplitBatch groups a set of already-prepared single-document contents
// into batch files, each kept under maxBytes (accounting for the JSON
// array wrapper's own overhead) — the reusable primitive behind the bulk-
// preparation flow docs/architecture.md §9b describes (the actual
// multiple-invoice-selection API/UI is out of Stage 8c's scope, since
// apps/web doesn't exist yet, but this split logic is real and tested so
// that flow has something correct to call into later).
func SplitBatch(documents [][]byte, maxBytes int) ([]portal.PreparedFile, error) {
	if len(documents) == 0 {
		return nil, nil
	}
	var batches []portal.PreparedFile
	var current []json.RawMessage
	currentSize := 2 // "[]"

	flush := func() error {
		if len(current) == 0 {
			return nil
		}
		content, err := json.MarshalIndent(current, "", "  ")
		if err != nil {
			return fmt.Errorf("ewaybill/portal/v1: marshaling batch: %w", err)
		}
		batches = append(batches, portal.PreparedFile{
			FileName: BatchFileName(len(batches) + 1),
			Content:  content,
		})
		current = nil
		currentSize = 2
		return nil
	}

	for _, d := range documents {
		// +1 for the comma/bracket overhead of adding this element to the
		// current array — an approximation, not exact re-marshaling cost,
		// which is fine for a size *ceiling* check.
		addSize := len(d) + 1
		if len(d) > maxBytes {
			return nil, fmt.Errorf("ewaybill/portal/v1: a single document (%d bytes) exceeds the batch size limit (%d bytes) on its own", len(d), maxBytes)
		}
		if currentSize+addSize > maxBytes && len(current) > 0 {
			if err := flush(); err != nil {
				return nil, err
			}
		}
		current = append(current, json.RawMessage(d))
		currentSize += addSize
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return batches, nil
}
