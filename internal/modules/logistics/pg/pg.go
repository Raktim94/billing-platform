// Package pg is logistics' PostgreSQL repository implementation — same
// database.Pool.Q(ctx) convention as every other module (see
// organisation/pg's package doc).
package pg

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"billing-platform/internal/modules/logistics/domain"
	"billing-platform/internal/platform/database"
)

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
func strOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// --- Vehicles ---

type VehicleRepo struct{ pool *database.Pool }

func NewVehicleRepo(pool *database.Pool) *VehicleRepo { return &VehicleRepo{pool: pool} }

func (r *VehicleRepo) Create(ctx context.Context, v *domain.Vehicle) error {
	const q = `
		INSERT INTO vehicles (id, organisation_id, registration_number, nickname, vehicle_type, default_transport_mode, active, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8)`
	_, err := r.pool.Q(ctx).Exec(ctx, q, v.ID, v.OrganisationID, v.RegistrationNumber,
		nullIfEmpty(v.Nickname), nullIfEmpty(v.VehicleType), nullIfEmpty(v.DefaultTransportMode), v.Active, v.CreatedAt)
	if err != nil {
		return fmt.Errorf("logistics: inserting vehicle: %w", err)
	}
	return nil
}

func scanVehicle(row pgx.Row) (*domain.Vehicle, error) {
	var v domain.Vehicle
	var nickname, vehicleType, mode *string
	if err := row.Scan(&v.ID, &v.OrganisationID, &v.RegistrationNumber, &nickname, &vehicleType, &mode, &v.Active, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	v.Nickname, v.VehicleType, v.DefaultTransportMode = strOrEmpty(nickname), strOrEmpty(vehicleType), strOrEmpty(mode)
	return &v, nil
}

const vehicleCols = "id, organisation_id, registration_number, nickname, vehicle_type, default_transport_mode, active, created_at, updated_at"

func (r *VehicleRepo) GetByID(ctx context.Context, orgID, id uuid.UUID) (*domain.Vehicle, error) {
	row := r.pool.Q(ctx).QueryRow(ctx, `SELECT `+vehicleCols+` FROM vehicles WHERE organisation_id = $1 AND id = $2`, orgID, id)
	v, err := scanVehicle(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("logistics: querying vehicle: %w", err)
	}
	return v, nil
}

func (r *VehicleRepo) ListByOrganisation(ctx context.Context, orgID uuid.UUID, activeOnly bool) ([]*domain.Vehicle, error) {
	q := `SELECT ` + vehicleCols + ` FROM vehicles WHERE organisation_id = $1`
	if activeOnly {
		q += ` AND active = true`
	}
	q += ` ORDER BY registration_number`
	rows, err := r.pool.Q(ctx).Query(ctx, q, orgID)
	if err != nil {
		return nil, fmt.Errorf("logistics: listing vehicles: %w", err)
	}
	defer rows.Close()
	var out []*domain.Vehicle
	for rows.Next() {
		v, err := scanVehicle(rows)
		if err != nil {
			return nil, fmt.Errorf("logistics: scanning vehicle: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *VehicleRepo) Deactivate(ctx context.Context, orgID, id uuid.UUID) error {
	n, err := r.pool.Q(ctx).Exec(ctx, `UPDATE vehicles SET active = false, updated_at = now() WHERE organisation_id = $1 AND id = $2`, orgID, id)
	if err != nil {
		return fmt.Errorf("logistics: deactivating vehicle: %w", err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// --- Transporters ---

type TransporterRepo struct{ pool *database.Pool }

func NewTransporterRepo(pool *database.Pool) *TransporterRepo { return &TransporterRepo{pool: pool} }

func (r *TransporterRepo) Create(ctx context.Context, t *domain.Transporter) error {
	const q = `
		INSERT INTO transporters (id, organisation_id, name, transporter_id, gstin, phone, address, default_transport_mode, active, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$10)`
	_, err := r.pool.Q(ctx).Exec(ctx, q, t.ID, t.OrganisationID, t.Name,
		nullIfEmpty(t.TransporterID), nullIfEmpty(t.GSTIN), nullIfEmpty(t.Phone), nullIfEmpty(t.Address),
		nullIfEmpty(t.DefaultTransportMode), t.Active, t.CreatedAt)
	if err != nil {
		return fmt.Errorf("logistics: inserting transporter: %w", err)
	}
	return nil
}

func scanTransporter(row pgx.Row) (*domain.Transporter, error) {
	var t domain.Transporter
	var transporterID, gstin, phone, address, mode *string
	if err := row.Scan(&t.ID, &t.OrganisationID, &t.Name, &transporterID, &gstin, &phone, &address, &mode, &t.Active, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	t.TransporterID, t.GSTIN, t.Phone, t.Address, t.DefaultTransportMode =
		strOrEmpty(transporterID), strOrEmpty(gstin), strOrEmpty(phone), strOrEmpty(address), strOrEmpty(mode)
	return &t, nil
}

const transporterCols = "id, organisation_id, name, transporter_id, gstin, phone, address, default_transport_mode, active, created_at, updated_at"

func (r *TransporterRepo) GetByID(ctx context.Context, orgID, id uuid.UUID) (*domain.Transporter, error) {
	row := r.pool.Q(ctx).QueryRow(ctx, `SELECT `+transporterCols+` FROM transporters WHERE organisation_id = $1 AND id = $2`, orgID, id)
	t, err := scanTransporter(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("logistics: querying transporter: %w", err)
	}
	return t, nil
}

func (r *TransporterRepo) ListByOrganisation(ctx context.Context, orgID uuid.UUID, activeOnly bool) ([]*domain.Transporter, error) {
	q := `SELECT ` + transporterCols + ` FROM transporters WHERE organisation_id = $1`
	if activeOnly {
		q += ` AND active = true`
	}
	q += ` ORDER BY name`
	rows, err := r.pool.Q(ctx).Query(ctx, q, orgID)
	if err != nil {
		return nil, fmt.Errorf("logistics: listing transporters: %w", err)
	}
	defer rows.Close()
	var out []*domain.Transporter
	for rows.Next() {
		t, err := scanTransporter(rows)
		if err != nil {
			return nil, fmt.Errorf("logistics: scanning transporter: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *TransporterRepo) Deactivate(ctx context.Context, orgID, id uuid.UUID) error {
	n, err := r.pool.Q(ctx).Exec(ctx, `UPDATE transporters SET active = false, updated_at = now() WHERE organisation_id = $1 AND id = $2`, orgID, id)
	if err != nil {
		return fmt.Errorf("logistics: deactivating transporter: %w", err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// --- Preferences ("smart defaults") ---

type PreferenceRepo struct{ pool *database.Pool }

func NewPreferenceRepo(pool *database.Pool) *PreferenceRepo { return &PreferenceRepo{pool: pool} }

func (r *PreferenceRepo) RecordUsage(ctx context.Context, orgID, customerPartyID uuid.UUID, vehicleID, transporterID *uuid.UUID) error {
	// vehicle_id/transporter_id NULLs don't collide under a UNIQUE
	// constraint the way two non-NULL equal values would — Postgres
	// treats each NULL as distinct — so this upsert is only meaningful
	// (and only called) when at least one of the two is set; a row with
	// both NULL would never conflict with itself on a second call. That's
	// an acceptable edge case here: "record usage with nothing to
	// remember" isn't a real caller scenario.
	const q = `
		INSERT INTO customer_transport_preferences (id, organisation_id, customer_party_id, vehicle_id, transporter_id, used_count, last_used_at)
		VALUES ($1,$2,$3,$4,$5,1,now())
		ON CONFLICT (organisation_id, customer_party_id, vehicle_id, transporter_id)
		DO UPDATE SET used_count = customer_transport_preferences.used_count + 1, last_used_at = now()`
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("logistics: generating preference id: %w", err)
	}
	_, err = r.pool.Q(ctx).Exec(ctx, q, id, orgID, customerPartyID, vehicleID, transporterID)
	if err != nil {
		return fmt.Errorf("logistics: recording transport preference: %w", err)
	}
	return nil
}

func (r *PreferenceRepo) MostRecent(ctx context.Context, orgID, customerPartyID uuid.UUID) (*domain.Preference, error) {
	const q = `
		SELECT id, organisation_id, customer_party_id, vehicle_id, transporter_id, used_count, last_used_at
		FROM customer_transport_preferences
		WHERE organisation_id = $1 AND customer_party_id = $2
		ORDER BY last_used_at DESC LIMIT 1`
	var p domain.Preference
	err := r.pool.Q(ctx).QueryRow(ctx, q, orgID, customerPartyID).Scan(
		&p.ID, &p.OrganisationID, &p.CustomerPartyID, &p.VehicleID, &p.TransporterID, &p.UsedCount, &p.LastUsedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("logistics: querying transport preference: %w", err)
	}
	return &p, nil
}
