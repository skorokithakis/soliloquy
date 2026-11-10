package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// newTitleTestPage starts a page whose title the handler can fetch.
func newTitleTestPage(t *testing.T) *httptest.Server {
	t.Helper()

	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, `<html><head><meta property="og:title" content="Open &amp; Graph"></head><body></body></html>`)
	}))
	t.Cleanup(page.Close)
	return page
}

func TestTitleFetchesPageTitle(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)
	// The page is an httptest server on 127.0.0.1, which the guard would
	// otherwise refuse.
	server.titleFetcher.AllowPrivateAddresses = true
	page := newTitleTestPage(t)

	response := postForm(t, server, "/title", url.Values{"url": {page.URL}}, cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}

	var result titleResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v (body %q)", err, response.Body.String())
	}
	if result.Title != "Open & Graph" {
		t.Errorf("title = %q, want %q", result.Title, "Open & Graph")
	}
}

func TestTitleReturnsEmptyTitleOnFailure(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)
	page := newTitleTestPage(t)

	tests := []struct {
		name string
		url  string
	}{
		{name: "missing URL", url: ""},
		{name: "invalid URL", url: "javascript:alert(1)"},
		{name: "unsupported scheme", url: "ftp://example.com/file"},
		{name: "private address", url: page.URL},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := postForm(t, server, "/title", url.Values{"url": {test.url}}, cookie)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
			}

			var result titleResponse
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatalf("decode response: %v (body %q)", err, response.Body.String())
			}
			if result.Title != "" {
				t.Errorf("title = %q, want empty", result.Title)
			}
		})
	}
}

func TestTitleRequiresLogin(t *testing.T) {
	server := newTestServer(t)

	response := doRequest(t, server, http.MethodPost, "/title", url.Values{"url": {"https://example.com"}}, nil, testBaseURL)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	if location := response.Header().Get("Location"); location != "/login" {
		t.Errorf("Location = %q, want /login", location)
	}
}

func TestTitleRejectsWrongOrigin(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)
	page := newTitleTestPage(t)
	server.titleFetcher.AllowPrivateAddresses = true

	response := doRequest(t, server, http.MethodPost, "/title", url.Values{"url": {page.URL}}, cookie, "http://evil.example")
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}
