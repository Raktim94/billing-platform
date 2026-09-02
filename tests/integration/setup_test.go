//go:build integration

// Package integration runs Stage 2's integration tests against a real
// PostgreSQL 18 container (Testcontainers) — no mocks for the database
// layer, per brief §65. Run with:
//
//	go test ./tests/integration/... -tags=integration -v
//
// Excluded from the default `go test ./...` run (which stays fast and
// Docker-free) by the integration build tag.
package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"billing-platform/internal/platform/database"
	"billing-platform/migrations"
)

// sharedPool is set up once for the whole integration test binary
// (TestMain) rather than per-test, since spinning up a fresh Postgres
// container per test would make this suite unpleasantly slow. Isolation
// between tests comes from each test using its own freshly generated
// UUIDs (organisations, users, ...), not from a clean database per test.
//
// Deliberately connects as a NON-owning role (billing_app), not the role
// that ran migrations — see the deployment-requirement comment in
// migrations/0001_organisation_hierarchy.up.sql. The first version of
// this suite connected as the migration/owner role for everything, which
// made every RLS test pass FOR THE WRONG REASON: Postgres exempts a
// table's owner from its own RLS policies, so the tests weren't
// exercising RLS at all, they were exercising owner-bypass. Provisioning
// a second role here is what makes TestRLS_* actually test what their
// names claim.
var sharedPool *database.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:18",
		tcpostgres.WithDatabase("billing_test"),
		tcpostgres.WithUsername("billing_migrator"),
		tcpostgres.WithPassword("billing_migrator"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		panic("starting postgres container: " + err.Error())
	}
	defer func() { _ = container.Terminate(ctx) }()

	migratorDSN, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic("getting connection string: " + err.Error())
	}

	if err := database.Migrate(migratorDSN, migrations.FS); err != nil {
		panic("applying migrations: " + err.Error())
	}

	if err := provisionAppRole(ctx, migratorDSN); err != nil {
		panic("provisioning billing_app role: " + err.Error())
	}

	appDSN := strings.Replace(migratorDSN, "billing_migrator:billing_migrator@", "billing_app:billing_app_pw@", 1)

	pool, err := database.NewPool(ctx, database.Config{DSN: appDSN, MaxConns: 10})
	if err != nil {
		panic("connecting pool: " + err.Error())
	}
	defer pool.Close()

	sharedPool = pool

	m.Run()
}

// provisionAppRole creates the non-owning runtime role and grants it
// exactly the privileges apps/server/apps/worker need: DML on every
// table, and EXECUTE on the one SECURITY DEFINER bootstrap function.
// A real deployment does this once, as part of provisioning (documented
// in migrations/0001_organisation_hierarchy.up.sql), not on every
// process startup — this test suite does it here because it is also
// standing up the entire database from scratch.
func provisionAppRole(ctx context.Context, migratorDSN string) error {
	conn, err := pgxpool.New(ctx, migratorDSN)
	if err != nil {
		return err
	}
	defer conn.Close()

	statements := []string{
		`CREATE ROLE billing_app WITH LOGIN PASSWORD 'billing_app_pw'`,
		`GRANT USAGE ON SCHEMA public TO billing_app`,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO billing_app`,
		`GRANT EXECUTE ON FUNCTION auth_lookup_user_by_email(text) TO billing_app`,
	}
	for _, stmt := range statements {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}
