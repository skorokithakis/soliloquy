package db

import (
	"context"
	"testing"
	"time"
)

func TestSetFeedTokenIfEmptyKeepsExistingToken(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	user, err := database.CreateUser(ctx, "alice", "hash", false, nil, now)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	first, err := database.SetFeedTokenIfEmpty(ctx, user.ID, "first-token")
	if err != nil {
		t.Fatalf("set first token: %v", err)
	}
	if first != "first-token" {
		t.Fatalf("stored token = %q, want first-token", first)
	}

	second, err := database.SetFeedTokenIfEmpty(ctx, user.ID, "second-token")
	if err != nil {
		t.Fatalf("set second token: %v", err)
	}
	if second != "first-token" {
		t.Fatalf("second ensure stored %q, want the existing first-token", second)
	}

	found, err := database.UserByFeedToken(ctx, "first-token")
	if err != nil || found == nil || found.ID != user.ID {
		t.Fatalf("lookup by first token = %+v, err = %v", found, err)
	}
	if found, err := database.UserByFeedToken(ctx, "second-token"); err != nil || found != nil {
		t.Fatalf("lookup by discarded token = %+v, err = %v", found, err)
	}
}
