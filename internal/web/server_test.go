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
	for _, want := range []string{"<h1>Soliloquy</h1>", `class="site-header"`, "/static/style.css", "/static/app.js", "alice", "My invites"} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
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
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			response := get(t, test.path)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
			}
			if contentType := response.Header().Get("Content-Type"); !strings.Contains(contentType, test.contentType) {
				t.Errorf("Content-Type = %q, want %q", contentType, test.contentType)
			}
			if test.contains != "" && !strings.Contains(response.Body.String(), test.contains) {
				t.Errorf("body does not contain %q", test.contains)
			}
		})
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
