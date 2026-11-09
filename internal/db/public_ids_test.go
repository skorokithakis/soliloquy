package db

import (
	"context"
	"strconv"
	"testing"
	"time"
)

func TestThreadPublicIDLookup(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	alice, err := database.CreateUser(ctx, "alice", "hash", false, nil, now)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	thread, err := database.CreateThread(ctx, alice.ID, "Thread", "", "body", "<p>body</p>", now)
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	if len(thread.PublicID) != 8 {
		t.Fatalf("thread public ID = %q, want 8 characters", thread.PublicID)
	}

	got, err := database.ThreadByPublicID(ctx, thread.PublicID)
	if err != nil || got == nil {
		t.Fatalf("ThreadByPublicID: thread = %v, err = %v", got, err)
	}
	if got.ID != thread.ID || got.PublicID != thread.PublicID {
		t.Errorf("ThreadByPublicID returned thread %d/%q, want %d/%q", got.ID, got.PublicID, thread.ID, thread.PublicID)
	}

	// The old sequential URL must not resolve through the public lookup.
	if got, err := database.ThreadByPublicID(ctx, strconv.FormatInt(thread.ID, 10)); err != nil || got != nil {
		t.Errorf("ThreadByPublicID(numeric ID) = %v, %v; want nil, nil", got, err)
	}
}

func TestCommentPublicIDs(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	alice, err := database.CreateUser(ctx, "alice", "hash", false, nil, now)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	thread, err := database.CreateThread(ctx, alice.ID, "Thread", "", "body", "<p>body</p>", now)
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	root, err := database.CreateComment(ctx, thread.ID, nil, alice.ID, "root", "<p>root</p>", now)
	if err != nil {
		t.Fatalf("CreateComment root: %v", err)
	}
	reply, err := database.CreateComment(ctx, thread.ID, &root.ID, alice.ID, "reply", "<p>reply</p>", now)
	if err != nil {
		t.Fatalf("CreateComment reply: %v", err)
	}

	if len(root.PublicID) != 8 || root.PublicID == reply.PublicID {
		t.Errorf("comment public IDs = %q and %q, want distinct 8-character IDs", root.PublicID, reply.PublicID)
	}
	if root.ThreadPublicID != thread.PublicID {
		t.Errorf("root thread public ID = %q, want %q", root.ThreadPublicID, thread.PublicID)
	}
	if root.ParentPublicID != nil {
		t.Errorf("top-level comment parent public ID = %q, want nil", *root.ParentPublicID)
	}
	if reply.ParentPublicID == nil || *reply.ParentPublicID != root.PublicID {
		t.Errorf("reply parent public ID = %v, want %q", reply.ParentPublicID, root.PublicID)
	}

	got, err := database.CommentByPublicID(ctx, reply.PublicID, 0)
	if err != nil || got == nil {
		t.Fatalf("CommentByPublicID: comment = %v, err = %v", got, err)
	}
	if got.ID != reply.ID || got.PublicID != reply.PublicID {
		t.Errorf("CommentByPublicID returned comment %d/%q, want %d/%q", got.ID, got.PublicID, reply.ID, reply.PublicID)
	}
}

func TestInsertsRequirePublicID(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	alice, err := database.CreateUser(ctx, "alice", "hash", false, nil, now)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	thread, err := database.CreateThread(ctx, alice.ID, "Thread", "", "body", "<p>body</p>", now)
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}

	if _, err := database.ExecContext(ctx, `
		INSERT INTO threads (author_id, title, created_at, last_activity_at)
		VALUES (?, 'No public ID', ?, ?)`, alice.ID, now.Unix(), now.Unix()); err == nil {
		t.Error("inserting a thread without a public ID succeeded")
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO comments (thread_id, author_id, body, body_html, created_at)
		VALUES (?, ?, 'No public ID', '', ?)`, thread.ID, alice.ID, now.Unix()); err == nil {
		t.Error("inserting a comment without a public ID succeeded")
	}
}
