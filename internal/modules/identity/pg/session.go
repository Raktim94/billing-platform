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

type SessionRepo struct{ pool *database.Pool }

func NewSessionRepo(pool *database.Pool) *SessionRepo { return &SessionRepo{pool: pool} }

func (r *SessionRepo) Create(ctx context.Context, s *domain.Session) error {
	const q = `
		INSERT INTO sessions (id, organisation_id, user_id, token_hash, created_at, last_seen_at,
			idle_expires_at, absolute_expires_at, ip, user_agent)
		VALUES ($1, $2, $3, $4, $5, $5, $6, $7, $8, $9)`
	var ip *string
	if s.IP != "" {
		ip = &s.IP
	}
	var ua *string
	if s.UserAgent != "" {
		ua = &s.UserAgent
	}
	_, err := r.pool.Q(ctx).Exec(ctx, q, s.ID, s.OrganisationID, s.UserID, s.TokenHash, s.CreatedAt,
		s.IdleExpiresAt, s.AbsoluteExpiresAt, ip, ua)
	if err != nil {
		return fmt.Errorf("identity: inserting session: %w", err)
	}
	return nil
}

// GetByTokenHash is, like auth_lookup_user_by_email, deliberately called
// against an unscoped transaction (database.Pool.Run) at the point in a
// request where organisation context isn't known yet — see
// migrations/0004_sessions.up.sql for why the sessions table itself has
// no RLS policy.
func (r *SessionRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.Session, error) {
	const q = `
		SELECT id, organisation_id, user_id, token_hash, created_at, last_seen_at,
			idle_expires_at, absolute_expires_at, revoked_at, ip, user_agent
		FROM sessions WHERE token_hash = $1`
	row := r.pool.Q(ctx).QueryRow(ctx, q, tokenHash)
	var s domain.Session
	var ip, ua *string
	if err := row.Scan(&s.ID, &s.OrganisationID, &s.UserID, &s.TokenHash, &s.CreatedAt, &s.LastSeenAt,
		&s.IdleExpiresAt, &s.AbsoluteExpiresAt, &s.RevokedAt, &ip, &ua); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("identity: querying session: %w", err)
	}
	if ip != nil {
		s.IP = *ip
	}
	if ua != nil {
		s.UserAgent = *ua
	}
	return &s, nil
}

func (r *SessionRepo) Touch(ctx context.Context, id uuid.UUID, lastSeenAt time.Time) error {
	const q = `UPDATE sessions SET last_seen_at = $2 WHERE id = $1`
	_, err := r.pool.Q(ctx).Exec(ctx, q, id, lastSeenAt)
	if err != nil {
		return fmt.Errorf("identity: touching session: %w", err)
	}
	return nil
}

func (r *SessionRepo) Revoke(ctx context.Context, id uuid.UUID, at time.Time) error {
	const q = `UPDATE sessions SET revoked_at = $2 WHERE id = $1 AND revoked_at IS NULL`
	_, err := r.pool.Q(ctx).Exec(ctx, q, id, at)
	if err != nil {
		return fmt.Errorf("identity: revoking session: %w", err)
	}
	return nil
}

func (r *SessionRepo) RevokeAllForUser(ctx context.Context, userID uuid.UUID, at time.Time) error {
	const q = `UPDATE sessions SET revoked_at = $2 WHERE user_id = $1 AND revoked_at IS NULL`
	_, err := r.pool.Q(ctx).Exec(ctx, q, userID, at)
	if err != nil {
		return fmt.Errorf("identity: revoking all sessions for user: %w", err)
	}
	return nil
}

func (r *SessionRepo) ListActiveForUser(ctx context.Context, userID uuid.UUID) ([]*domain.Session, error) {
	const q = `
		SELECT id, organisation_id, user_id, token_hash, created_at, last_seen_at,
			idle_expires_at, absolute_expires_at, revoked_at, ip, user_agent
		FROM sessions WHERE user_id = $1 AND revoked_at IS NULL ORDER BY last_seen_at DESC`
	rows, err := r.pool.Q(ctx).Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("identity: listing active sessions: %w", err)
	}
	defer rows.Close()

	var out []*domain.Session
	for rows.Next() {
		var s domain.Session
		var ip, ua *string
		if err := rows.Scan(&s.ID, &s.OrganisationID, &s.UserID, &s.TokenHash, &s.CreatedAt, &s.LastSeenAt,
			&s.IdleExpiresAt, &s.AbsoluteExpiresAt, &s.RevokedAt, &ip, &ua); err != nil {
			return nil, fmt.Errorf("identity: scanning session row: %w", err)
		}
		if ip != nil {
			s.IP = *ip
		}
		if ua != nil {
			s.UserAgent = *ua
		}
		out = append(out, &s)
	}
	return out, rows.Err()
}
