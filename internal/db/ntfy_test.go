package db

import (
	"context"
	"testing"
	"time"
)

func TestNtfySettingsRoundTrip(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()

	user, err := database.CreateUser(ctx, "alice", "hash", false, nil, time.Unix(1_700_000_000, 0).UTC())
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Defaults: no topic, personal notifications on, new threads off.
	if user.NtfyURL != "" || !user.NtfyReplies || user.NtfyThreads {
		t.Errorf("new user ntfy settings = %+v, want empty URL, replies on, threads off", user)
	}

	if err := database.SetNtfySettings(ctx, user.ID, "https://ntfy.sh/a-secret-topic", false, true); err != nil {
		t.Fatalf("SetNtfySettings: %v", err)
	}

	stored, err := database.UserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("UserByID: %v", err)
	}
	if stored.NtfyURL != "https://ntfy.sh/a-secret-topic" || stored.NtfyReplies || !stored.NtfyThreads {
		t.Errorf("stored ntfy settings = %+v", stored)
	}

	// An empty URL turns pushes off without touching the options.
	if err := database.SetNtfySettings(ctx, user.ID, "", true, true); err != nil {
		t.Fatalf("SetNtfySettings off: %v", err)
	}
	stored, err = database.UserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("UserByID after off: %v", err)
	}
	if stored.NtfyURL != "" || !stored.NtfyReplies || !stored.NtfyThreads {
		t.Errorf("settings after turning off = %+v", stored)
	}
}
