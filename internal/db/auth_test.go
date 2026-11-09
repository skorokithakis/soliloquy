package db

import (
	"context"
	"errors"
	"testing"
	"time"
)

func migratedTestDB(t *testing.T) *DB {
	t.Helper()

	database := openTestDB(t)
	if err := database.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return database
}

func TestInviteSingleUse(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	inviter, err := database.CreateUser(ctx, "alice", "hash", false, nil, now)
	if err != nil {
		t.Fatalf("create inviter: %v", err)
	}

	const token = "invite-token"
	if err := database.CreateInvite(ctx, token, &inviter.ID, false, now); err != nil {
		t.Fatalf("create invite: %v", err)
	}

	invite, err := database.ActiveInvite(ctx, token, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("ActiveInvite: %v", err)
	}
	if invite.CreatedBy == nil || *invite.CreatedBy != inviter.ID {
		t.Errorf("CreatedBy = %v, want %d", invite.CreatedBy, inviter.ID)
	}
	if invite.IsAdmin {
		t.Error("normal invite should not create an admin")
	}

	user, err := database.RedeemInvite(ctx, token, "bob", "hash", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("RedeemInvite: %v", err)
	}
	if user.InvitedBy == nil || *user.InvitedBy != inviter.ID {
		t.Errorf("InvitedBy = %v, want %d", user.InvitedBy, inviter.ID)
	}
	if user.IsAdmin {
		t.Error("normal invite created an admin")
	}

	if _, err := database.ActiveInvite(ctx, token, now.Add(2*time.Hour)); !errors.Is(err, ErrInviteInvalid) {
		t.Errorf("ActiveInvite after use error = %v, want ErrInviteInvalid", err)
	}
	if _, err := database.RedeemInvite(ctx, token, "carol", "hash", now.Add(2*time.Hour)); !errors.Is(err, ErrInviteInvalid) {
		t.Errorf("second RedeemInvite error = %v, want ErrInviteInvalid", err)
	}

	invites, err := database.ListActiveInvites(ctx, inviter.ID, now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("ListActiveInvites: %v", err)
	}
	if len(invites) != 0 {
		t.Errorf("active invites = %d, want 0", len(invites))
	}
}

func TestInviteExpiresAfterSevenDays(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	inviter, err := database.CreateUser(ctx, "alice", "hash", false, nil, now)
	if err != nil {
		t.Fatalf("create inviter: %v", err)
	}
	if err := database.CreateInvite(ctx, "invite-token", &inviter.ID, false, now); err != nil {
		t.Fatalf("create invite: %v", err)
	}

	if _, err := database.ActiveInvite(ctx, "invite-token", now.Add(InviteLifetime-time.Second)); err != nil {
		t.Fatalf("invite expired too early: %v", err)
	}
	if _, err := database.ActiveInvite(ctx, "invite-token", now.Add(InviteLifetime)); !errors.Is(err, ErrInviteInvalid) {
		t.Errorf("invite at exactly seven days: error = %v, want ErrInviteInvalid", err)
	}

	invites, err := database.ListActiveInvites(ctx, inviter.ID, now.Add(InviteLifetime))
	if err != nil {
		t.Fatalf("ListActiveInvites: %v", err)
	}
	if len(invites) != 0 {
		t.Errorf("expired invite still listed")
	}
}

func TestUsernamesAreUniqueCaseInsensitively(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	if _, err := database.CreateUser(ctx, "alice", "hash", false, nil, now); err != nil {
		t.Fatalf("create alice: %v", err)
	}
	if _, err := database.CreateUser(ctx, "ALICE", "hash", false, nil, now); !errors.Is(err, ErrUsernameTaken) {
		t.Errorf("case-insensitive duplicate error = %v, want ErrUsernameTaken", err)
	}
}

