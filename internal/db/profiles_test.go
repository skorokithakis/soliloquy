package db

import (
	"context"
	"testing"
	"time"
)

func TestListThreadsByAuthor(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()

	alice, err := database.CreateUser(ctx, "alice", "hash", false, nil, base)
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, err := database.CreateUser(ctx, "bob", "hash", false, nil, base)
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}

	first, err := database.CreateThread(ctx, alice.ID, "First", "", "body", "", base)
	if err != nil {
		t.Fatalf("CreateThread first: %v", err)
	}
	second, err := database.CreateThread(ctx, alice.ID, "Second", "", "body", "", base.Add(time.Minute))
	if err != nil {
		t.Fatalf("CreateThread second: %v", err)
	}
	third, err := database.CreateThread(ctx, alice.ID, "Third", "", "body", "", base.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("CreateThread third: %v", err)
	}
	if _, err := database.CreateThread(ctx, bob.ID, "Bob's thread", "", "body", "", base.Add(3*time.Minute)); err != nil {
		t.Fatalf("CreateThread bob: %v", err)
	}

	// A deleted thread must not appear on the profile.
	if _, err := database.DeleteThread(ctx, first.ID, base.Add(4*time.Minute)); err != nil {
		t.Fatalf("DeleteThread: %v", err)
	}

	threads, err := database.ListThreadsByAuthor(ctx, alice.ID, 10)
	if err != nil {
		t.Fatalf("ListThreadsByAuthor: %v", err)
	}
	assertThreadIDs(t, threads, third.ID, second.ID)

	limited, err := database.ListThreadsByAuthor(ctx, alice.ID, 1)
	if err != nil {
		t.Fatalf("ListThreadsByAuthor limit: %v", err)
	}
	assertThreadIDs(t, limited, third.ID)
}

func TestListCommentsByAuthor(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()

	alice, err := database.CreateUser(ctx, "alice", "hash", false, nil, base)
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, err := database.CreateUser(ctx, "bob", "hash", false, nil, base)
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}

	live, err := database.CreateThread(ctx, alice.ID, "Live thread", "", "body", "", base)
	if err != nil {
		t.Fatalf("CreateThread live: %v", err)
	}
	deletedThread, err := database.CreateThread(ctx, alice.ID, "Deleted thread", "", "body", "", base)
	if err != nil {
		t.Fatalf("CreateThread deleted: %v", err)
	}

	first, err := database.CreateComment(ctx, live.ID, nil, alice.ID, "first comment", "", base.Add(time.Minute))
	if err != nil {
		t.Fatalf("CreateComment first: %v", err)
	}
	second, err := database.CreateComment(ctx, live.ID, nil, alice.ID, "second comment", "", base.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("CreateComment second: %v", err)
	}
	deleted, err := database.CreateComment(ctx, live.ID, nil, alice.ID, "deleted comment", "", base.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("CreateComment deleted: %v", err)
	}
	if _, err := database.DeleteComment(ctx, deleted.ID, base.Add(4*time.Minute)); err != nil {
		t.Fatalf("DeleteComment: %v", err)
	}
	if _, err := database.CreateComment(ctx, live.ID, nil, bob.ID, "bob comment", "", base.Add(5*time.Minute)); err != nil {
		t.Fatalf("CreateComment bob: %v", err)
	}
	if _, err := database.CreateComment(ctx, deletedThread.ID, nil, alice.ID, "comment on a deleted thread", "", base.Add(6*time.Minute)); err != nil {
		t.Fatalf("CreateComment deleted thread: %v", err)
	}
	if _, err := database.DeleteThread(ctx, deletedThread.ID, base.Add(7*time.Minute)); err != nil {
		t.Fatalf("DeleteThread: %v", err)
	}

	comments, err := database.ListCommentsByAuthor(ctx, alice.ID, 10)
	if err != nil {
		t.Fatalf("ListCommentsByAuthor: %v", err)
	}
	if len(comments) != 2 {
		t.Fatalf("comments = %d, want 2", len(comments))
	}
	if comments[0].PublicID != second.PublicID || comments[1].PublicID != first.PublicID {
		t.Errorf("comment order = %q, %q; want newest first", comments[0].PublicID, comments[1].PublicID)
	}
	if comments[0].ThreadPublicID != live.PublicID || comments[0].ThreadTitle != "Live thread" {
		t.Errorf("comment thread = %q, %q; want %q, %q",
			comments[0].ThreadPublicID, comments[0].ThreadTitle, live.PublicID, "Live thread")
	}
	if comments[0].Body != "second comment" {
		t.Errorf("comment body = %q, want %q", comments[0].Body, "second comment")
	}

	limited, err := database.ListCommentsByAuthor(ctx, alice.ID, 1)
	if err != nil {
		t.Fatalf("ListCommentsByAuthor limit: %v", err)
	}
	if len(limited) != 1 || limited[0].PublicID != second.PublicID {
		t.Errorf("limited comments = %+v, want only %q", limited, second.PublicID)
	}
}
