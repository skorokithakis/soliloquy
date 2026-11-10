package db

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPasswordResetSingleUseAndExpiry(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	user, err := database.CreateUser(ctx, "alice", "old-hash", false, nil, now)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := database.CreateSession(ctx, user.ID, "old-session", now, now.Add(365*24*time.Hour)); err != nil {
		t.Fatalf("create session: %v", err)
	}

	const token = "reset-token"
	if err := database.CreatePasswordReset(ctx, token, user.ID, now); err != nil {
		t.Fatalf("CreatePasswordReset: %v", err)
	}

	if _, err := database.ActivePasswordReset(ctx, token, now.Add(InviteLifetime-time.Second)); err != nil {
		t.Fatalf("reset link expired too early: %v", err)
	}
	if _, err := database.ActivePasswordReset(ctx, token, now.Add(InviteLifetime)); !errors.Is(err, ErrResetInvalid) {
		t.Errorf("reset at exactly seven days: error = %v, want ErrResetInvalid", err)
	}

	resetUser, err := database.RedeemPasswordReset(ctx, token, "new-hash", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("RedeemPasswordReset: %v", err)
	}
	if resetUser == nil || resetUser.ID != user.ID {
		t.Fatalf("RedeemPasswordReset returned user %v, want %d", resetUser, user.ID)
	}

	updated, err := database.UserByID(ctx, user.ID)
	if err != nil || updated == nil {
		t.Fatalf("UserByID: user = %v, err = %v", updated, err)
	}
	if updated.PasswordHash != "new-hash" {
		t.Errorf("password hash = %q, want new-hash", updated.PasswordHash)
	}

	var sessions int
	if err := database.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = ?`, user.ID).Scan(&sessions); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if sessions != 0 {
		t.Errorf("sessions after reset = %d, want 0", sessions)
	}

	if _, err := database.ActivePasswordReset(ctx, token, now.Add(2*time.Hour)); !errors.Is(err, ErrResetInvalid) {
		t.Errorf("used reset link error = %v, want ErrResetInvalid", err)
	}
	if _, err := database.RedeemPasswordReset(ctx, token, "other-hash", now.Add(2*time.Hour)); !errors.Is(err, ErrResetInvalid) {
		t.Errorf("second RedeemPasswordReset error = %v, want ErrResetInvalid", err)
	}
}

func TestDeleteAccountRemovesPrivateDataAndKeepsContent(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	alice, err := database.CreateUser(ctx, "alice", "hash", false, nil, now)
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, err := database.CreateUser(ctx, "bob", "hash", false, &alice.ID, now)
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}

	thread, err := database.CreateThread(ctx, alice.ID, "Thread", "", "body", "", now)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	if _, err := database.ToggleThreadVote(ctx, bob.ID, thread.ID); err != nil {
		t.Fatalf("bob vote: %v", err)
	}
	if err := database.CreateSession(ctx, bob.ID, "bob-session", now, now.Add(time.Hour)); err != nil {
		t.Fatalf("create bob session: %v", err)
	}
	if _, err := database.CreateNotification(ctx, bob.ID, alice.ID, NotificationReply, thread.ID, nil, now); err != nil {
		t.Fatalf("create notification: %v", err)
	}
	if err := database.SetFeedToken(ctx, bob.ID, "bob-feed-token"); err != nil {
		t.Fatalf("set bob feed token: %v", err)
	}
	if err := database.SetNtfySettings(ctx, bob.ID, "https://ntfy.sh/bob-secret", true, false); err != nil {
		t.Fatalf("set bob ntfy settings: %v", err)
	}
	if err := database.SetFeedToken(ctx, alice.ID, "alice-feed-token"); err != nil {
		t.Fatalf("set alice feed token: %v", err)
	}

	usedInvite := "used-invite"
	if err := database.CreateInvite(ctx, usedInvite, &bob.ID, false, now); err != nil {
		t.Fatalf("create used invite: %v", err)
	}
	if _, err := database.RedeemInvite(ctx, usedInvite, "carol", "hash", now); err != nil {
		t.Fatalf("redeem invite: %v", err)
	}
	if err := database.CreateInvite(ctx, "unused-invite", &bob.ID, false, now); err != nil {
		t.Fatalf("create unused invite: %v", err)
	}
	if err := database.CreatePasswordReset(ctx, "reset", bob.ID, now); err != nil {
		t.Fatalf("create reset: %v", err)
	}

	if err := database.DeleteAccount(ctx, bob.ID, now.Add(time.Hour)); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}

	deleted, err := database.UserByID(ctx, bob.ID)
	if err != nil || deleted == nil {
		t.Fatalf("UserByID: user = %v, err = %v", deleted, err)
	}
	if deleted.DeletedAt == nil {
		t.Error("account was not soft-deleted")
	}

	assertCount(t, database, `SELECT COUNT(*) FROM sessions WHERE user_id = ?`, bob.ID, 0)
	assertCount(t, database, `SELECT COUNT(*) FROM notifications WHERE recipient_id = ?`, bob.ID, 0)
	assertCount(t, database, `SELECT COUNT(*) FROM invites WHERE created_by = ? AND used_at IS NULL`, bob.ID, 0)
	assertCount(t, database, `SELECT COUNT(*) FROM password_resets WHERE user_id = ? AND used_at IS NULL`, bob.ID, 0)

	// The push URL and feed token are secrets and are cleared to their
	// "not set" values; the feed token must be NULL, not an empty string,
	// so a future SetFeedTokenIfEmpty still stores a token.
	assertCount(t, database, `SELECT COUNT(*) FROM users WHERE id = ? AND ntfy_url = ''`, bob.ID, 1)
	assertCount(t, database, `SELECT COUNT(*) FROM users WHERE id = ? AND feed_token IS NULL`, bob.ID, 1)

	// Other users keep their own.
	kept, err := database.UserByID(ctx, alice.ID)
	if err != nil || kept == nil {
		t.Fatalf("UserByID(alice): user = %v, err = %v", kept, err)
	}
	if kept.FeedToken != "alice-feed-token" {
		t.Errorf("alice feed token = %q, want it unchanged", kept.FeedToken)
	}

	// Content and votes stay.
	stored, err := database.ThreadByID(ctx, thread.ID)
	if err != nil || stored == nil {
		t.Fatalf("thread after account deletion: thread = %v, err = %v", stored, err)
	}
	if stored.DeletedAt != nil {
		t.Error("account deletion deleted the user's thread")
	}
	assertCount(t, database, `SELECT COUNT(*) FROM votes WHERE user_id = ?`, bob.ID, 1)
	assertCount(t, database, `SELECT COUNT(*) FROM invites WHERE created_by = ? AND used_at IS NOT NULL`, bob.ID, 1)

	// The username stays reserved.
	if _, err := database.CreateUser(ctx, "bob", "hash", false, nil, now); !errors.Is(err, ErrUsernameTaken) {
		t.Errorf("creating the deleted username error = %v, want ErrUsernameTaken", err)
	}
}

func TestLastActiveAdminCannotDeleteAccount(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	admin, err := database.CreateUser(ctx, "admin", "hash", true, nil, now)
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}

	if err := database.DeleteAccount(ctx, admin.ID, now); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("DeleteAccount as last admin error = %v, want ErrLastAdmin", err)
	}
	stillThere, err := database.UserByID(ctx, admin.ID)
	if err != nil || stillThere == nil || stillThere.DeletedAt != nil {
		t.Fatalf("last admin was deleted: user = %+v, err = %v", stillThere, err)
	}

	other, err := database.CreateUser(ctx, "other", "hash", true, nil, now)
	if err != nil {
		t.Fatalf("create second admin: %v", err)
	}
	if err := database.DeleteAccount(ctx, admin.ID, now); err != nil {
		t.Fatalf("DeleteAccount with another active admin: %v", err)
	}

	// A disabled admin does not count as active.
	third, err := database.CreateUser(ctx, "third", "hash", true, nil, now)
	if err != nil {
		t.Fatalf("create third admin: %v", err)
	}
	if _, err := database.DisableUser(ctx, other.ID, now); err != nil {
		t.Fatalf("disable second admin: %v", err)
	}
	if err := database.DeleteAccount(ctx, third.ID, now); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("deleting the only active admin error = %v, want ErrLastAdmin", err)
	}
}

func TestLastActiveAdminCannotBeDisabledOrDemoted(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	admin, err := database.CreateUser(ctx, "admin", "hash", true, nil, now)
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}

	if _, err := database.DisableUser(ctx, admin.ID, now); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("DisableUser as last admin error = %v, want ErrLastAdmin", err)
	}
	if _, err := database.SetAdmin(ctx, admin.ID, false); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("SetAdmin(false) as last admin error = %v, want ErrLastAdmin", err)
	}

	stillAdmin, err := database.UserByID(ctx, admin.ID)
	if err != nil || stillAdmin == nil {
		t.Fatalf("UserByID: user = %v, err = %v", stillAdmin, err)
	}
	if !stillAdmin.Active() || !stillAdmin.IsAdmin {
		t.Fatalf("last admin was disabled or demoted: user = %+v", stillAdmin)
	}

	other, err := database.CreateUser(ctx, "other", "hash", true, nil, now)
	if err != nil {
		t.Fatalf("create second admin: %v", err)
	}
	if disabled, err := database.DisableUser(ctx, admin.ID, now); err != nil || !disabled {
		t.Fatalf("DisableUser with another active admin: disabled = %v, err = %v", disabled, err)
	}
	if _, err := database.SetAdmin(ctx, other.ID, false); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("demoting the only active admin error = %v, want ErrLastAdmin", err)
	}

	if _, err := database.CreateUser(ctx, "third", "hash", true, nil, now); err != nil {
		t.Fatalf("create third admin: %v", err)
	}
	if changed, err := database.SetAdmin(ctx, other.ID, false); err != nil || !changed {
		t.Fatalf("SetAdmin(false) with another active admin: changed = %v, err = %v", changed, err)
	}
}

func TestDisableUserEndsSessionsAndCancelsUnusedInvites(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	user, err := database.CreateUser(ctx, "alice", "hash", false, nil, now)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := database.CreateSession(ctx, user.ID, "session", now, now.Add(time.Hour)); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := database.CreateInvite(ctx, "unused", &user.ID, false, now); err != nil {
		t.Fatalf("create unused invite: %v", err)
	}
	if err := database.CreateInvite(ctx, "used", &user.ID, false, now); err != nil {
		t.Fatalf("create used invite: %v", err)
	}
	if _, err := database.RedeemInvite(ctx, "used", "bob", "hash", now); err != nil {
		t.Fatalf("redeem invite: %v", err)
	}

	disabled, err := database.DisableUser(ctx, user.ID, now.Add(time.Minute))
	if err != nil || !disabled {
		t.Fatalf("DisableUser: disabled = %v, err = %v", disabled, err)
	}
	updated, err := database.UserByID(ctx, user.ID)
	if err != nil || updated == nil {
		t.Fatalf("UserByID: user = %v, err = %v", updated, err)
	}
	if updated.DisabledAt == nil {
		t.Error("disabled_at was not set")
	}

	assertCount(t, database, `SELECT COUNT(*) FROM sessions WHERE user_id = ?`, user.ID, 0)
	assertCount(t, database, `SELECT COUNT(*) FROM invites WHERE created_by = ? AND used_at IS NULL`, user.ID, 0)
	assertCount(t, database, `SELECT COUNT(*) FROM invites WHERE created_by = ? AND used_at IS NOT NULL`, user.ID, 1)
}

func TestBuildInviteTreeIncludesDeletedAndDisabledUsers(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	admin, err := database.CreateUser(ctx, "admin", "hash", true, nil, now)
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	bob, err := database.CreateUser(ctx, "bob", "hash", false, &admin.ID, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}
	carol, err := database.CreateUser(ctx, "carol", "hash", false, &bob.ID, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("create carol: %v", err)
	}
	if _, err := database.DisableUser(ctx, bob.ID, now.Add(time.Hour)); err != nil {
		t.Fatalf("disable bob: %v", err)
	}
	if err := database.DeleteAccount(ctx, carol.ID, now.Add(time.Hour)); err != nil {
		t.Fatalf("delete carol: %v", err)
	}

	users, err := database.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	roots := BuildInviteTree(users)
	if len(roots) != 1 || roots[0].User.ID != admin.ID {
		t.Fatalf("roots = %+v, want only the first admin", roots)
	}

	if len(roots[0].Children) != 1 || roots[0].Children[0].User.ID != bob.ID {
		t.Fatalf("admin children = %+v, want bob", roots[0].Children)
	}
	bobNode := roots[0].Children[0]
	if bobNode.User.DisabledAt == nil {
		t.Error("disabled user is not marked in the tree")
	}
	if len(bobNode.Children) != 1 || bobNode.Children[0].User.ID != carol.ID {
		t.Fatalf("bob children = %+v, want carol", bobNode.Children)
	}
	if name := bobNode.Children[0].User.DisplayName(); name != "[deleted user]" {
		t.Errorf("deleted user display name = %q, want [deleted user]", name)
	}
}

func assertCount(t *testing.T, database *DB, query string, arg any, want int) {
	t.Helper()

	var got int
	if err := database.QueryRow(query, arg).Scan(&got); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	if got != want {
		t.Errorf("count for %q = %d, want %d", query, got, want)
	}
}
