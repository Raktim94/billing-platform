package pg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"billing-platform/internal/modules/identity/domain"
	"billing-platform/internal/platform/database"
)

type APIKeyRepo struct{ pool *database.Pool }

func NewAPIKeyRepo(pool *database.Pool) *APIKeyRepo { return &APIKeyRepo{pool: pool} }

func (r *APIKeyRepo) Create(ctx context.Context, k *domain.APIKey) error {
	const q = `
		INSERT INTO api_keys (id, organisation_id, user_id, name, key_prefix, key_hash, scopes,
			expires_at, allowed_ip, created_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`
	_, err := r.pool.Q(ctx).Exec(ctx, q, k.ID, k.OrganisationID, k.UserID, k.Name, k.KeyPrefix, k.KeyHash,
		scopesToText(k.Scopes), k.ExpiresAt, k.AllowedIP, k.CreatedAt, k.CreatedBy)
	if err != nil {
		return fmt.Errorf("identity: inserting api key: %w", err)
	}
	return nil
}

// GetByHash is, like SessionRepo.GetByTokenHash, deliberately called
// against an unscoped transaction — see migrations/0025_api_keys.up.sql.
func (r *APIKeyRepo) GetByHash(ctx context.Context, keyHash string) (*domain.APIKey, error) {
	const q = `
		SELECT id, organisation_id, user_id, name, key_prefix, key_hash, scopes,
			expires_at, allowed_ip, last_used_at, revoked_at, created_at, created_by
		FROM api_keys WHERE key_hash = $1`
	row := r.pool.Q(ctx).QueryRow(ctx, q, keyHash)
	return scanAPIKey(row)
}

func (r *APIKeyRepo) Touch(ctx context.Context, id uuid.UUID, at time.Time) error {
	_, err := r.pool.Q(ctx).Exec(ctx, `UPDATE api_keys SET last_used_at = $2 WHERE id = $1`, id, at)
	if err != nil {
		return fmt.Errorf("identity: touching api key: %w", err)
	}
	return nil
}

func (r *APIKeyRepo) Revoke(ctx context.Context, id uuid.UUID, at time.Time) error {
	_, err := r.pool.Q(ctx).Exec(ctx,
		`UPDATE api_keys SET revoked_at = $2 WHERE id = $1 AND revoked_at IS NULL`, id, at)
	if err != nil {
		return fmt.Errorf("identity: revoking api key: %w", err)
	}
	return nil
}

func (r *APIKeyRepo) ListActiveForOrganisation(ctx context.Context, organisationID uuid.UUID) ([]*domain.APIKey, error) {
	const q = `
		SELECT id, organisation_id, user_id, name, key_prefix, key_hash, scopes,
			expires_at, allowed_ip, last_used_at, revoked_at, created_at, created_by
		FROM api_keys WHERE organisation_id = $1 AND revoked_at IS NULL ORDER BY created_at DESC`
	rows, err := r.pool.Q(ctx).Query(ctx, q, organisationID)
	if err != nil {
		return nil, fmt.Errorf("identity: listing api keys: %w", err)
	}
	defer rows.Close()

	var out []*domain.APIKey
	for rows.Next() {
		k, err := scanAPIKeyRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAPIKey(row rowScanner) (*domain.APIKey, error) {
	k, err := scanAPIKeyRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return k, nil
}

func scanAPIKeyRow(row rowScanner) (*domain.APIKey, error) {
	var k domain.APIKey
	var scopes []string
	if err := row.Scan(&k.ID, &k.OrganisationID, &k.UserID, &k.Name, &k.KeyPrefix, &k.KeyHash, &scopes,
		&k.ExpiresAt, &k.AllowedIP, &k.LastUsedAt, &k.RevokedAt, &k.CreatedAt, &k.CreatedBy); err != nil {
		return nil, fmt.Errorf("identity: scanning api key row: %w", err)
	}
	k.Scopes = textToScopes(scopes)
	return &k, nil
}

func scopesToText(scopes []domain.APIScope) []string {
	out := make([]string, len(scopes))
	for i, s := range scopes {
		out[i] = string(s)
	}
	return out
}

func textToScopes(text []string) []domain.APIScope {
	out := make([]domain.APIScope, len(text))
	for i, s := range text {
		out[i] = domain.APIScope(s)
	}
	return out
}
