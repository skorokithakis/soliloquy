package db

import (
	"context"
	"testing"
	"time"
)

func TestMarkThreadReadReturnsPreviousCutoff(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()

	alice, err := database.CreateUser(ctx, "alice", "hash", false, nil, base)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	thread, err := database.CreateThread(ctx, alice.ID, "Thread", "", "body", "<p>body</p>", base.Add(time.Minute))
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}

	// The first visit has no earlier read to compare with, so its cutoff is
	// the visit time itself and nothing that already existed counts as new.
	firstVisit := base.Add(2 * time.Minute)
	cutoff, err := database.MarkThreadRead(ctx, alice.ID, thread.ID, firstVisit)
	if err != nil {
		t.Fatalf("MarkThreadRead first: %v", err)
	}
	if !cutoff.Equal(firstVisit) {
		t.Errorf("first cutoff = %v, want the visit time %v", cutoff, firstVisit)
	}

	var lastReadAt int64
	if err := database.QueryRowContext(ctx, `
		SELECT last_read_at FROM thread_reads WHERE user_id = ? AND thread_id = ?`,
		alice.ID, thread.ID).Scan(&lastReadAt); err != nil {
		t.Fatalf("read thread_reads row: %v", err)
	}
	if lastReadAt != firstVisit.Unix() {
		t.Errorf("last_read_at = %d, want %d", lastReadAt, firstVisit.Unix())
	}

	// Later reads start from the previous visit.
	cutoff, err = database.MarkThreadRead(ctx, alice.ID, thread.ID, base.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("MarkThreadRead second: %v", err)
	}
	if !cutoff.Equal(firstVisit) {
		t.Errorf("second cutoff = %v, want the previous read %v", cutoff, firstVisit)
	}

	if err := database.QueryRowContext(ctx, `
		SELECT last_read_at FROM thread_reads WHERE user_id = ? AND thread_id = ?`,
		alice.ID, thread.ID).Scan(&lastReadAt); err != nil {
		t.Fatalf("read updated thread_reads row: %v", err)
	}
	if lastReadAt != base.Add(3*time.Minute).Unix() {
		t.Errorf("updated last_read_at = %d, want %d", lastReadAt, base.Add(3*time.Minute).Unix())
	}
}

func TestListThreadsReportsReadStateForViewer(t *testing.T) {
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

	oldThread, err := database.CreateThread(ctx, alice.ID, "Old", "", "body", "<p>body</p>", base.Add(-time.Hour))
	if err != nil {
		t.Fatalf("CreateThread old: %v", err)
	}
	newThread, err := database.CreateThread(ctx, alice.ID, "New", "", "body", "<p>body</p>", base.Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateThread new: %v", err)
	}

	if _, err := database.CreateComment(ctx, newThread.ID, nil, bob.ID, "from bob", "<p>from bob</p>", base.Add(2*time.Hour)); err != nil {
		t.Fatalf("CreateComment from bob: %v", err)
	}
	if _, err := database.CreateComment(ctx, newThread.ID, nil, alice.ID, "from alice", "<p>from alice</p>", base.Add(3*time.Hour)); err != nil {
		t.Fatalf("CreateComment from alice: %v", err)
	}
	deleted, err := database.CreateComment(ctx, newThread.ID, nil, bob.ID, "deleted", "<p>deleted</p>", base.Add(4*time.Hour))
	if err != nil {
		t.Fatalf("CreateComment deleted: %v", err)
	}
	if _, err := database.DeleteComment(ctx, deleted.ID, base.Add(5*time.Hour)); err != nil {
		t.Fatalf("DeleteComment: %v", err)
	}

	// A thread created before the member's unread_since counts as read, and
	// the new thread is unopened until the member reads it. Neither reports
	// new comments: the unopened thread has no previous visit to count from,
	// and the old thread predates the member's account.
	threads := listThreadsByID(t, database, alice.ID)
	assertReadState(t, threads[oldThread.ID], 0, false)
	assertReadState(t, threads[newThread.ID], 0, true)

	// A visitor gets no read state at all.
	visitorThreads := listThreadsByID(t, database, 0)
	assertReadState(t, visitorThreads[oldThread.ID], 0, false)
	assertReadState(t, visitorThreads[newThread.ID], 0, false)

	// Reading the thread clears its unopened marker. The first visit's cutoff
	// is the visit time, so no earlier comment counts as new.
	visit := base.Add(6 * time.Hour)
	cutoff, err := database.MarkThreadRead(ctx, alice.ID, newThread.ID, visit)
	if err != nil {
		t.Fatalf("MarkThreadRead: %v", err)
	}
	if !cutoff.Equal(visit) {
		t.Errorf("cutoff = %v, want the visit time %v", cutoff, visit)
	}
	threads = listThreadsByID(t, database, alice.ID)
	assertReadState(t, threads[newThread.ID], 0, false)

	// Comments after the cutoff count, except the viewer's own.
	if _, err := database.CreateComment(ctx, newThread.ID, nil, bob.ID, "new from bob", "<p>new from bob</p>", base.Add(7*time.Hour)); err != nil {
		t.Fatalf("CreateComment new from bob: %v", err)
	}
	if _, err := database.CreateComment(ctx, newThread.ID, nil, alice.ID, "new from alice", "<p>new from alice</p>", base.Add(8*time.Hour)); err != nil {
		t.Fatalf("CreateComment new from alice: %v", err)
	}
	threads = listThreadsByID(t, database, alice.ID)
	assertReadState(t, threads[newThread.ID], 1, false)
}

func TestThreadReadsCascadeOnUserDelete(t *testing.T) {
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
	thread, err := database.CreateThread(ctx, alice.ID, "Thread", "", "body", "<p>body</p>", now)
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	if _, err := database.MarkThreadRead(ctx, bob.ID, thread.ID, now.Add(time.Minute)); err != nil {
		t.Fatalf("MarkThreadRead: %v", err)
	}

	if _, err := database.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, bob.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	var count int64
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM thread_reads`).Scan(&count); err != nil {
		t.Fatalf("count thread_reads: %v", err)
	}
	if count != 0 {
		t.Errorf("thread_reads rows after deleting the user = %d, want 0", count)
	}
}

func listThreadsByID(t *testing.T, database *DB, viewerID int64) map[int64]Thread {
	t.Helper()

	threads, err := database.ListThreads(context.Background(), viewerID, 10, 0)
	if err != nil {
		t.Fatalf("ListThreads(%d): %v", viewerID, err)
	}
	byID := make(map[int64]Thread, len(threads))
	for _, thread := range threads {
		byID[thread.ID] = thread
	}
	return byID
}

func assertReadState(t *testing.T, thread Thread, wantNewComments int64, wantUnopened bool) {
	t.Helper()

	if thread.NewComments != wantNewComments || thread.Unopened != wantUnopened {
		t.Errorf("thread %d read state = %d new, unopened %t; want %d, %t",
			thread.ID, thread.NewComments, thread.Unopened, wantNewComments, wantUnopened)
	}
}
