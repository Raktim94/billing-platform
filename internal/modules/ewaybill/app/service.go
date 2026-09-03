// Package app is e-Way Bill's application layer. GenerateForDocument is
// called directly (via httpapi), NOT auto-enqueued from
// sales.FinalizeDocument the way einvoice's is — whether a given sale
// needs an e-Way Bill depends on real-world facts this codebase doesn't
// model yet (goods-movement distance/value thresholds, exempt-goods
// lists), so auto-triggering it would be guessing at a business rule
// rather than implementing one. An operator/API caller decides when to
// generate one, same as they'd decide when to actually dispatch goods.
package app

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	einvoicedomain "billing-platform/internal/modules/einvoice/domain"
	"billing-platform/internal/modules/ewaybill/domain"
	salesapp "billing-platform/internal/modules/sales/app"
)

type Service struct {
	records  domain.Repository
	provider einvoicedomain.EInvoiceProvider
	sales    *salesapp.Service
	now      func() time.Time
}

func NewService(records domain.Repository, provider einvoicedomain.EInvoiceProvider, salesSvc *salesapp.Service) *Service {
	return &Service{records: records, provider: provider, sales: salesSvc, now: time.Now}
}

type GenerateParams struct {
	SalesDocumentID uuid.UUID
	IRN             string // from the sales document's already-GENERATED einvoice_records row
	TransporterID   string
	TransporterName string
	TransportMode   string
	VehicleNumber   string
	DistanceKM      decimal.Decimal
	// ShipToGSTIN: the 2026-08-01 GSTN requirement — pass "URP" explicitly
	// for an unregistered/not-applicable recipient, never leave it empty
	// when ship-to details are present (docs/research.md).
	ShipToGSTIN string
}

// GenerateForDocument must run inside a caller-provided RunScoped block
// (same non-self-scoping convention as einvoice.Service — its only
// intended caller is httpapi, invoked after that layer's own permission
// check and RunScoped wrapper; see internal/modules/ewaybill/httpapi if/when
// it's added). Idempotent the same way einvoice is: an existing GENERATED
// record for this sales document is left alone.
func (s *Service) GenerateForDocument(ctx context.Context, orgID uuid.UUID, p GenerateParams) (*domain.Record, error) {
	existing, err := s.records.GetBySalesDocumentID(ctx, p.SalesDocumentID)
	if err != nil && err != domain.ErrNotFound {
		return nil, fmt.Errorf("ewaybill: loading existing record: %w", err)
	}
	if existing != nil && existing.Status.Terminal() {
		return existing, nil
	}

	if existing == nil {
		id, err := uuid.NewV7()
		if err != nil {
			return nil, fmt.Errorf("ewaybill: generating record id: %w", err)
		}
		irn := p.IRN
		rec := &domain.Record{
			ID: id, OrganisationID: orgID, SalesDocumentID: p.SalesDocumentID, IRN: &irn,
			Status: domain.StatusQueued, TransporterID: p.TransporterID, TransporterName: p.TransporterName,
			VehicleNumber: p.VehicleNumber,
		}
		if p.ShipToGSTIN != "" {
			rec.ShipToGSTIN = &p.ShipToGSTIN
		}
		distStr := p.DistanceKM.String()
		rec.DistanceKM = &distStr
		if err := s.records.Create(ctx, rec); err != nil {
			return nil, fmt.Errorf("ewaybill: creating record: %w", err)
		}
		existing = rec
	}

	if err := s.records.UpdateStatus(ctx, existing.ID, domain.StatusSubmitting, domain.UpdateFields{}); err != nil {
		return nil, fmt.Errorf("ewaybill: marking submitting: %w", err)
	}

	resp, genErr := s.provider.GenerateEWayBillByIRN(ctx, p.IRN, einvoicedomain.TransportDetails{
		TransporterID: p.TransporterID, TransporterName: p.TransporterName, TransportMode: p.TransportMode,
		VehicleNumber: p.VehicleNumber, DistanceKM: p.DistanceKM, ShipToGSTIN: p.ShipToGSTIN,
	})
	if genErr != nil {
		msg := genErr.Error()
		if updErr := s.records.UpdateStatus(ctx, existing.ID, domain.StatusFailedRetryable, domain.UpdateFields{ErrorMessage: &msg}); updErr != nil {
			return nil, fmt.Errorf("ewaybill: marking failed-retryable: %w", updErr)
		}
		return nil, fmt.Errorf("ewaybill: GenerateEWayBillByIRN: %w", genErr)
	}

	ewbNo := resp.EWBNumber
	validFrom, validUntil := resp.ValidFrom, resp.ValidUntil
	if err := s.records.UpdateStatus(ctx, existing.ID, domain.StatusGenerated, domain.UpdateFields{
		EWBNumber: &ewbNo, ValidFrom: &validFrom, ValidUntil: &validUntil,
	}); err != nil {
		return nil, fmt.Errorf("ewaybill: marking generated: %w", err)
	}
	existing.Status = domain.StatusGenerated
	existing.EWBNumber = &ewbNo
	return existing, nil
}

// UpdatePartB records a vehicle/transporter change en route — appended to
// history (brief §10's "Part-B history"), not overwritten.
func (s *Service) UpdatePartB(ctx context.Context, id uuid.UUID, update domain.PartBUpdate) error {
	update.At = s.now()
	return s.records.AppendPartBHistory(ctx, id, update)
}

// Close implements the 2026-08-01 GSTN voluntary EWB closure facility
// (docs/research.md): supplier/recipient/transporter/driver marks an
// already-GENERATED EWB CLOSED (the shipment happened) — distinct from
// Cancel (the movement never happened).
func (s *Service) Close(ctx context.Context, id uuid.UUID, role domain.ClosedByRole) error {
	switch role {
	case domain.ClosedBySupplier, domain.ClosedByRecipient, domain.ClosedByTransporter, domain.ClosedByDriver:
	default:
		return domain.ErrInvalidCloseRole
	}
	now := time.Now()
	return s.records.UpdateStatus(ctx, id, domain.StatusClosed, domain.UpdateFields{ClosedAt: &now, ClosedByRole: &role})
}

// Cancel marks an EWB CANCELLED (the movement never happened), as opposed
// to Close (it happened, it's just no longer in transit).
func (s *Service) Cancel(ctx context.Context, salesDocumentID uuid.UUID, reason string) error {
	rec, err := s.records.GetBySalesDocumentID(ctx, salesDocumentID)
	if err != nil {
		return fmt.Errorf("ewaybill: loading record to cancel: %w", err)
	}
	if rec.Status != domain.StatusGenerated || rec.EWBNumber == nil {
		return domain.ErrNotGenerated
	}
	if err := s.provider.CancelEWayBill(ctx, *rec.EWBNumber, reason); err != nil {
		return fmt.Errorf("ewaybill: provider cancel: %w", err)
	}
	now := time.Now()
	return s.records.UpdateStatus(ctx, rec.ID, domain.StatusCancelled, domain.UpdateFields{CancelledAt: &now, CancelReason: &reason})
}
