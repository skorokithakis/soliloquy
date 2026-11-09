package db

import (
	"context"
	"testing"
	"time"
)

func TestThreadLifecycle(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()

	alice, err := database.CreateUser(ctx, "alice", "hash", false, nil, base)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	first, err := database.CreateThread(ctx, alice.ID, "First", "", "first body", "<p>first body</p>", base)
	if err != nil {
		t.Fatalf("CreateThread first: %v", err)
	}
	second, err := database.CreateThread(ctx, alice.ID, "Second", "", "", "", base.Add(time.Minute))
	if err != nil {
		t.Fatalf("CreateThread second: %v", err)
	}
	third, err := database.CreateThread(ctx, alice.ID, "Third", "https://example.com/", "", "", base.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("CreateThread third: %v", err)
	}

	if first.Score != 1 || first.CommentCount != 0 {
		t.Errorf("new thread score/count = %d/%d, want 1/0", first.Score, first.CommentCount)
	}
	if !first.CreatedAt.Equal(base) || !first.LastActivityAt.Equal(base) {
		t.Errorf("new thread times = %v/%v, want %v", first.CreatedAt, first.LastActivityAt, base)
	}

	threads, err := database.ListThreads(ctx, 10, 0)
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	assertThreadIDs(t, threads, third.ID, second.ID, first.ID)

	// Comment code moves a thread's last_activity_at; the list follows it.
	if _, err := database.ExecContext(ctx, `UPDATE threads SET last_activity_at = ? WHERE id = ?`, base.Add(3*time.Minute).Unix(), first.ID); err != nil {
		t.Fatalf("move activity: %v", err)
	}
	threads, err = database.ListThreads(ctx, 10, 0)
	if err != nil {
		t.Fatalf("ListThreads after activity: %v", err)
	}
	assertThreadIDs(t, threads, first.ID, third.ID, second.ID)

	deleted, err := database.DeleteThread(ctx, second.ID, base.Add(4*time.Minute))
	if err != nil {
		t.Fatalf("DeleteThread: %v", err)
	}
	if !deleted {
		t.Fatal("DeleteThread did not delete a live thread")
	}
	if deleted, err := database.DeleteThread(ctx, second.ID, base.Add(5*time.Minute)); err != nil || deleted {
		t.Errorf("second DeleteThread = %t, %v; want false, nil", deleted, err)
	}

	threads, err = database.ListThreads(ctx, 10, 0)
	if err != nil {
		t.Fatalf("ListThreads after delete: %v", err)
	}
	assertThreadIDs(t, threads, first.ID, third.ID)

	got, err := database.ThreadByID(ctx, second.ID)
	if err != nil || got == nil {
		t.Fatalf("ThreadByID deleted: thread = %v, err = %v", got, err)
	}
	if got.DeletedAt == nil {
		t.Error("soft-deleted thread has no deleted_at")
	}

	page, err := database.ListThreads(ctx, 1, 0)
	if err != nil {
		t.Fatalf("ListThreads page: %v", err)
	}
	assertThreadIDs(t, page, first.ID)
}

func TestUpdateThreadRequiresAuthor(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	alice, err := database.CreateUser(ctx, "alice", "hash", false, nil, now)
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, err := database.CreateUser(ctx, "bob", "hash", false, nil, now)
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}

	thread, err := database.CreateThread(ctx, alice.ID, "Original", "", "old", "<p>old</p>", now)
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}

	editedAt := now.Add(time.Hour)
	updated, err := database.UpdateThread(ctx, thread.ID, bob.ID, "Hijacked", "", "new", "<p>new</p>", editedAt)
	if err != nil {
		t.Fatalf("UpdateThread as bob: %v", err)
	}
	if updated {
		t.Fatal("a non-author updated the thread")
	}

	updated, err = database.UpdateThread(ctx, thread.ID, alice.ID, "Edited", "https://example.com/", "new body", "<p>new body</p>", editedAt)
	if err != nil {
		t.Fatalf("UpdateThread as alice: %v", err)
	}
	if !updated {
		t.Fatal("the author could not update the thread")
	}

	got, err := database.ThreadByID(ctx, thread.ID)
	if err != nil || got == nil {
		t.Fatalf("ThreadByID: thread = %v, err = %v", got, err)
	}
	if got.Title != "Edited" || got.URL != "https://example.com/" || got.Body != "new body" || got.BodyHTML != "<p>new body</p>" {
		t.Errorf("updated thread = %+v", got)
	}
	if got.EditedAt == nil || !got.EditedAt.Equal(editedAt) {
		t.Errorf("EditedAt = %v, want %v", got.EditedAt, editedAt)
	}
	if !got.LastActivityAt.Equal(now) {
		t.Errorf("LastActivityAt = %v, want editing not to change it (%v)", got.LastActivityAt, now)
	}
}

func TestDeletedUserAuthorName(t *testing.T) {
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

	if _, err := database.ExecContext(ctx, `UPDATE users SET deleted_at = ? WHERE id = ?`, now.Unix(), alice.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	got, err := database.ThreadByID(ctx, thread.ID)
	if err != nil || got == nil {
		t.Fatalf("ThreadByID: thread = %v, err = %v", got, err)
	}
	if got.AuthorName() != "[deleted user]" {
		t.Errorf("AuthorName = %q, want [deleted user]", got.AuthorName())
	}
}

func assertThreadIDs(t *testing.T, threads []Thread, want ...int64) {
	t.Helper()

	if len(threads) != len(want) {
		t.Fatalf("thread count = %d, want %d", len(threads), len(want))
	}
	for i, id := range want {
		if threads[i].ID != id {
			t.Errorf("thread %d ID = %d, want %d", i, threads[i].ID, id)
		}
	}
}
