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
		`data-desktop-alerts`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("account page does not contain %q", want)
		}
	}
}

func TestAPIDocsPageRequiresLogin(t *testing.T) {
	server := newTestServer(t)
	setPublic(t, server, true)

	response := doRequest(t, server, http.MethodGet, "/account/api", nil, nil, "")
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login" {
		t.Errorf("status = %d, Location = %q; want a login redirect", response.Code, response.Header().Get("Location"))
	}
}

func TestAPIDocsPageShowsTokenizedURLs(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)

	body := doRequest(t, server, http.MethodGet, "/account/api", nil, cookie, "").Body.String()

	stored, err := server.db.UserByID(context.Background(), alice.ID)
	if err != nil || stored == nil {
		t.Fatalf("load user: %v", err)
	}
	token := stored.FeedToken
	if token == "" {
		t.Fatal("viewing the API page did not create a feed token")
	}

	for _, want := range []string{
		testBaseURL + "/api/threads?token=" + token,
		testBaseURL + "/api/threads/{id}?token=" + token,
		"GET /api/threads",
		"has_next",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("API page does not contain %q", want)
		}
	}

	// The pattern is documentation, not a second link.
	if strings.Contains(body, `href="`+testBaseURL+`/api/threads/{id}?token=`+token+`"`) {
		t.Error("API page links to the thread ID pattern")
	}
}

func TestAccountPageLinksToAPIDocs(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)

	body := doRequest(t, server, http.MethodGet, "/account", nil, cookie, "").Body.String()
	if !strings.Contains(body, `href="/account/api"`) || !strings.Contains(body, "API documentation") {
		t.Error("account page does not link to the API documentation")
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
