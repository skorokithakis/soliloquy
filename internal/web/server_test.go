package web

import (
	"net/http"
	"strings"
	"testing"
)

func TestSignedOutRedirectsToLogin(t *testing.T) {
	response := get(t, "/")

	if response.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	if location := response.Header().Get("Location"); location != "/login" {
		t.Fatalf("Location = %q, want /login", location)
	}
}

func TestLoginPage(t *testing.T) {
	response := get(t, "/login")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", contentType)
	}
	for _, want := range []string{`action="/login"`, `name="password"`, "Log in"} {
		if !strings.Contains(response.Body.String(), want) {
			t.Errorf("body does not contain %q", want)
		}
	}
}

func TestIndexPage(t *testing.T) {
	server := newTestServer(t)
	user := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, user)

	response := doRequest(t, server, http.MethodGet, "/", nil, cookie, "")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", contentType)
	}

	body := response.Body.String()
	if strings.Contains(body, `class="pagination"`) {
		t.Error("index shows the pagination bar when there is only one page")
	}
	for _, want := range []string{
		`<h1 class="pt"><span class="p">~/</span>threads</h1>`,
		`class="sq auto"`,
		`class="top"`,
		`href="/invites">invites</a>`,
		staticURL(t, "style.css"),
		staticURL(t, "app.js"),
		"alice",
		`href="/account"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
	}
}

func TestAdminNavOnlyForAdmins(t *testing.T) {
	server := newTestServer(t)
	user := createTestUser(t, server, "alice", false)
	admin := createTestUser(t, server, "bob", true)

	userBody := doRequest(t, server, http.MethodGet, "/", nil, signIn(t, server, user), "").Body.String()
	if strings.Contains(userBody, `href="/admin/users"`) {
		t.Error("normal user sees the admin nav item")
	}

	adminBody := doRequest(t, server, http.MethodGet, "/", nil, signIn(t, server, admin), "").Body.String()
	if !strings.Contains(adminBody, `href="/admin/users">admin</a>`) {
		t.Error("admin does not see the admin nav item")
	}
}

func TestLayoutShowsThemeFooterAndCurrentNav(t *testing.T) {
	server := newTestServer(t)
	user := createTestUser(t, server, "alice", false)

	body := doRequest(t, server, http.MethodGet, "/", nil, signIn(t, server, user), "").Body.String()
	for _, want := range []string{
		`<html lang="en" class="sq auto">`,
		`<a class="on" aria-current="page" href="/">threads</a>`,
		`<a class="ib notifications-link" href="/notifications" aria-label="Notifications">`,
		`<a class="me" href="/account">alice</a>`,
		`<span>powered by <a href="https://github.com/skorokithakis/soliloquy">soliloquy</a></span>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("layout does not contain %q", want)
		}
	}

	invitesBody := doRequest(t, server, http.MethodGet, "/invites", nil, signIn(t, server, user), "").Body.String()
	if !strings.Contains(invitesBody, `class="on" aria-current="page" href="/invites">invites</a>`) {
		t.Error("invites navigation item is not marked current")
	}
}

func TestLoggedOutPagesUseBareLayout(t *testing.T) {
	body := get(t, "/login").Body.String()

	if !strings.Contains(body, `class="sq auto"`) {
		t.Error("logged-out page does not follow the system theme")
	}
	if !strings.Contains(body, `class="bare"`) {
		t.Error("logged-out page does not use the bare layout")
	}
	if !strings.Contains(body, `class="logo"`) {
		t.Error("logged-out page does not show the logo")
	}
	if strings.Contains(body, `class="top"`) {
		t.Error("logged-out page shows the site header")
	}
	if strings.Contains(body, `class="foot"`) {
		t.Error("logged-out page shows the footer")
	}
}

func TestStaticAssets(t *testing.T) {
	tests := []struct {
		path        string
		contentType string
		contains    string
	}{
		{path: "/static/style.css", contentType: "text/css", contains: "prefers-color-scheme"},
		{path: "/static/app.js", contentType: "javascript"},
		{path: "/static/fonts/ibm-plex-mono-latin-400.woff2"},
		{path: "/static/fonts/OFL.txt", contentType: "text/plain", contains: "SIL Open Font License"},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			response := get(t, test.path)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
			}
			if contentType := response.Header().Get("Content-Type"); test.contentType != "" && !strings.Contains(contentType, test.contentType) {
				t.Errorf("Content-Type = %q, want %q", contentType, test.contentType)
			}
			if test.contains != "" && !strings.Contains(response.Body.String(), test.contains) {
				t.Errorf("body does not contain %q", test.contains)
			}
		})
	}
}

func TestStylesheetDoesNotLoadRemoteFonts(t *testing.T) {
	css := get(t, "/static/style.css").Body.String()

	for _, host := range []string{"fonts.googleapis.com", "fonts.gstatic.com"} {
		if strings.Contains(css, host) {
			t.Errorf("stylesheet requests fonts from %s", host)
		}
	}
	for _, path := range []string{
		"/static/fonts/ibm-plex-sans-latin-400.woff2",
		"/static/fonts/ibm-plex-sans-latin-500.woff2",
		"/static/fonts/ibm-plex-sans-latin-600.woff2",
		"/static/fonts/ibm-plex-mono-latin-400.woff2",
		"/static/fonts/ibm-plex-mono-latin-500.woff2",
		"/static/fonts/ibm-plex-mono-latin-600.woff2",
	} {
		if !strings.Contains(css, path) {
			t.Errorf("stylesheet does not reference %s", path)
		}
	}
}

func TestUnknownPathReturnsNotFound(t *testing.T) {
	server := newTestServer(t)
	user := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, user)

	response := doRequest(t, server, http.MethodGet, "/does-not-exist", nil, cookie, "")
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestIndexRejectsPost(t *testing.T) {
	server := newTestServer(t)
	user := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, user)

	response := doRequest(t, server, http.MethodPost, "/", nil, cookie, testBaseURL)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}
