package db

import (
	"context"
	"io/fs"
	"path"
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
	if version := schemaVersion(t, database); version != 11 {
		t.Fatalf("user_version = %d, want 11", version)
	}
}

// TestPublicIDMigrationBackfillsExistingRows applies the schema as it was
// before public IDs existed, inserts content, then runs the current migrations.
// The old rows must get usable public IDs.
func TestPublicIDMigrationBackfillsExistingRows(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	entries, err := fs.Glob(migrationsFS, migrationsDir+"/*.sql")
	if err != nil {
		t.Fatalf("list migrations: %v", err)
	}
	before := fstest.MapFS{}
	for _, entry := range entries {
		if path.Base(entry) >= "0010_public_ids.sql" {
			continue
		}
		data, err := fs.ReadFile(migrationsFS, entry)
		if err != nil {
			t.Fatalf("read migration %q: %v", entry, err)
		}
		before[entry] = &fstest.MapFile{Data: data}
	}
	if err := migrate(ctx, database.DB, before); err != nil {
		t.Fatalf("migrate before public IDs: %v", err)
	}

	if _, err := database.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, created_at)
		VALUES ('alice', 'hash', 1)`); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	result, err := database.ExecContext(ctx, `
		INSERT INTO threads (author_id, title, created_at, last_activity_at)
		VALUES (1, 'Thread', 1, 1)`)
	if err != nil {
		t.Fatalf("insert thread: %v", err)
	}
	threadID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("thread ID: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO comments (thread_id, author_id, body, body_html, created_at)
		VALUES (?, 1, 'body', '<p>body</p>', 2)`, threadID); err != nil {
		t.Fatalf("insert comment: %v", err)
	}

	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("migrate public IDs: %v", err)
	}

	var threadPublicID, commentPublicID string
	if err := database.QueryRowContext(ctx, `SELECT public_id FROM threads WHERE id = ?`, threadID).Scan(&threadPublicID); err != nil {
		t.Fatalf("read thread public ID: %v", err)
	}
	if err := database.QueryRowContext(ctx, `SELECT public_id FROM comments WHERE thread_id = ?`, threadID).Scan(&commentPublicID); err != nil {
		t.Fatalf("read comment public ID: %v", err)
	}
	if len(threadPublicID) != 8 || len(commentPublicID) != 8 {
		t.Errorf("backfilled public IDs = %q and %q, want 8 characters each", threadPublicID, commentPublicID)
	}

	thread, err := database.ThreadByPublicID(ctx, threadPublicID)
	if err != nil || thread == nil || thread.ID != threadID {
		t.Errorf("ThreadByPublicID after backfill = %v, %v; want thread %d", thread, err, threadID)
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
