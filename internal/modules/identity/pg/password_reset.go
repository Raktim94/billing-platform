package pg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"rechvix/internal/modules/identity/domain"
	"rechvix/internal/platform/database"
)

type PasswordResetRepo struct{ pool *database.Pool }

func NewPasswordResetRepo(pool *database.Pool) *PasswordResetRepo {
	return &PasswordResetRepo{pool: pool}
}

func (r *PasswordResetRepo) Create(ctx context.Context, t *domain.PasswordResetToken) error {
	const q = `
		INSERT INTO password_reset_tokens (id, organisation_id, user_id, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := r.pool.Q(ctx).Exec(ctx, q, t.ID, t.OrganisationID, t.UserID, t.TokenHash, t.ExpiresAt, t.CreatedAt)
	if err != nil {
		return fmt.Errorf("identity: inserting password_reset_token: %w", err)
	}
	return nil
}

// GetByTokenHash runs unscoped, same reasoning as sessions — see
// migrations/0006_password_reset_tokens.up.sql.
func (r *PasswordResetRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.PasswordResetToken, error) {
	const q = `
		SELECT id, organisation_id, user_id, token_hash, expires_at, used_at, created_at
		FROM password_reset_tokens WHERE token_hash = $1`
	row := r.pool.Q(ctx).QueryRow(ctx, q, tokenHash)
	var t domain.PasswordResetToken
	if err := row.Scan(&t.ID, &t.OrganisationID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &t.UsedAt, &t.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("identity: querying password_reset_token: %w", err)
	}
	return &t, nil
}

func (r *PasswordResetRepo) MarkUsed(ctx context.Context, id uuid.UUID, at time.Time) error {
	const q = `UPDATE password_reset_tokens SET used_at = $2 WHERE id = $1 AND used_at IS NULL`
	tag, err := r.pool.Q(ctx).Exec(ctx, q, id, at)
	if err != nil {
		return fmt.Errorf("identity: marking password_reset_token used: %w", err)
	}
	if tag == 0 {
		// Already used (or never existed) — treated as invalid by the
		// caller, not silently ignored, so a race between two "use this
		// reset link" requests can only ever succeed once.
		return domain.ErrTokenInvalid
	}
	return nil
}
