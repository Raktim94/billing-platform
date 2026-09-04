//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"rechvix/internal/platform/database"
	"rechvix/migrations"
)

// TestMigrate_UpAndDownRoundTrip verifies every migration's down.sql
// actually reverses its up.sql cleanly (brief §72), using its own
// throwaway container/schema so this destructive up/down cycling never
// touches the shared, already-seeded schema the other integration tests
// depend on. Same TEST_POSTGRES_ADMIN_DSN escape hatch as setup_test.go's
// acquirePostgres, for the same reason (no Docker in this environment) —
// duplicated rather than shared because this test intentionally uses its
// own separate throwaway database/role, not the suite-wide one.
func TestMigrate_UpAndDownRoundTrip(t *testing.T) {
	ctx := context.Background()
	dsn, cleanup, err := acquireMigrateTestPostgres(ctx)
	if err != nil {
		t.Fatalf("acquiring postgres: %v", err)
	}
	t.Cleanup(cleanup)

	if err := database.Migrate(dsn, migrations.FS); err != nil {
		t.Fatalf("first Migrate (up) failed: %v", err)
	}
	// Roll every migration back...
	if err := database.MigrateDown(dsn, migrations.FS, 8); err != nil {
		t.Fatalf("MigrateDown failed: %v", err)
	}
	// ...and confirm re-applying from scratch still works cleanly, which
	// is only true if every down.sql actually removed everything its
	// up.sql created (no leftover objects causing a duplicate-object
	// error on the second Migrate).
	if err := database.Migrate(dsn, migrations.FS); err != nil {
		t.Fatalf("second Migrate (up after full down) failed: %v", err)
	}
}

func acquireMigrateTestPostgres(ctx context.Context) (dsn string, cleanup func(), err error) {
	adminDSN := os.Getenv("TEST_POSTGRES_ADMIN_DSN")
	if adminDSN == "" {
		container, err := tcpostgres.Run(ctx, "postgres:18",
			tcpostgres.WithDatabase("billing_migrate_test"),
			tcpostgres.WithUsername("billing_migrate_test"),
			tcpostgres.WithPassword("billing_migrate_test"),
			testcontainers.WithWaitStrategy(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(60*time.Second)),
		)
		if err != nil {
			return "", nil, fmt.Errorf("starting postgres container: %w", err)
		}
		dsn, err := container.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			return "", nil, fmt.Errorf("getting connection string: %w", err)
		}
		return dsn, func() { _ = container.Terminate(ctx) }, nil
	}

	admin, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		return "", nil, fmt.Errorf("connecting to TEST_POSTGRES_ADMIN_DSN: %w", err)
	}
	defer admin.Close()
	for _, stmt := range []string{
		`DROP DATABASE IF EXISTS billing_migrate_test`,
		`DROP ROLE IF EXISTS billing_migrate_test`,
		`CREATE ROLE billing_migrate_test WITH LOGIN PASSWORD 'billing_migrate_test' CREATEDB`,
		`CREATE DATABASE billing_migrate_test OWNER billing_migrate_test`,
	} {
		if _, err := admin.Exec(ctx, stmt); err != nil {
			return "", nil, fmt.Errorf("resetting billing_migrate_test (%s): %w", stmt, err)
		}
	}
	base := strings.SplitN(adminDSN, "@", 2)
	if len(base) != 2 {
		return "", nil, fmt.Errorf("TEST_POSTGRES_ADMIN_DSN is not in postgres://user:pass@host:port/db form")
	}
	hostPart := strings.SplitN(base[1], "/", 2)[0]
	return fmt.Sprintf("postgres://billing_migrate_test:billing_migrate_test@%s/billing_migrate_test?sslmode=disable", hostPart), func() {}, nil
}
