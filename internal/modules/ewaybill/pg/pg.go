package pg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"billing-platform/internal/modules/ewaybill/domain"
	"billing-platform/internal/platform/database"
)

type RecordRepo struct {
	pool *database.Pool
}

func NewRecordRepo(pool *database.Pool) *RecordRepo {
	return &RecordRepo{pool: pool}
}

var _ domain.Repository = (*RecordRepo)(nil)

const selectCols = `id, organisation_id, sales_document_id, einvoice_record_id, irn, ewb_number, status,
	valid_from, valid_until, ship_to_gstin, transporter_id, transporter_name, vehicle_number,
	distance_km, part_b_history, closed_at, closed_by_role, error_code, error_message,
	correlation_id, cancelled_at, cancel_reason, created_at, updated_at`

func scanRecord(row pgx.Row) (*domain.Record, error) {
	var r domain.Record
	var status string
	var closedByRole *string
	var partBRaw []byte
	var distance *string
	err := row.Scan(&r.ID, &r.OrganisationID, &r.SalesDocumentID, &r.EInvoiceRecordID, &r.IRN, &r.EWBNumber, &status,
		&r.ValidFrom, &r.ValidUntil, &r.ShipToGSTIN, &r.TransporterID, &r.TransporterName, &r.VehicleNumber,
		&distance, &partBRaw, &r.ClosedAt, &closedByRole, &r.ErrorCode, &r.ErrorMessage,
		&r.CorrelationID, &r.CancelledAt, &r.CancelReason, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	r.Status = domain.Status(status)
	r.DistanceKM = distance
	if closedByRole != nil {
		role := domain.ClosedByRole(*closedByRole)
		r.ClosedByRole = &role
	}
	if len(partBRaw) > 0 {
		if err := json.Unmarshal(partBRaw, &r.PartBHistory); err != nil {
			return nil, fmt.Errorf("ewaybill: unmarshaling part_b_history: %w", err)
		}
	}
	return &r, nil
}

func (repo *RecordRepo) GetBySalesDocumentID(ctx context.Context, salesDocumentID uuid.UUID) (*domain.Record, error) {
	row := repo.pool.Q(ctx).QueryRow(ctx, `SELECT `+selectCols+` FROM ewaybill_records WHERE sales_document_id = $1
		ORDER BY created_at DESC LIMIT 1`, salesDocumentID)
	r, err := scanRecord(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("ewaybill: querying record by sales_document_id: %w", err)
	}
	return r, nil
}

func (repo *RecordRepo) Create(ctx context.Context, r *domain.Record) error {
	const q = `
		INSERT INTO ewaybill_records
			(id, organisation_id, sales_document_id, einvoice_record_id, irn, status,
			 transporter_id, transporter_name, vehicle_number, distance_km, ship_to_gstin)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`
	_, err := repo.pool.Q(ctx).Exec(ctx, q, r.ID, r.OrganisationID, r.SalesDocumentID, r.EInvoiceRecordID, r.IRN, string(r.Status),
		r.TransporterID, r.TransporterName, r.VehicleNumber, r.DistanceKM, r.ShipToGSTIN)
	if err != nil {
		return fmt.Errorf("ewaybill: inserting record: %w", err)
	}
	return nil
}

func (repo *RecordRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.Status, f domain.UpdateFields) error {
	var closedByRole *string
	if f.ClosedByRole != nil {
		s := string(*f.ClosedByRole)
		closedByRole = &s
	}
	const q = `
		UPDATE ewaybill_records SET
			status = $2,
			ewb_number = COALESCE($3, ewb_number),
			valid_from = COALESCE($4, valid_from),
			valid_until = COALESCE($5, valid_until),
			ship_to_gstin = COALESCE($6, ship_to_gstin),
			closed_at = COALESCE($7, closed_at),
			closed_by_role = COALESCE($8, closed_by_role),
			error_code = COALESCE($9, error_code),
			error_message = COALESCE($10, error_message),
			correlation_id = COALESCE($11, correlation_id),
			cancelled_at = COALESCE($12, cancelled_at),
			cancel_reason = COALESCE($13, cancel_reason),
			updated_at = now()
		WHERE id = $1`
	n, err := repo.pool.Q(ctx).Exec(ctx, q, id, string(status),
		f.EWBNumber, f.ValidFrom, f.ValidUntil, f.ShipToGSTIN, f.ClosedAt, closedByRole,
		f.ErrorCode, f.ErrorMessage, f.CorrelationID, f.CancelledAt, f.CancelReason)
	if err != nil {
		return fmt.Errorf("ewaybill: updating record status: %w", err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (repo *RecordRepo) AppendPartBHistory(ctx context.Context, id uuid.UUID, entry domain.PartBUpdate) error {
	body, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("ewaybill: marshaling part-B entry: %w", err)
	}
	const q = `UPDATE ewaybill_records SET part_b_history = part_b_history || $2::jsonb, updated_at = now() WHERE id = $1`
	n, err := repo.pool.Q(ctx).Exec(ctx, q, id, body)
	if err != nil {
		return fmt.Errorf("ewaybill: appending part-B history: %w", err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}
