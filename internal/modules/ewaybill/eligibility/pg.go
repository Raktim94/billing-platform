package eligibility

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"

	"billing-platform/internal/platform/database"
)

// PGRepository reads ewaybill_eligibility_rules — a GLOBAL table (no
// organisation_id, no RLS), same reasoning as Stage 5a's gst_state_codes:
// this is government-set reference data, not per-tenant configuration.
type PGRepository struct{ pool *database.Pool }

func NewPGRepository(pool *database.Pool) *PGRepository { return &PGRepository{pool: pool} }

var _ Repository = (*PGRepository)(nil)

func (r *PGRepository) ListActive(ctx context.Context) ([]Rule, error) {
	const q = `SELECT state_code, min_consignment_value, valid_from, valid_until FROM ewaybill_eligibility_rules`
	rows, err := r.pool.Q(ctx).Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("eligibility: listing rules: %w", err)
	}
	defer rows.Close()
	var out []Rule
	for rows.Next() {
		var rule Rule
		var minValue decimal.Decimal
		if err := rows.Scan(&rule.StateCode, &minValue, &rule.ValidFrom, &rule.ValidUntil); err != nil {
			return nil, fmt.Errorf("eligibility: scanning rule: %w", err)
		}
		rule.MinConsignmentValue = minValue
		out = append(out, rule)
	}
	return out, rows.Err()
}
