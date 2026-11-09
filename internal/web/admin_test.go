package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/skorokithakis/soliloquy/internal/auth"
	"github.com/skorokithakis/soliloquy/internal/db"
)

func TestAdminUsersPage(t *testing.T) {
	server := newTestServer(t)
	admin := createTestUser(t, server, "admin", true)
	bob := createInvitedTestUser(t, server, "bob", false, admin)
	carol := createInvitedTestUser(t, server, "carol", false, bob)
	dave := createInvitedTestUser(t, server, "dave", false, carol)
	cookie := signIn(t, server, admin)

	if _, err := server.db.DisableUser(context.Background(), bob.ID, server.now()); err != nil {
		t.Fatalf("disable bob: %v", err)
	}
	if err := server.db.DeleteAccount(context.Background(), carol.ID, server.now()); err != nil {
		t.Fatalf("delete carol: %v", err)
	}

	response := doRequest(t, server, http.MethodGet, "/admin/users", nil, cookie, "")
	if response.Code != http.StatusOK {
		t.Fatalf("admin users status = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	for _, want := range []string{
		"admin/users",
		"3 accounts · 1 admin · 1 disabled",
		"bob",
		"disabled",
		"[deleted user]",
		"dave",
		`data-username="dave"`,
		fmt.Sprintf(`action="/admin/users/%d/reset"`, dave.ID),
		fmt.Sprintf(`action="/admin/users/%d/grant-admin"`, dave.ID),
		fmt.Sprintf(`action="/admin/users/%d/disable"`, dave.ID),
		`title="You cannot remove your own admin rights"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("admin page does not contain %q", want)
		}
	}
	for _, unwanted := range []string{
		fmt.Sprintf(`action="/admin/users/%d/reset"`, bob.ID), // disabled users have no actions
		fmt.Sprintf(`action="/admin/users/%d/disable"`, bob.ID),
		fmt.Sprintf(`action="/admin/users/%d/disable"`, admin.ID), // cannot disable self
		fmt.Sprintf(`action="/admin/users/%d/reset"`, admin.ID),   // self row only shows the disabled admin button
		`data-username="carol"`,                                   // deleted accounts are omitted
	} {
		if strings.Contains(body, unwanted) {
			t.Errorf("admin page contains unexpected control %q", unwanted)
		}
	}
}

func TestAdminUsersPageRequiresAdmin(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)

	response := doRequest(t, server, http.MethodGet, "/admin/users", nil, cookie, "")
	if response.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestAdminActionsRequireAdmin(t *testing.T) {
	server := newTestServer(t)
	admin := createTestUser(t, server, "admin", true)
	user := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, user)

	paths := []string{
		fmt.Sprintf("/admin/users/%d/disable", admin.ID),
		fmt.Sprintf("/admin/users/%d/reset", admin.ID),
		fmt.Sprintf("/admin/users/%d/grant-admin", user.ID),
		fmt.Sprintf("/admin/users/%d/revoke-admin", admin.ID),
	}
	for _, path := range paths {
		response := postForm(t, server, path, nil, cookie)
		if response.Code != http.StatusForbidden {
			t.Errorf("%s as normal user: status = %d, want %d", path, response.Code, http.StatusForbidden)
		}
	}

	stillAdmin, err := server.db.UserByID(context.Background(), admin.ID)
	if err != nil || stillAdmin == nil {
		t.Fatalf("UserByID: user = %v, err = %v", stillAdmin, err)
	}
	if !stillAdmin.IsAdmin {
		t.Error("normal user changed admin rights")
	}
}

func TestAdminDisablesUser(t *testing.T) {
	server := newTestServer(t)
	admin := createTestUser(t, server, "admin", true)
	bob := createTestUser(t, server, "bob", false)
	adminCookie := signIn(t, server, admin)
	bobCookie := signIn(t, server, bob)
	if err := server.db.CreateInvite(context.Background(), "bob-invite", &bob.ID, false, server.now()); err != nil {
		t.Fatalf("create bob invite: %v", err)
	}

	response := postForm(t, server, fmt.Sprintf("/admin/users/%d/disable", bob.ID), nil, adminCookie)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/admin/users" {
		t.Fatalf("disable status = %d, Location = %q", response.Code, response.Header().Get("Location"))
	}

	updated, err := server.db.UserByID(context.Background(), bob.ID)
	if err != nil || updated == nil {
		t.Fatalf("UserByID: user = %v, err = %v", updated, err)
	}
	if updated.DisabledAt == nil {
		t.Error("disable did not set disabled_at")
	}
	if invites, err := server.db.ListActiveInvites(context.Background(), bob.ID, server.now()); err != nil || len(invites) != 0 {
		t.Errorf("disabled user invites = %d, err = %v; want none", len(invites), err)
	}

	response = doRequest(t, server, http.MethodGet, "/", nil, bobCookie, "")
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login" {
		t.Errorf("disabled user session: status = %d, Location = %q", response.Code, response.Header().Get("Location"))
	}
	response = postForm(t, server, "/login", url.Values{
		"username": {"bob"},
		"password": {testPassword},
	}, nil)
	if response.Code != http.StatusUnauthorized {
		t.Errorf("disabled user login status = %d, want %d", response.Code, http.StatusUnauthorized)
	}

	// Admins cannot disable themselves.
	response = postForm(t, server, fmt.Sprintf("/admin/users/%d/disable", admin.ID), nil, adminCookie)
	if response.Code != http.StatusForbidden {
		t.Errorf("self-disable status = %d, want %d", response.Code, http.StatusForbidden)
	}
	stillActive, err := server.db.UserByID(context.Background(), admin.ID)
	if err != nil || stillActive == nil || stillActive.DisabledAt != nil {
		t.Errorf("admin disabled themselves: user = %+v, err = %v", stillActive, err)
	}
}

func TestAdminGrantAndRevokeAdmin(t *testing.T) {
	server := newTestServer(t)
	admin := createTestUser(t, server, "admin", true)
	bob := createTestUser(t, server, "bob", false)
	adminCookie := signIn(t, server, admin)

	// An admin cannot remove their own rights.
	response := postForm(t, server, fmt.Sprintf("/admin/users/%d/revoke-admin", admin.ID), nil, adminCookie)
	if response.Code != http.StatusForbidden {
		t.Errorf("self-revoke status = %d, want %d", response.Code, http.StatusForbidden)
	}
	stillAdmin, err := server.db.UserByID(context.Background(), admin.ID)
	if err != nil || stillAdmin == nil || !stillAdmin.IsAdmin {
		t.Errorf("admin removed their own rights: user = %+v, err = %v", stillAdmin, err)
	}

	response = postForm(t, server, fmt.Sprintf("/admin/users/%d/grant-admin", bob.ID), nil, adminCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("grant status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	bobUser, err := server.db.UserByID(context.Background(), bob.ID)
	if err != nil || bobUser == nil {
		t.Fatalf("UserByID: user = %v, err = %v", bobUser, err)
	}
	if !bobUser.IsAdmin {
		t.Error("grant did not make bob an admin")
	}

	bobCookie := signIn(t, server, bob)
	response = postForm(t, server, fmt.Sprintf("/admin/users/%d/revoke-admin", bob.ID), nil, bobCookie)
	if response.Code != http.StatusForbidden {
		t.Errorf("new admin self-revoke status = %d, want %d", response.Code, http.StatusForbidden)
	}

	response = postForm(t, server, fmt.Sprintf("/admin/users/%d/revoke-admin", bob.ID), nil, adminCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("revoke status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	bobUser, err = server.db.UserByID(context.Background(), bob.ID)
	if err != nil || bobUser == nil {
		t.Fatalf("UserByID: user = %v, err = %v", bobUser, err)
	}
	if bobUser.IsAdmin {
		t.Error("revoke left bob an admin")
	}
}

func TestAdminDeletesAnyThreadAndComment(t *testing.T) {
	server := newTestServer(t)
	admin := createTestUser(t, server, "admin", true)
	bob := createTestUser(t, server, "bob", false)
	adminCookie := signIn(t, server, admin)
	thread := createThreadDirect(t, server, bob, "Bob's thread", "", "body", server.now())
	comment := createCommentDirect(t, server, thread, bob, nil, "Bob's comment", server.now())

	page := doRequest(t, server, http.MethodGet, threadPath(thread.ID), nil, adminCookie, "")
	if page.Code != http.StatusOK {
		t.Fatalf("thread page status = %d", page.Code)
	}
	for _, want := range []string{
		fmt.Sprintf(`action="/threads/%d/delete"`, thread.ID),
		fmt.Sprintf(`action="/comments/%d/delete"`, comment.ID),
	} {
		if !strings.Contains(page.Body.String(), want) {
			t.Errorf("admin thread page does not contain %q", want)
		}
	}

	response := postForm(t, server, commentAction(comment.ID, "delete"), nil, adminCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("admin comment delete status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	deletedComment, err := server.db.CommentByID(context.Background(), comment.ID, 0)
	if err != nil || deletedComment == nil {
		t.Fatalf("CommentByID: comment = %v, err = %v", deletedComment, err)
	}
	if deletedComment.DeletedAt == nil {
		t.Error("admin comment delete did not soft-delete the comment")
	}

	response = postForm(t, server, threadPath(thread.ID)+"/delete", nil, adminCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("admin thread delete status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	deletedThread, err := server.db.ThreadByID(context.Background(), thread.ID)
	if err != nil || deletedThread == nil {
		t.Fatalf("ThreadByID: thread = %v, err = %v", deletedThread, err)
	}
	if deletedThread.DeletedAt == nil {
		t.Error("admin thread delete did not soft-delete the thread")
	}
}

func TestDeleteOwnAccount(t *testing.T) {
	server := newTestServer(t)
	admin := createTestUser(t, server, "admin", true)
	cookie := signIn(t, server, admin)

	response := postForm(t, server, "/account/delete", url.Values{"username": {"wrong"}}, cookie)
	if response.Code != http.StatusBadRequest {
		t.Errorf("wrong password status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	stored, err := server.db.UserByID(context.Background(), admin.ID)
	if err != nil || stored == nil || stored.DeletedAt != nil {
		t.Fatalf("account deleted with a wrong password: user = %+v, err = %v", stored, err)
	}

	response = postForm(t, server, "/account/delete", url.Values{"username": {"admin"}}, cookie)
	if response.Code != http.StatusBadRequest {
		t.Errorf("last admin delete status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if !strings.Contains(response.Body.String(), "last active admin") {
		t.Error("last-admin error message is missing")
	}

	secondAdmin := createTestUser(t, server, "second", true)
	if secondAdmin == nil {
		t.Fatal("failed to create second admin")
	}
	response = postForm(t, server, "/account/delete", url.Values{"username": {"admin"}}, cookie)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login" {
		t.Fatalf("delete status = %d, Location = %q", response.Code, response.Header().Get("Location"))
	}

	deleted, err := server.db.UserByID(context.Background(), admin.ID)
	if err != nil || deleted == nil {
		t.Fatalf("UserByID: user = %v, err = %v", deleted, err)
	}
	if deleted.DeletedAt == nil {
		t.Error("account was not deleted")
	}
	response = doRequest(t, server, http.MethodGet, "/", nil, cookie, "")
	if response.Code != http.StatusSeeOther {
		t.Errorf("deleted user's session still works: status = %d", response.Code)
	}
}

func TestPasswordResetLinkFlow(t *testing.T) {
	server := newTestServer(t)
	admin := createTestUser(t, server, "admin", true)
	bob := createTestUser(t, server, "bob", false)
	adminCookie := signIn(t, server, admin)
	bobCookie := signIn(t, server, bob)

	response := postForm(t, server, fmt.Sprintf("/admin/users/%d/reset", bob.ID), nil, adminCookie)
	if response.Code != http.StatusOK {
		t.Fatalf("create reset status = %d, want %d", response.Code, http.StatusOK)
	}
	token := resetTokenFromPage(t, response.Body.String())
	if !strings.Contains(response.Body.String(), `class="notice"`) || !strings.Contains(response.Body.String(), `class="highlight"`) {
		t.Error("admin page does not show the reset banner or highlighted row")
	}

	page := doRequest(t, server, http.MethodGet, "/reset/"+token, nil, nil, "")
	if page.Code != http.StatusOK {
		t.Fatalf("reset page status = %d, want %d", page.Code, http.StatusOK)
	}
	if !strings.Contains(page.Body.String(), fmt.Sprintf(`action="/reset/%s"`, token)) {
		t.Error("reset page does not post to the token URL")
	}

	response = postForm(t, server, "/reset/"+token, url.Values{
		"password":         {"newpassword123"},
		"confirm_password": {"different"},
	}, nil)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("mismatched password status = %d, want %d", response.Code, http.StatusBadRequest)
	}

	newPassword := "newpassword123"
	response = postForm(t, server, "/reset/"+token, url.Values{
		"password":         {newPassword},
		"confirm_password": {newPassword},
	}, nil)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login" {
		t.Fatalf("reset status = %d, Location = %q", response.Code, response.Header().Get("Location"))
	}

	updated, err := server.db.UserByID(context.Background(), bob.ID)
	if err != nil || updated == nil {
		t.Fatalf("UserByID: user = %v, err = %v", updated, err)
	}
	if !auth.CheckPassword(updated.PasswordHash, newPassword) {
		t.Error("reset did not set the new password")
	}
	if auth.CheckPassword(updated.PasswordHash, testPassword) {
		t.Error("old password still matches after reset")
	}

	response = doRequest(t, server, http.MethodGet, "/", nil, bobCookie, "")
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login" {
		t.Errorf("session survived password reset: status = %d, Location = %q", response.Code, response.Header().Get("Location"))
	}

	if response := postForm(t, server, "/reset/"+token, url.Values{
		"password":         {"againpassword123"},
		"confirm_password": {"againpassword123"},
	}, nil); response.Code != http.StatusNotFound {
		t.Errorf("reusing reset link status = %d, want %d", response.Code, http.StatusNotFound)
	}

	expiredAt := server.now().Add(-db.InviteLifetime - time.Hour)
	if err := server.db.CreatePasswordReset(context.Background(), "expired-reset", bob.ID, expiredAt); err != nil {
		t.Fatalf("create expired reset: %v", err)
	}
	if response := doRequest(t, server, http.MethodGet, "/reset/expired-reset", nil, nil, ""); response.Code != http.StatusNotFound {
		t.Errorf("expired reset page status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func createInvitedTestUser(t *testing.T, server *Server, username string, isAdmin bool, inviter *db.User) *db.User {
	t.Helper()

	user, err := server.db.CreateUser(context.Background(), username, testPasswordHash, isAdmin, &inviter.ID, server.now())
	if err != nil {
		t.Fatalf("create invited user %q: %v", username, err)
	}
	return user
}

var resetTokenPattern = regexp.MustCompile(`/reset/([A-Za-z0-9_-]+)`)

func resetTokenFromPage(t *testing.T, body string) string {
	t.Helper()

	match := resetTokenPattern.FindStringSubmatch(body)
	if match == nil {
		t.Fatalf("page does not contain a reset link: %q", body)
	}
	return match[1]
}
