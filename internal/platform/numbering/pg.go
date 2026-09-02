package numbering

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"billing-platform/internal/platform/database"
)

type PGRepository struct{ pool *database.Pool }

func NewPGRepository(pool *database.Pool) *PGRepository { return &PGRepository{pool: pool} }

var _ Repository = (*PGRepository)(nil)

// Next mirrors purchases' NextNumber pattern (purchases/pg/pg.go) exactly,
// extended with the branch/financial_year scoping and configurable
// prefix this shared version supports. The first call for a given scope
// inserts the counter row already at 2 and returns 1; every later call
// hits DO UPDATE and returns the post-increment value. The row lock taken
// by the INSERT/UPDATE is what makes two concurrent callers serialize
// instead of both computing the same "next" value (Scenario I).
func (r *PGRepository) Next(ctx context.Context, orgID, branchID uuid.UUID, documentType, financialYear, prefix string) (int64, error) {
	const q = `
		INSERT INTO document_number_counters (organisation_id, branch_id, document_type, financial_year, prefix, next_number)
		VALUES ($1, $2, $3, $4, $5, 2)
		ON CONFLICT (organisation_id, branch_id, document_type, financial_year)
		DO UPDATE SET next_number = document_number_counters.next_number + 1
		RETURNING next_number - 1`
	var allocated int64
	row := r.pool.Q(ctx).QueryRow(ctx, q, orgID, branchID, documentType, financialYear, prefix)
	if err := row.Scan(&allocated); err != nil {
		return 0, fmt.Errorf("numbering: allocating next number: %w", err)
	}
	return allocated, nil
}
