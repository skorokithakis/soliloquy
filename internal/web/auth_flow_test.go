package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/skorokithakis/soliloquy/internal/auth"
)

func TestSignupInviteAndPasswordFlow(t *testing.T) {
	server := newTestServer(t)
	ctx := context.Background()

	bootstrapToken, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate bootstrap token: %v", err)
	}
	created, err := server.db.EnsureBootstrapInvite(ctx, bootstrapToken, server.now())
	if err != nil || !created {
		t.Fatalf("EnsureBootstrapInvite: created = %v, err = %v", created, err)
	}

	response := doRequest(t, server, http.MethodGet, "/invite/"+bootstrapToken, nil, nil, "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Sign up") {
		t.Fatalf("sign-up page: status = %d, body = %q", response.Code, response.Body.String())
	}

	response = postForm(t, server, "/invite/"+bootstrapToken, url.Values{
		"username": {"Admin"},
		"password": {testPassword},
	}, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("admin sign-up status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	adminCookie := sessionCookie(t, response)

	admin, err := server.db.UserByUsername(ctx, "admin")
	if err != nil || admin == nil {
		t.Fatalf("load admin: user = %v, err = %v", admin, err)
	}
	if !admin.IsAdmin {
		t.Error("bootstrap invite did not create an admin")
	}
	if admin.InvitedBy != nil {
		t.Errorf("admin InvitedBy = %v, want nil", admin.InvitedBy)
	}

	response = doRequest(t, server, http.MethodGet, "/", nil, adminCookie, "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "admin") {
		t.Fatalf("signed-in home: status = %d, body = %q", response.Code, response.Body.String())
	}

	response = postForm(t, server, "/invites", nil, adminCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("create invite status = %d, want %d", response.Code, http.StatusSeeOther)
	}

	invites, err := server.db.ListActiveInvites(ctx, admin.ID, server.now())
	if err != nil {
		t.Fatalf("list invites: %v", err)
	}
	if len(invites) != 1 {
		t.Fatalf("active invites = %d, want 1", len(invites))
	}

	response = postForm(t, server, "/invite/"+invites[0].Token, url.Values{
		"username": {"bob"},
		"password": {testPassword},
	}, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("second sign-up status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	bobCookie := sessionCookie(t, response)

	bob, err := server.db.UserByUsername(ctx, "bob")
	if err != nil || bob == nil {
		t.Fatalf("load bob: user = %v, err = %v", bob, err)
	}
	if bob.IsAdmin {
		t.Error("second user should not be an admin")
	}
	if bob.InvitedBy == nil || *bob.InvitedBy != admin.ID {
		t.Errorf("bob InvitedBy = %v, want %d", bob.InvitedBy, admin.ID)
	}

	response = postForm(t, server, "/invite/"+invites[0].Token, url.Values{
		"username": {"carol"},
		"password": {testPassword},
	}, nil)
	if response.Code != http.StatusNotFound {
		t.Fatalf("reusing invite status = %d, want %d", response.Code, http.StatusNotFound)
	}

	otherCookie := signIn(t, server, bob)

	newPassword := "newpassword123"
	response = postForm(t, server, "/password", url.Values{
		"current_password": {testPassword},
		"new_password":     {newPassword},
		"confirm_password": {newPassword},
	}, bobCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("change password status = %d, want %d", response.Code, http.StatusSeeOther)
	}

	response = doRequest(t, server, http.MethodGet, "/", nil, otherCookie, "")
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login" {
		t.Errorf("other session after password change: status = %d, Location = %q", response.Code, response.Header().Get("Location"))
	}
	response = doRequest(t, server, http.MethodGet, "/", nil, bobCookie, "")
	if response.Code != http.StatusOK {
		t.Errorf("current session after password change: status = %d, want %d", response.Code, http.StatusOK)
	}

	bob, err = server.db.UserByUsername(ctx, "bob")
	if err != nil || bob == nil {
		t.Fatalf("reload bob: user = %v, err = %v", bob, err)
	}
	if !auth.CheckPassword(bob.PasswordHash, newPassword) {
		t.Error("new password was not stored")
	}
	if auth.CheckPassword(bob.PasswordHash, testPassword) {
		t.Error("old password still matches")
	}
}

func TestLoginAndLogout(t *testing.T) {
	server := newTestServer(t)
	createTestUser(t, server, "alice", false)

	response := postForm(t, server, "/login", url.Values{
		"username": {"ALICE"},
		"password": {testPassword},
	}, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("login status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	cookie := sessionCookie(t, response)

	response = doRequest(t, server, http.MethodGet, "/", nil, cookie, "")
	if response.Code != http.StatusOK {
		t.Fatalf("home after login: status = %d, want %d", response.Code, http.StatusOK)
	}

	response = postForm(t, server, "/logout", nil, cookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("logout status = %d, want %d", response.Code, http.StatusSeeOther)
	}

	response = doRequest(t, server, http.MethodGet, "/", nil, cookie, "")
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login" {
		t.Fatalf("home after logout: status = %d, Location = %q", response.Code, response.Header().Get("Location"))
	}
}

func TestDisabledUserCannotLogInOrUseSession(t *testing.T) {
	server := newTestServer(t)
	user := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, user)

	if _, err := server.db.Exec(
		"UPDATE users SET disabled_at = ? WHERE id = ?", server.now().Unix(), user.ID,
	); err != nil {
		t.Fatalf("disable user: %v", err)
	}

	response := doRequest(t, server, http.MethodGet, "/", nil, cookie, "")
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login" {
		t.Errorf("disabled session: status = %d, Location = %q", response.Code, response.Header().Get("Location"))
	}

	response = postForm(t, server, "/login", url.Values{
		"username": {"alice"},
		"password": {testPassword},
	}, nil)
	if response.Code != http.StatusUnauthorized {
		t.Errorf("disabled login status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestLoginThrottleRejectsDuringCooldown(t *testing.T) {
	server := newTestServer(t)
	createTestUser(t, server, "alice", false)

	response := postForm(t, server, "/login", url.Values{
		"username": {"alice"},
		"password": {"wrong"},
	}, nil)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("failed login status = %d, want %d", response.Code, http.StatusUnauthorized)
	}

	response = postForm(t, server, "/login", url.Values{
		"username": {"alice"},
		"password": {testPassword},
	}, nil)
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("throttled login status = %d, want %d", response.Code, http.StatusTooManyRequests)
	}
	if response.Header().Get("Retry-After") == "" {
		t.Error("throttled response has no Retry-After header")
	}
}

func TestSignupRejectsInvalidUsernameWithoutConsumingInvite(t *testing.T) {
	server := newTestServer(t)
	ctx := context.Background()

	token, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	created, err := server.db.EnsureBootstrapInvite(ctx, token, server.now())
	if err != nil || !created {
		t.Fatalf("EnsureBootstrapInvite: created = %v, err = %v", created, err)
	}

	response := postForm(t, server, "/invite/"+token, url.Values{
		"username": {"x"},
		"password": {testPassword},
	}, nil)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid username status = %d, want %d", response.Code, http.StatusBadRequest)
	}

	response = postForm(t, server, "/invite/"+token, url.Values{
		"username": {"admin"},
		"password": {testPassword},
	}, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("valid sign-up after rejected attempt status = %d, want %d", response.Code, http.StatusSeeOther)
	}
}
