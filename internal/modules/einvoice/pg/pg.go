package pg

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"billing-platform/internal/modules/einvoice/domain"
	"billing-platform/internal/platform/database"
)

type RecordRepo struct {
	pool *database.Pool
}

func NewRecordRepo(pool *database.Pool) *RecordRepo {
	return &RecordRepo{pool: pool}
}

var _ domain.Repository = (*RecordRepo)(nil)

const selectCols = `id, organisation_id, sales_document_id, provider, status, request_version,
	request_payload_hash, response_payload, irn, ack_number, ack_date, signed_invoice,
	signed_qr_payload, error_code, error_message, correlation_id, cancelled_at, cancel_reason,
	created_at, updated_at`

func scanRecord(row pgx.Row) (*domain.Record, error) {
	var r domain.Record
	var status string
	err := row.Scan(&r.ID, &r.OrganisationID, &r.SalesDocumentID, &r.Provider, &status, &r.RequestVersion,
		&r.RequestHash, &r.ResponsePayload, &r.IRN, &r.AckNumber, &r.AckDate, &r.SignedInvoice,
		&r.SignedQRPayload, &r.ErrorCode, &r.ErrorMessage, &r.CorrelationID, &r.CancelledAt, &r.CancelReason,
		&r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	r.Status = domain.Status(status)
	return &r, nil
}

func (repo *RecordRepo) GetBySalesDocumentID(ctx context.Context, salesDocumentID uuid.UUID) (*domain.Record, error) {
	row := repo.pool.Q(ctx).QueryRow(ctx,
		`SELECT `+selectCols+` FROM einvoice_records WHERE sales_document_id = $1`, salesDocumentID)
	r, err := scanRecord(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("einvoice: querying record by sales_document_id: %w", err)
	}
	return r, nil
}

func (repo *RecordRepo) Create(ctx context.Context, r *domain.Record) error {
	const q = `
		INSERT INTO einvoice_records
			(id, organisation_id, sales_document_id, provider, status, request_version)
		VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := repo.pool.Q(ctx).Exec(ctx, q, r.ID, r.OrganisationID, r.SalesDocumentID, r.Provider, string(r.Status), r.RequestVersion)
	if err != nil {
		return fmt.Errorf("einvoice: inserting record: %w", err)
	}
	return nil
}

// UpdateStatus applies a status transition plus any non-nil UpdateFields —
// nil fields use COALESCE to leave the existing column value untouched
// (never silently reset to NULL on a partial update, same convention as
// every other module's UpdateFields-shaped call).
func (repo *RecordRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.Status, f domain.UpdateFields) error {
	const q = `
		UPDATE einvoice_records SET
			status = $2,
			response_payload = COALESCE($3, response_payload),
			irn = COALESCE($4, irn),
			ack_number = COALESCE($5, ack_number),
			ack_date = COALESCE($6, ack_date),
			signed_invoice = COALESCE($7, signed_invoice),
			signed_qr_payload = COALESCE($8, signed_qr_payload),
			error_code = COALESCE($9, error_code),
			error_message = COALESCE($10, error_message),
			correlation_id = COALESCE($11, correlation_id),
			cancelled_at = COALESCE($12, cancelled_at),
			cancel_reason = COALESCE($13, cancel_reason),
			updated_at = now()
		WHERE id = $1`
	n, err := repo.pool.Q(ctx).Exec(ctx, q, id, string(status),
		nilIfEmptyBytes(f.ResponsePayload), f.IRN, f.AckNumber, f.AckDate, f.SignedInvoice, f.SignedQRPayload,
		f.ErrorCode, f.ErrorMessage, f.CorrelationID, f.CancelledAt, f.CancelReason)
	if err != nil {
		return fmt.Errorf("einvoice: updating record status: %w", err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func nilIfEmptyBytes(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	return b
}
