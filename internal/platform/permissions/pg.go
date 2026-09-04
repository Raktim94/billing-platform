package permissions

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"rechvix/internal/platform/database"
)

// PGStore loads grants from role_permissions joined through user_roles.
// Queries run through database.Pool.Q(ctx), so they participate in
// whatever transaction/organisation scope the caller's request already
// established (database.Pool.RunScoped) — this table join is itself
// subject to user_roles' and roles' RLS policies, so a bug that somehow
// called this with the wrong organisation scope set would come back
// empty rather than leaking another tenant's grants.
type PGStore struct {
	pool *database.Pool
}

func NewPGStore(pool *database.Pool) *PGStore {
	return &PGStore{pool: pool}
}

func (s *PGStore) Grants(ctx context.Context, userID uuid.UUID) ([]Grant, error) {
	const q = `
		SELECT rp.permission_code, ur.legal_entity_id, ur.branch_id, ur.warehouse_id
		FROM user_roles ur
		JOIN role_permissions rp ON rp.role_id = ur.role_id
		WHERE ur.user_id = $1`

	rows, err := s.pool.Q(ctx).Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("permissions: querying grants: %w", err)
	}
	defer rows.Close()

	var grants []Grant
	for rows.Next() {
		var g Grant
		if err := rows.Scan(&g.PermissionCode, &g.LegalEntityID, &g.BranchID, &g.WarehouseID); err != nil {
			return nil, fmt.Errorf("permissions: scanning grant row: %w", err)
		}
		grants = append(grants, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("permissions: iterating grant rows: %w", err)
	}
	return grants, nil
}
