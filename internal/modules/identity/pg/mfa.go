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

type MFARepo struct{ pool *database.Pool }

func NewMFARepo(pool *database.Pool) *MFARepo { return &MFARepo{pool: pool} }

func (r *MFARepo) UpsertSecret(ctx context.Context, secret *domain.MFASecret) error {
	const q = `
		INSERT INTO mfa_secrets (user_id, organisation_id, secret_encrypted, enabled, created_at, updated_at)
		VALUES ($1, $2, $3, $4, now(), now())
		ON CONFLICT (user_id) DO UPDATE SET secret_encrypted = $3, enabled = $4, updated_at = now()`
	_, err := r.pool.Q(ctx).Exec(ctx, q, secret.UserID, secret.OrganisationID, secret.SecretEncrypted, secret.Enabled)
	if err != nil {
		return fmt.Errorf("identity: upserting mfa_secret: %w", err)
	}
	return nil
}

func (r *MFARepo) GetSecret(ctx context.Context, userID uuid.UUID) (*domain.MFASecret, error) {
	const q = `SELECT user_id, organisation_id, secret_encrypted, enabled FROM mfa_secrets WHERE user_id = $1`
	row := r.pool.Q(ctx).QueryRow(ctx, q, userID)
	var s domain.MFASecret
	if err := row.Scan(&s.UserID, &s.OrganisationID, &s.SecretEncrypted, &s.Enabled); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("identity: querying mfa_secret: %w", err)
	}
	return &s, nil
}

func (r *MFARepo) SetEnabled(ctx context.Context, userID uuid.UUID, enabled bool) error {
	const q = `UPDATE mfa_secrets SET enabled = $2, updated_at = now() WHERE user_id = $1`
	_, err := r.pool.Q(ctx).Exec(ctx, q, userID, enabled)
	if err != nil {
		return fmt.Errorf("identity: updating mfa_secret enabled: %w", err)
	}
	return nil
}

func (r *MFARepo) ReplaceRecoveryCodes(ctx context.Context, orgID, userID uuid.UUID, codeHashes []string) error {
	q := r.pool.Q(ctx)
	if _, err := q.Exec(ctx, `DELETE FROM mfa_recovery_codes WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("identity: clearing old recovery codes: %w", err)
	}
	for _, hash := range codeHashes {
		id, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("identity: generating recovery code id: %w", err)
		}
		const insert = `INSERT INTO mfa_recovery_codes (id, organisation_id, user_id, code_hash, created_at) VALUES ($1, $2, $3, $4, now())`
		if _, err := q.Exec(ctx, insert, id, orgID, userID, hash); err != nil {
			return fmt.Errorf("identity: inserting recovery code: %w", err)
		}
	}
	return nil
}

func (r *MFARepo) ConsumeRecoveryCode(ctx context.Context, userID uuid.UUID, codeHash string, at time.Time) (bool, error) {
	// Single UPDATE ... WHERE used_at IS NULL, checked via rows-affected,
	// is atomic under Postgres's row-level locking — two concurrent
	// requests racing to consume the same recovery code cannot both
	// succeed.
	const q = `UPDATE mfa_recovery_codes SET used_at = $3 WHERE user_id = $1 AND code_hash = $2 AND used_at IS NULL`
	tag, err := r.pool.Q(ctx).Exec(ctx, q, userID, codeHash, at)
	if err != nil {
		return false, fmt.Errorf("identity: consuming recovery code: %w", err)
	}
	return tag > 0, nil
}
