package migrations

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// FS contains the schema used by both the migration command and tests.
// The API never executes these migrations.
//
//go:embed *.sql
var FS embed.FS

func Apply(path string, down bool) error {
	if path == "" {
		return fmt.Errorf("database path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}

	source, err := iofs.New(FS, ".")
	if err != nil {
		return fmt.Errorf("create migration source: %w", err)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve database path: %w", err)
	}
	dbURL := "sqlite://" + absPath
	migrator, err := migrate.NewWithSourceInstance("iofs", source, dbURL)
	if err != nil {
		return fmt.Errorf("open migration database: %w", err)
	}
	defer func() { _, _ = migrator.Close() }()

	if down {
		err = migrator.Down()
	} else {
		err = migrator.Up()
	}
	if err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
