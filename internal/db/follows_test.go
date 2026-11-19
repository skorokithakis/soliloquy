package db

import (
	"context"
	"testing"
	"time"
)

func TestFollowThreadIsIdempotent(t *testing.T) {
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

	if err := database.FollowThread(ctx, bob.ID, thread.ID, now); err != nil {
		t.Fatalf("FollowThread: %v", err)
	}
	if err := database.FollowThread(ctx, bob.ID, thread.ID, now.Add(time.Minute)); err != nil {
		t.Fatalf("second FollowThread: %v", err)
	}

	follows, err := database.FollowsThread(ctx, bob.ID, thread.ID)
	if err != nil || !follows {
		t.Fatalf("FollowsThread = %t, %v; want true, nil", follows, err)
	}
	followers, err := database.ThreadFollowers(ctx, thread.ID)
	if err != nil {
		t.Fatalf("ThreadFollowers: %v", err)
	}
	if len(followers) != 1 || followers[0] != bob.ID {
		t.Errorf("followers = %v, want [%d]", followers, bob.ID)
	}

	if err := database.UnfollowThread(ctx, bob.ID, thread.ID); err != nil {
		t.Fatalf("UnfollowThread: %v", err)
	}
	if err := database.UnfollowThread(ctx, bob.ID, thread.ID); err != nil {
		t.Fatalf("second UnfollowThread: %v", err)
	}
	follows, err = database.FollowsThread(ctx, bob.ID, thread.ID)
	if err != nil || follows {
		t.Errorf("FollowsThread after unfollow = %t, %v; want false, nil", follows, err)
	}
	followers, err = database.ThreadFollowers(ctx, thread.ID)
	if err != nil {
		t.Fatalf("ThreadFollowers after unfollow: %v", err)
	}
	if len(followers) != 0 {
		t.Errorf("followers after unfollow = %v, want none", followers)
	}
}

func TestThreadFollowersOnlyReturnsActiveMembers(t *testing.T) {
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
	carol, err := database.CreateUser(ctx, "carol", "hash", false, nil, now)
	if err != nil {
		t.Fatalf("create carol: %v", err)
	}
	thread, err := database.CreateThread(ctx, alice.ID, "Thread", "", "body", "<p>body</p>", now)
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	for _, user := range []*User{bob, carol} {
		if err := database.FollowThread(ctx, user.ID, thread.ID, now); err != nil {
			t.Fatalf("FollowThread(%s): %v", user.Username, err)
		}
	}

	if _, err := database.DisableUser(ctx, carol.ID, now.Add(time.Minute)); err != nil {
		t.Fatalf("DisableUser: %v", err)
	}

	followers, err := database.ThreadFollowers(ctx, thread.ID)
	if err != nil {
		t.Fatalf("ThreadFollowers: %v", err)
	}
	if len(followers) != 1 || followers[0] != bob.ID {
		t.Errorf("followers = %v, want only bob", followers)
	}

	// Disabling keeps the row: re-enabling the account restores the follow.
	if follows, err := database.FollowsThread(ctx, carol.ID, thread.ID); err != nil || !follows {
		t.Errorf("disabled carol FollowsThread = %t, %v; want true, nil", follows, err)
	}
	assertCount(t, database, `SELECT COUNT(*) FROM thread_follows WHERE thread_id = ?`, thread.ID, 2)
}

func TestThreadFollowsCascadeOnUserDelete(t *testing.T) {
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
	if err := database.FollowThread(ctx, bob.ID, thread.ID, now); err != nil {
		t.Fatalf("FollowThread: %v", err)
	}

	if _, err := database.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, bob.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	var count int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM thread_follows`).Scan(&count); err != nil {
		t.Fatalf("count thread_follows: %v", err)
	}
	if count != 0 {
		t.Errorf("thread_follows rows after deleting the user = %d, want 0", count)
	}
}
