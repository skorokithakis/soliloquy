package db

import (
	"context"
	"io/fs"
	"path"
	"testing"
	"testing/fstest"
	"time"
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
	if version := schemaVersion(t, database); version != 14 {
		t.Fatalf("user_version = %d, want 14", version)
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

// TestThreadReadsMigrationBackfillsUnreadSince applies the schema as it was
// before thread read state existed, inserts content, then runs the current
// migrations. Threads that existed before the migration must count as read for
// existing members.
func TestThreadReadsMigrationBackfillsUnreadSince(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	entries, err := fs.Glob(migrationsFS, migrationsDir+"/*.sql")
	if err != nil {
		t.Fatalf("list migrations: %v", err)
	}
	before := fstest.MapFS{}
	for _, entry := range entries {
		if path.Base(entry) >= "0013_thread_reads.sql" {
			continue
		}
		data, err := fs.ReadFile(migrationsFS, entry)
		if err != nil {
			t.Fatalf("read migration %q: %v", entry, err)
		}
		before[entry] = &fstest.MapFile{Data: data}
	}
	if err := migrate(ctx, database.DB, before); err != nil {
		t.Fatalf("migrate before thread reads: %v", err)
	}

	if _, err := database.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, created_at)
		VALUES ('alice', 'hash', 1)`); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO threads (public_id, author_id, title, created_at, last_activity_at)
		VALUES ('AAAAAAAA', 1, 'Old thread', 1, 1)`); err != nil {
		t.Fatalf("insert thread: %v", err)
	}

	startedAt := time.Now().Add(-time.Minute).Unix()
	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("migrate thread reads: %v", err)
	}

	var unreadSince int64
	if err := database.QueryRowContext(ctx, `SELECT unread_since FROM users WHERE id = 1`).Scan(&unreadSince); err != nil {
		t.Fatalf("read unread_since: %v", err)
	}
	if unreadSince < startedAt {
		t.Errorf("unread_since = %d, want the migration time (at least %d)", unreadSince, startedAt)
	}

	threads, err := database.ListThreads(ctx, 1, 10, 0)
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	if len(threads) != 1 {
		t.Fatalf("thread count = %d, want 1", len(threads))
	}
	if threads[0].Unopened || threads[0].NewComments != 0 {
		t.Errorf("existing thread after migration = unopened %t, %d new; want false, 0",
			threads[0].Unopened, threads[0].NewComments)
	}
}

// TestFollowedNotificationMigrationKeepsRowsAndSequence applies the schema as
// it was before the 'followed' kind existed, inserts notifications, deletes the
// newest one, then runs the current migrations. The table is rebuilt, so the
// rows and indexes must survive and the deleted notification's ID must not be
// reused.
func TestFollowedNotificationMigrationKeepsRowsAndSequence(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	entries, err := fs.Glob(migrationsFS, migrationsDir+"/*.sql")
	if err != nil {
		t.Fatalf("list migrations: %v", err)
	}
	before := fstest.MapFS{}
	for _, entry := range entries {
		if path.Base(entry) >= "0014_thread_follows.sql" {
			continue
		}
		data, err := fs.ReadFile(migrationsFS, entry)
		if err != nil {
			t.Fatalf("read migration %q: %v", entry, err)
		}
		before[entry] = &fstest.MapFile{Data: data}
	}
	if err := migrate(ctx, database.DB, before); err != nil {
		t.Fatalf("migrate before followed notifications: %v", err)
	}

	if _, err := database.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, created_at)
		VALUES ('alice', 'hash', 1), ('bob', 'hash', 1)`); err != nil {
		t.Fatalf("insert users: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO threads (public_id, author_id, title, created_at, last_activity_at)
		VALUES ('AAAAAAAA', 1, 'Thread', 1, 1)`); err != nil {
		t.Fatalf("insert thread: %v", err)
	}
	for i, kind := range []string{"reply", "thread_comment", "mention"} {
		if _, err := database.ExecContext(ctx, `
			INSERT INTO notifications (recipient_id, kind, actor_id, thread_id, created_at)
			VALUES (2, ?, 1, 1, ?)`, kind, 10+i); err != nil {
			t.Fatalf("insert %s notification: %v", kind, err)
		}
	}
	// Deleting the newest row leaves the AUTOINCREMENT sequence above MAX(id).
	if _, err := database.ExecContext(ctx, `DELETE FROM notifications WHERE id = 3`); err != nil {
		t.Fatalf("delete newest notification: %v", err)
	}

	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("migrate followed notifications: %v", err)
	}

	rows, err := database.QueryContext(ctx, `
		SELECT id, recipient_id, kind, actor_id, thread_id, created_at
		FROM notifications ORDER BY id`)
	if err != nil {
		t.Fatalf("read notifications: %v", err)
	}
	defer rows.Close()

	type notificationRow struct {
		id, recipientID, actorID, threadID, createdAt int64
		kind                                          string
	}
	var got []notificationRow
	for rows.Next() {
		var row notificationRow
		if err := rows.Scan(&row.id, &row.recipientID, &row.kind, &row.actorID, &row.threadID, &row.createdAt); err != nil {
			t.Fatalf("scan notification: %v", err)
		}
		got = append(got, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read notifications: %v", err)
	}
	want := []notificationRow{
		{id: 1, recipientID: 2, kind: "reply", actorID: 1, threadID: 1, createdAt: 10},
		{id: 2, recipientID: 2, kind: "thread_comment", actorID: 1, threadID: 1, createdAt: 11},
	}
	if len(got) != len(want) {
		t.Fatalf("notification rows after migration = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("notification row %d = %+v, want %+v", i, got[i], want[i])
		}
	}

	for _, name := range []string{
		"notifications_recipient_created_idx",
		"notifications_recipient_unread_idx",
	} {
		var count int
		if err := database.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, name).Scan(&count); err != nil {
			t.Fatalf("read index %q: %v", name, err)
		}
		if count != 1 {
			t.Errorf("index %q after migration = %d, want 1", name, count)
		}
	}

	// The new kind is allowed, and IDs continue after the deleted row rather
	// than reusing 3.
	created, err := database.CreateNotification(ctx, 2, 1, NotificationFollowed, 1, nil, time.Unix(20, 0))
	if err != nil {
		t.Fatalf("create followed notification: %v", err)
	}
	if created.ID != 4 {
		t.Errorf("new notification ID = %d, want 4", created.ID)
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
