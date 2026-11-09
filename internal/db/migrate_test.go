package db

import (
	"context"
	"testing"
	"testing/fstest"
)

func TestMigrateAppliesMigrationsInOrder(t *testing.T) {
	database := openTestDB(t)

	migrations := fstest.MapFS{
		"migrations/0001_create_widgets.sql": &fstest.MapFile{
			Data: []byte("CREATE TABLE widgets (id INTEGER PRIMARY KEY);"),
		},
		"migrations/0002_add_widget_name.sql": &fstest.MapFile{
			Data: []byte("ALTER TABLE widgets ADD COLUMN name TEXT NOT NULL DEFAULT '';"),
		},
	}

	if err := migrate(context.Background(), database.DB, migrations); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if version := schemaVersion(t, database); version != 2 {
		t.Fatalf("user_version = %d, want 2", version)
	}

	if _, err := database.Exec("INSERT INTO widgets (name) VALUES ('test')"); err != nil {
		t.Fatalf("use migrated schema: %v", err)
	}

	// Running again must be a no-op rather than failing on existing objects.
	if err := migrate(context.Background(), database.DB, migrations); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if version := schemaVersion(t, database); version != 2 {
		t.Fatalf("user_version after second migrate = %d, want 2", version)
	}
}

func TestMigrateRejectsNewerSchema(t *testing.T) {
	database := openTestDB(t)
	if _, err := database.Exec("PRAGMA user_version = 99"); err != nil {
		t.Fatalf("set user_version: %v", err)
	}

	migrations := fstest.MapFS{
		"migrations/0001_init.sql": &fstest.MapFile{
			Data: []byte("CREATE TABLE widgets (id INTEGER PRIMARY KEY);"),
		},
	}

	if err := migrate(context.Background(), database.DB, migrations); err == nil {
		t.Fatal("migrate accepted a database with a newer schema version")
	}
}

func TestLoadMigrationsValidatesNames(t *testing.T) {
	tests := map[string]fstest.MapFS{
		"missing underscore": {
			"migrations/0001init.sql": &fstest.MapFile{Data: []byte("SELECT 1;")},
		},
		"zero version": {
			"migrations/0000_init.sql": &fstest.MapFile{Data: []byte("SELECT 1;")},
		},
		"duplicate version": {
			"migrations/0001_first.sql":  &fstest.MapFile{Data: []byte("SELECT 1;")},
			"migrations/0001_second.sql": &fstest.MapFile{Data: []byte("SELECT 2;")},
		},
	}

	for name, migrations := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := loadMigrations(migrations); err == nil {
				t.Fatal("loadMigrations accepted invalid migration names")
			}
		})
	}
}

func TestEmbeddedMigrationsRun(t *testing.T) {
	database := openTestDB(t)

	if err := database.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if version := schemaVersion(t, database); version != 7 {
		t.Fatalf("user_version = %d, want 7", version)
	}
}

func schemaVersion(t *testing.T, database *DB) int {
	t.Helper()

	var version int
	if err := database.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	return version
}
