//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"billing-platform/internal/platform/database"
	"billing-platform/migrations"
)

// TestMigrate_UpAndDownRoundTrip verifies every migration's down.sql
// actually reverses its up.sql cleanly (brief §72), using its own
// throwaway container/schema so this destructive up/down cycling never
// touches the shared, already-seeded schema the other integration tests
// depend on.
func TestMigrate_UpAndDownRoundTrip(t *testing.T) {
	ctx := context.Background()
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
		t.Fatalf("starting postgres container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("getting connection string: %v", err)
	}

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