func TestSessionLifecycle(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	user, err := database.CreateUser(ctx, "alice", "hash", false, nil, now)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	const tokenHash = "session-hash"
	if err := database.CreateSession(ctx, user.ID, tokenHash, now, now.Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	session, gotUser, err := database.SessionUser(ctx, tokenHash, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("SessionUser: %v", err)
	}
	if session == nil || gotUser == nil || gotUser.ID != user.ID {
		t.Fatalf("SessionUser = %v, %v; want the session and user", session, gotUser)
	}

	extended := now.Add(2 * time.Hour)
	if err := database.ExtendSession(ctx, tokenHash, extended); err != nil {
		t.Fatalf("ExtendSession: %v", err)
	}
	if _, _, err := database.SessionUser(ctx, tokenHash, now.Add(90*time.Minute)); err != nil {
		t.Fatalf("SessionUser after extension: %v", err)
	}

	session, gotUser, err = database.SessionUser(ctx, tokenHash, extended)
	if err != nil {
		t.Fatalf("SessionUser at expiry: %v", err)
	}
	if session != nil || gotUser != nil {
		t.Fatal("expired session was accepted")
	}

	var count int
	if err := database.QueryRow("SELECT COUNT(*) FROM sessions WHERE token_hash = ?", tokenHash).Scan(&count); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if count != 0 {
		t.Errorf("expired session row count = %d, want 0", count)
	}
}

func TestDisabledUserSessionIsRejected(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	user, err := database.CreateUser(ctx, "alice", "hash", false, nil, now)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := database.CreateSession(ctx, user.ID, "session-hash", now, now.Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, err := database.Exec("UPDATE users SET disabled_at = ? WHERE id = ?", now.Unix(), user.ID); err != nil {
		t.Fatalf("disable user: %v", err)
	}

	session, gotUser, err := database.SessionUser(ctx, "session-hash", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("SessionUser: %v", err)
	}
	if session != nil || gotUser != nil {
		t.Fatal("session for disabled user was accepted")
	}
}

func TestCreateSessionIfPasswordRejectsStaleHash(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	user, err := database.CreateUser(ctx, "alice", "current-hash", false, nil, now)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	created, err := database.CreateSessionIfPassword(ctx, user.ID, "stale-hash", "stale-session", now, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateSessionIfPassword with stale hash: %v", err)
	}
	if created {
		t.Fatal("created a session for a stale password hash")
	}

	var count int
	if err := database.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&count); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if count != 0 {
		t.Errorf("sessions = %d, want 0", count)
	}

	created, err = database.CreateSessionIfPassword(ctx, user.ID, "current-hash", "current-session", now, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateSessionIfPassword with current hash: %v", err)
	}
	if !created {
		t.Fatal("did not create a session for the current password hash")
	}
	if _, gotUser, err := database.SessionUser(ctx, "current-session", now.Add(time.Minute)); err != nil || gotUser == nil {
		t.Errorf("session lookup: user = %v, err = %v; want a user", gotUser, err)
	}
}

func TestChangePasswordKeepsCurrentSession(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	user, err := database.CreateUser(ctx, "alice", "old-hash", false, nil, now)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	for _, hash := range []string{"current", "other"} {
		if err := database.CreateSession(ctx, user.ID, hash, now, now.Add(time.Hour)); err != nil {
			t.Fatalf("CreateSession(%q): %v", hash, err)
		}
	}

	changed, err := database.ChangePassword(ctx, user.ID, "stale-hash", "new-hash", "current")
	if err != nil {
		t.Fatalf("ChangePassword with stale hash: %v", err)
	}
	if changed {
		t.Fatal("changed the password with a stale expected hash")
	}
	if _, gotUser, err := database.SessionUser(ctx, "other", now.Add(time.Minute)); err != nil || gotUser == nil {
		t.Errorf("stale password change ended sessions: user = %v, err = %v", gotUser, err)
	}

	changed, err = database.ChangePassword(ctx, user.ID, "old-hash", "new-hash", "current")
	if err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if !changed {
		t.Fatal("ChangePassword did not update the current password hash")
	}

	if _, gotUser, err := database.SessionUser(ctx, "current", now.Add(time.Minute)); err != nil || gotUser == nil {
		t.Errorf("current session: user = %v, err = %v; want a user", gotUser, err)
	}
	if _, gotUser, err := database.SessionUser(ctx, "other", now.Add(time.Minute)); err != nil || gotUser != nil {
		t.Errorf("other session: user = %v, err = %v; want no user", gotUser, err)
	}

	updated, err := database.UserByID(ctx, user.ID)
	if err != nil || updated == nil {
		t.Fatalf("UserByID: user = %v, err = %v", updated, err)
	}
	if updated.PasswordHash != "new-hash" {
		t.Errorf("password hash = %q, want new-hash", updated.PasswordHash)
	}
}

func TestEnsureBootstrapInvite(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	created, err := database.EnsureBootstrapInvite(ctx, "first", now)
	if err != nil || !created {
		t.Fatalf("EnsureBootstrapInvite: created = %v, err = %v", created, err)
	}

	invite, err := database.ActiveInvite(ctx, "first", now)
	if err != nil {
		t.Fatalf("ActiveInvite: %v", err)
	}
	if !invite.IsAdmin || invite.CreatedBy != nil {
		t.Errorf("bootstrap invite = admin %v, creator %v; want admin with no creator", invite.IsAdmin, invite.CreatedBy)
	}

	// Restarting before setup replaces the old unused invite.
	created, err = database.EnsureBootstrapInvite(ctx, "second", now)
	if err != nil || !created {
		t.Fatalf("second EnsureBootstrapInvite: created = %v, err = %v", created, err)
	}
	if _, err := database.ActiveInvite(ctx, "first", now); !errors.Is(err, ErrInviteInvalid) {
		t.Errorf("old bootstrap invite error = %v, want ErrInviteInvalid", err)
	}

	user, err := database.RedeemInvite(ctx, "second", "admin", "hash", now)
	if err != nil {
		t.Fatalf("RedeemInvite: %v", err)
	}
	if !user.IsAdmin {
		t.Error("bootstrap invite did not create an admin")
	}

	created, err = database.EnsureBootstrapInvite(ctx, "third", now)
	if err != nil {
		t.Fatalf("EnsureBootstrapInvite after setup: %v", err)
	}
	if created {
		t.Error("created another bootstrap invite after a user existed")
	}
}
