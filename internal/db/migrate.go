package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

const migrationsDir = "migrations"

type migration struct {
	version int
	name    string
	sql     string
}

// Migrate applies all embedded migrations that have not run yet. Progress is
// tracked with PRAGMA user_version, so no bookkeeping table is needed.
func (db *DB) Migrate(ctx context.Context) error {
	return migrate(ctx, db.DB, migrationsFS)
}

func migrate(ctx context.Context, sqlDB *sql.DB, fsys fs.FS) error {
	migrations, err := loadMigrations(fsys)
	if err != nil {
		return err
	}

	current, err := userVersion(ctx, sqlDB)
	if err != nil {
		return err
	}

	if len(migrations) == 0 {
		if current != 0 {
			return fmt.Errorf("database schema version is %d but no migrations are embedded", current)
		}
		return nil
	}

	latest := migrations[len(migrations)-1].version
	if current > latest {
		return fmt.Errorf("database schema version %d is newer than the latest embedded migration %d", current, latest)
	}

	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		if err := applyMigration(ctx, sqlDB, m); err != nil {
			return fmt.Errorf("apply migration %d (%s): %w", m.version, m.name, err)
		}
	}

	return nil
}

func loadMigrations(fsys fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("read migrations directory: %w", err)
	}

	seen := make(map[int]string)
	var migrations []migration

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		base := strings.TrimSuffix(entry.Name(), ".sql")
		prefix, _, found := strings.Cut(base, "_")
		if !found {
			return nil, fmt.Errorf("migration %q must be named NNNN_description.sql", entry.Name())
		}

		version, err := strconv.Atoi(prefix)
		if err != nil || version <= 0 {
			return nil, fmt.Errorf("migration %q has an invalid version prefix", entry.Name())
		}
		if other, exists := seen[version]; exists {
			return nil, fmt.Errorf("migrations %q and %q share version %d", other, entry.Name(), version)
		}
		seen[version] = entry.Name()

		body, err := fs.ReadFile(fsys, path.Join(migrationsDir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}

		migrations = append(migrations, migration{
			version: version,
			name:    entry.Name(),
			sql:     string(body),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	return migrations, nil
}

func userVersion(ctx context.Context, sqlDB *sql.DB) (int, error) {
	var version int
	if err := sqlDB.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return version, nil
}

func applyMigration(ctx context.Context, sqlDB *sql.DB, m migration) error {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		return err
	}

	// The version is an integer parsed from the filename, so interpolating it
	// is safe. PRAGMA statements cannot take bound parameters.
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", m.version)); err != nil {
		return err
	}

	return tx.Commit()
}
