// Package pg is the gstindia module's PostgreSQL repository implementation.
package pg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"billing-platform/internal/modules/gstindia/domain"
	"billing-platform/internal/platform/database"
)

type scannable interface {
	Scan(dest ...any) error
}

// --- tax_rate_master ---

type TaxRateRepo struct{ pool *database.Pool }

func NewTaxRateRepo(pool *database.Pool) *TaxRateRepo { return &TaxRateRepo{pool: pool} }

func (r *TaxRateRepo) Create(ctx context.Context, m *domain.TaxRateMaster) error {
	const q = `
		INSERT INTO tax_rate_master (id, organisation_id, country_code, hsn_sac_code, classification, gst_rate, cess_rate, valid_from, valid_to, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`
	_, err := r.pool.Q(ctx).Exec(ctx, q, m.ID, m.OrganisationID, m.CountryCode, m.HSNSACCode, string(m.Classification), m.GSTRate, m.CessRate, m.ValidFrom, m.ValidTo, m.CreatedAt)
	if err != nil {
		return fmt.Errorf("gstindia: inserting tax_rate_master: %w", err)
	}
	return nil
}

// Resolve returns the row whose validity window covers asOf, preferring
// the most recently-started window if (pathologically) more than one
// matches. See migrations/0015_gstindia.up.sql's comment on why overlap
// isn't prevented by a database constraint.
func (r *TaxRateRepo) Resolve(ctx context.Context, orgID uuid.UUID, countryCode, hsnSacCode string, asOf time.Time) (*domain.TaxRateMaster, error) {
	const q = `
		SELECT id, organisation_id, country_code, hsn_sac_code, classification, gst_rate, cess_rate, valid_from, valid_to, created_at
		FROM tax_rate_master
		WHERE organisation_id = $1 AND country_code = $2 AND hsn_sac_code = $3
			AND valid_from <= $4 AND (valid_to IS NULL OR valid_to >= $4)
		ORDER BY valid_from DESC
		LIMIT 1`
	row := r.pool.Q(ctx).QueryRow(ctx, q, orgID, countryCode, hsnSacCode, asOf)
	m, err := scanTaxRateMaster(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return m, nil
}

func (r *TaxRateRepo) ListByHSN(ctx context.Context, orgID uuid.UUID, countryCode, hsnSacCode string) ([]*domain.TaxRateMaster, error) {
	const q = `
		SELECT id, organisation_id, country_code, hsn_sac_code, classification, gst_rate, cess_rate, valid_from, valid_to, created_at
		FROM tax_rate_master WHERE organisation_id = $1 AND country_code = $2 AND hsn_sac_code = $3 ORDER BY valid_from DESC`
	rows, err := r.pool.Q(ctx).Query(ctx, q, orgID, countryCode, hsnSacCode)
	if err != nil {
		return nil, fmt.Errorf("gstindia: listing tax_rate_master: %w", err)
	}
	defer rows.Close()
	var out []*domain.TaxRateMaster
	for rows.Next() {
		m, err := scanTaxRateMaster(rows)
		if err != nil {
			return nil, fmt.Errorf("gstindia: scanning tax_rate_master: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func scanTaxRateMaster(row scannable) (*domain.TaxRateMaster, error) {
	var m domain.TaxRateMaster
	var classification string
	var gstRate, cessRate decimal.Decimal
	err := row.Scan(&m.ID, &m.OrganisationID, &m.CountryCode, &m.HSNSACCode, &classification, &gstRate, &cessRate, &m.ValidFrom, &m.ValidTo, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	m.Classification = domain.RateClassification(classification)
	m.GSTRate = gstRate
	m.CessRate = cessRate
	return &m, nil
}

// --- gst_state_codes (global reference data, no organisation_id, no RLS) ---

type StateRepo struct{ pool *database.Pool }

func NewStateRepo(pool *database.Pool) *StateRepo { return &StateRepo{pool: pool} }

func (r *StateRepo) ListAll(ctx context.Context) ([]domain.GSTState, error) {
	const q = `SELECT code, name, is_union_territory FROM gst_state_codes ORDER BY name`
	rows, err := r.pool.Q(ctx).Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("gstindia: listing state codes: %w", err)
	}
	defer rows.Close()
	var out []domain.GSTState
	for rows.Next() {
		var s domain.GSTState
		if err := rows.Scan(&s.Code, &s.Name, &s.IsUnionTerritory); err != nil {
			return nil, fmt.Errorf("gstindia: scanning state code: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *StateRepo) GetByCode(ctx context.Context, code string) (*domain.GSTState, error) {
	const q = `SELECT code, name, is_union_territory FROM gst_state_codes WHERE code = $1`
	var s domain.GSTState
	err := r.pool.Q(ctx).QueryRow(ctx, q, code).Scan(&s.Code, &s.Name, &s.IsUnionTerritory)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUnknownStateCode
		}
		return nil, fmt.Errorf("gstindia: resolving state code %q: %w", code, err)
	}
	return &s, nil
}
