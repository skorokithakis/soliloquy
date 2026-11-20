package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/skorokithakis/soliloquy/internal/auth"
	"github.com/skorokithakis/soliloquy/internal/db"
)

func TestSeedIsIdempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "qa.db")

	database, err := db.Open(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	now := time.Date(2026, time.November, 20, 12, 0, 0, 0, time.UTC)
	seeded, err := seed(ctx, database, now)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if !seeded {
		t.Fatal("seed did not write anything to an empty database")
	}

	users, err := database.ListUsers(ctx)
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(users) != len(qaUsers) {
		t.Errorf("seeded %d users, want %d", len(users), len(qaUsers))
	}
	admin, err := database.UserByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("look up admin: %v", err)
	}
	if admin == nil || !admin.IsAdmin {
		t.Fatal("admin account was not seeded as an admin")
	}
	if !auth.CheckPassword(admin.PasswordHash, "admin") {
		t.Error("admin password does not match")
	}

	before := snapshot(t, database)
	if before.threads != len(qaThreads()) {
		t.Errorf("seeded %d threads, want %d", before.threads, len(qaThreads()))
	}

	seeded, err = seed(ctx, database, now.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if seeded {
		t.Error("second seed wrote data into a database that already had threads")
	}

	after := snapshot(t, database)
	if after != before {
		t.Errorf("second seed changed the database: before %+v, after %+v", before, after)
	}
}

func TestSeedSkipsDatabaseWithOnlyDeletedThread(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "qa.db")

	database, err := db.Open(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	now := time.Date(2026, time.November, 20, 12, 0, 0, 0, time.UTC)
	hash, err := auth.HashPassword("password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user, err := database.CreateUser(ctx, "existing", hash, false, nil, now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	thread, err := database.CreateThread(ctx, user.ID, "Deleted thread", "", "body", "", now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	deleted, err := database.DeleteThread(ctx, thread.ID, now)
	if err != nil {
		t.Fatalf("delete thread: %v", err)
	}
	if !deleted {
		t.Fatal("thread was not deleted")
	}

	before := snapshot(t, database)
	seeded, err := seed(ctx, database, now.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if seeded {
		t.Error("seed wrote data into a database whose only thread was deleted")
	}

	after := snapshot(t, database)
	if after != before {
		t.Errorf("seed changed a database with a deleted thread: before %+v, after %+v", before, after)
	}
}

type counts struct {
	users    int
	threads  int
	comments int
	votes    int
}

func snapshot(t *testing.T, database *db.DB) counts {
	t.Helper()

	var c counts
	for _, query := range []struct {
		name string
		sql  string
		dest *int
	}{
		{"users", `SELECT COUNT(*) FROM users`, &c.users},
		{"threads", `SELECT COUNT(*) FROM threads`, &c.threads},
		{"comments", `SELECT COUNT(*) FROM comments`, &c.comments},
		{"votes", `SELECT COUNT(*) FROM votes`, &c.votes},
	} {
		if err := database.QueryRow(query.sql).Scan(query.dest); err != nil {
			t.Fatalf("count %s: %v", query.name, err)
		}
	}
	return c
}
