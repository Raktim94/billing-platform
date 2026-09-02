package database

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Migrate applies every pending up migration from migrationsFS (pass
// migrations.FS from the embedded migrations package) against dsn. It is
// idempotent — running it with nothing pending is a no-op, not an error.
func Migrate(dsn string, migrationsFS fs.FS) error {
	sourceDriver, err := iofs.New(migrationsFS, ".")
	if err != nil {
		return fmt.Errorf("database: opening migrations source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", sourceDriver, dsn)
	if err != nil {
		return fmt.Errorf("database: initializing migrator: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("database: applying migrations: %w", err)
	}
	return nil
}

// MigrateDown rolls back exactly one migration. Intended for local
// development and the integration-test suite (which applies Up then Down
// to verify both directions are consistent) — not for routine production
// use, where forward-only migrations plus a new corrective migration are
// the safer default (brief §72: migrations must be "backward-aware where
// possible", not "routinely reversed").
func MigrateDown(dsn string, migrationsFS fs.FS, steps int) error {
	sourceDriver, err := iofs.New(migrationsFS, ".")
	if err != nil {
		return fmt.Errorf("database: opening migrations source: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", sourceDriver, dsn)
	if err != nil {
		return fmt.Errorf("database: initializing migrator: %w", err)
	}
	defer m.Close()

	if err := m.Steps(-steps); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("database: rolling back migrations: %w", err)
	}
	return nil
}

// unused import guard: postgres driver package is imported for its
// side-effecting init() (registers the "postgres" scheme with the
// database/sql-based migrate driver), referenced explicitly here so `go
// mod tidy` does not drop it as unused.
var _ = postgres.Config{}
