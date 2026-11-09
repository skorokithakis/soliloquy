package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestAccountPageShowsDetails(t *testing.T) {
	server := newTestServer(t)
	admin := createTestUser(t, server, "admin", true)
	bob := createInvitedTestUser(t, server, "bob", false, admin)
	createInvitedTestUser(t, server, "carol", false, bob)
	cookie := signIn(t, server, bob)

	body := doRequest(t, server, http.MethodGet, "/account", nil, cookie, "").Body.String()
	for _, want := range []string{
		"whoami",
		"bob",
		"user",
		bob.CreatedAt.Format("2006-01-02"),
		"invited by",
		"admin",
		"1 person",
		"see the tree",
		"this browser",
		`action="/account/theme"`,
		`action="/password"`,
		`action="/account/delete"`,
		`data-confirm-username="bob"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("account page does not contain %q", want)
		}
	}
}

func TestAccountThemeSetting(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)

	response := postForm(t, server, "/account/theme", url.Values{"theme": {"dark"}}, cookie)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/account" {
		t.Fatalf("theme status = %d, Location = %q", response.Code, response.Header().Get("Location"))
	}

	stored, err := server.db.UserByID(context.Background(), alice.ID)
	if err != nil || stored == nil || stored.Theme != "dark" {
		t.Fatalf("stored theme = %+v, err = %v", stored, err)
	}

	for _, path := range []string{"/account", "/"} {
		body := doRequest(t, server, http.MethodGet, path, nil, cookie, "").Body.String()
		if !strings.Contains(body, `<html lang="en" class="sq dark">`) {
			t.Errorf("%s does not use the dark theme class", path)
		}
	}
	body := doRequest(t, server, http.MethodGet, "/account", nil, cookie, "").Body.String()
	if !strings.Contains(body, `value="dark" checked`) {
		t.Error("account page does not mark dark as the selected theme")
	}

	response = postForm(t, server, "/account/theme", url.Values{"theme": {"sepia"}}, cookie)
	if response.Code != http.StatusBadRequest {
		t.Errorf("invalid theme status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	stored, err = server.db.UserByID(context.Background(), alice.ID)
	if err != nil || stored == nil || stored.Theme != "dark" {
		t.Errorf("invalid theme changed the stored value: %+v, %v", stored, err)
	}
}

func TestPasswordPageRedirectsToAccount(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)

	response := doRequest(t, server, http.MethodGet, "/password", nil, cookie, "")
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/account" {
		t.Errorf("status = %d, Location = %q; want redirect to /account", response.Code, response.Header().Get("Location"))
	}
}
