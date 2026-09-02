// Package database owns the PostgreSQL connection pool and the migration
// runner. Nothing outside this package (and the sqlc/hand-written
// query layers built on top of it) issues a raw SQL connection.
package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	pgxdecimal "github.com/jackc/pgx-shopspring-decimal"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool wraps *pgxpool.Pool. A thin wrapper (not a bare pgxpool.Pool alias)
// so this package can add health-check and future instrumentation without
// every caller depending on pgxpool directly.
type Pool struct {
	*pgxpool.Pool
}

// Config is the subset of platform config this package needs. Kept
// independent of internal/platform/config's Config type so this package
// has no import-time dependency on it — the composition root (apps/server)
// is the only place that bridges the two.
type Config struct {
	DSN      string
	MaxConns int32
}

// NewPool builds and validates a connection pool. It pings the database
// once before returning, so a misconfigured DSN fails at startup rather
// than on the first request.
func NewPool(ctx context.Context, cfg Config) (*Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("database: parsing DSN: %w", err)
	}
	if cfg.MaxConns > 0 {
		poolCfg.MaxConns = cfg.MaxConns
	}
	poolCfg.MaxConnLifetime = time.Hour
	poolCfg.MaxConnIdleTime = 30 * time.Minute
	poolCfg.HealthCheckPeriod = time.Minute

	// Registers NUMERIC <-> shopspring/decimal.Decimal scan/encode support
	// on every pooled connection, so repository code can Scan a NUMERIC
	// column directly into a decimal.Decimal (and, wrapped by
	// internal/platform/money, into a Money value) without a manual
	// text-cast round trip. Money is the only package permitted to
	// construct a Money from the resulting decimal (brief §6/§56).
	poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		pgxdecimal.Register(conn.TypeMap())
		return nil
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("database: creating pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database: initial ping failed: %w", err)
	}

	return &Pool{Pool: pool}, nil
}

// Ready reports whether the database is currently reachable. Used by the
// /health/ready endpoint (brief §58) — readiness, unlike liveness, must
// reflect real dependency health.
func (p *Pool) Ready(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return p.Ping(ctx)
}

// WarnIfRuntimeRoleOwnsTenantTables logs a startup warning if the
// connected role owns the `users` table. Per the deployment-requirement
// comment in migrations/0001_organisation_hierarchy.up.sql, the runtime
// role owning its own tenant tables silently defeats every Row-Level
// Security policy in the schema (RLS's owner-exemption applies, and this
// schema does not use FORCE ROW LEVEL SECURITY, by design, to allow the
// narrow SECURITY DEFINER auth-lookup bypass). This is a best-effort
// diagnostic, not an enforcement mechanism — it does not block startup,
// because a local/dev single-role setup is a legitimate, explicit choice
// as long as the operator understands the trade-off.
func (p *Pool) WarnIfRuntimeRoleOwnsTenantTables(ctx context.Context, logger *slog.Logger) error {
	const q = `
		SELECT current_user = (
			SELECT pg_get_userbyid(relowner)
			FROM pg_class
			WHERE relname = 'users' AND relnamespace = 'public'::regnamespace
		)`
	var isOwner bool
	if err := p.QueryRow(ctx, q).Scan(&isOwner); err != nil {
		// The users table may not exist yet (pre-migration); that's not
		// this check's problem to report as an error.
		if errors.Is(err, context.Canceled) {
			return err
		}
		logger.WarnContext(ctx, "database: could not verify runtime role ownership (users table may not exist yet)", "error", err)
		return nil
	}
	if isOwner {
		logger.WarnContext(ctx, "database: the runtime DB role owns the `users` table — every Row-Level Security "+
			"tenant-isolation policy in this schema is silently bypassed for this connection (RLS owner-exemption). "+
			"See the deployment-requirement comment in migrations/0001_organisation_hierarchy.up.sql: provision a "+
			"separate, non-owning role for apps/server and apps/worker.")
	}
	return nil
}
