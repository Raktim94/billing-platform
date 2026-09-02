// Package domain holds the purchases module's entity types and
// repository interfaces (docs/architecture.md §2, §4, brief §13). One
// document family (PURCHASE_ORDER, GOODS_RECEIPT, PURCHASE_INVOICE,
// PURCHASE_RETURN, DEBIT_NOTE) parameterized by DocumentType, mirroring
// the pattern the architecture doc specifies for sales.
package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"billing-platform/internal/platform/money"
)

type DocumentType string

const (
	DocPurchaseOrder   DocumentType = "PURCHASE_ORDER"
	DocGoodsReceipt    DocumentType = "GOODS_RECEIPT"
	DocPurchaseInvoice DocumentType = "PURCHASE_INVOICE"
	DocPurchaseReturn  DocumentType = "PURCHASE_RETURN"
	DocDebitNote       DocumentType = "DEBIT_NOTE"
)

func ValidDocumentType(t DocumentType) bool {
	switch t {
	case DocPurchaseOrder, DocGoodsReceipt, DocPurchaseInvoice, DocPurchaseReturn, DocDebitNote:
		return true
	default:
		return false
	}
}

// StockAffecting reports whether finalizing a document of this type
// should post stock_movements. GOODS_RECEIPT increases stock;
// PURCHASE_RETURN decreases it. A PURCHASE_ORDER is a commitment, not a
// receipt; PURCHASE_INVOICE/DEBIT_NOTE are billing documents whose
// accounting effect is Stage 6 scope — neither moves physical stock on
// their own.
func StockAffecting(t DocumentType) bool {
	return t == DocGoodsReceipt || t == DocPurchaseReturn
}

type DocumentStatus string

const (
	StatusDraft     DocumentStatus = "DRAFT"
	StatusFinalized DocumentStatus = "FINALIZED"
	StatusCancelled DocumentStatus = "CANCELLED"
)

type Document struct {
	ID                    uuid.UUID
	OrganisationID        uuid.UUID
	BranchID              uuid.UUID
	WarehouseID           uuid.UUID
	SupplierPartyID       uuid.UUID
	DocumentType          DocumentType
	DocumentNumber        string
	ReferenceDocumentID   *uuid.UUID
	Status                DocumentStatus
	SupplierInvoiceNumber string
	SupplierInvoiceDate   *time.Time
	DocumentDate          time.Time
	CurrencyCode          string
	Notes                 string
	CreatedBy             uuid.UUID
	CreatedAt             time.Time
	UpdatedAt             time.Time
	FinalizedAt           *time.Time
}

type DocumentLine struct {
	ID                 uuid.UUID
	OrganisationID     uuid.UUID
	PurchaseDocumentID uuid.UUID
	LineNumber         int
	ProductVariantID   uuid.UUID
	UnitID             uuid.UUID
	Quantity           decimal.Decimal
	// UnitPrice/LineTotal share the parent Document's CurrencyCode — see
	// internal/modules/pricing's identical pattern for why the amount is
	// stored bare and Money is reconstituted with the joined currency at
	// scan time, rather than duplicating currency_code on every line.
	UnitPrice money.Money
	LineTotal money.Money
	BatchCode string
	CreatedAt time.Time
}

type DocumentRepository interface {
	Create(ctx context.Context, d *Document) error
	GetByID(ctx context.Context, id uuid.UUID) (*Document, error)
	ListByOrganisation(ctx context.Context, orgID uuid.UUID, documentType *DocumentType) ([]*Document, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status DocumentStatus, finalizedAt *time.Time) error
	// NextNumber atomically allocates and returns the next sequence value
	// for (orgID, docType) via an UPDATE ... RETURNING on
	// purchase_document_counters, so two concurrent document creations
	// for the same organisation and type can never receive the same
	// number (Scenario I's building block).
	NextNumber(ctx context.Context, orgID uuid.UUID, docType DocumentType) (int64, error)
}

type DocumentLineRepository interface {
	Create(ctx context.Context, l *DocumentLine) error
	ListByDocument(ctx context.Context, documentID uuid.UUID) ([]*DocumentLine, error)
}
