package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/skorokithakis/soliloquy/internal/config"
)

// findCookie returns the named cookie from a response, or nil.
func findCookie(response *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

// TestDraftPostedCookie checks that every handler that posts a form marks its
// path in the draft_posted cookie, which app.js reads to clear a draft.
func TestDraftPostedCookie(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	comment := createCommentDirect(t, server, thread, alice, nil, "existing", server.now())

	posts := []struct {
		name string
		path string
		form url.Values
	}{
		{"create comment", threadPath(thread.PublicID) + "/comments", url.Values{"body": {"a comment"}}},
		{"create reply", commentAction(comment.PublicID, "reply"), url.Values{"body": {"a reply"}}},
		{"update comment", commentAction(comment.PublicID, "edit"), url.Values{"body": {"edited comment"}}},
		{"create thread", "/threads", url.Values{"title": {"New thread"}, "body": {"new body"}}},
		{"update thread", threadPath(thread.PublicID) + "/edit", url.Values{"title": {"Edited thread"}, "body": {"edited body"}}},
	}

	for _, post := range posts {
		t.Run(post.name, func(t *testing.T) {
			response := postForm(t, server, post.path, post.form, cookie)
			if response.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusSeeOther)
			}

			posted := findCookie(response, draftPostedCookieName)
			if posted == nil {
				t.Fatal("response did not set the draft_posted cookie")
			}
			if posted.Value != post.path {
				t.Errorf("cookie value = %q, want %q", posted.Value, post.path)
			}
			if posted.Path != "/" {
				t.Errorf("cookie Path = %q, want /", posted.Path)
			}
			if posted.MaxAge != 60 {
				t.Errorf("cookie MaxAge = %d, want 60", posted.MaxAge)
			}
			if posted.HttpOnly {
				t.Error("cookie is HttpOnly, so the script cannot read or delete it")
			}
			if posted.Secure {
				t.Error("cookie is Secure on an http base URL")
			}
			if posted.SameSite != http.SameSiteLaxMode {
				t.Errorf("cookie SameSite = %v, want Lax", posted.SameSite)
			}
		})
	}
}

// TestDraftPostedCookieAbsentOnValidationError checks that a rejected post
// leaves the draft alone, so its text is not cleared.
func TestDraftPostedCookieAbsentOnValidationError(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	comment := createCommentDirect(t, server, thread, alice, nil, "existing", server.now())

	posts := []struct {
		name string
		path string
		form url.Values
	}{
		{"create comment", threadPath(thread.PublicID) + "/comments", url.Values{"body": {""}}},
		{"create reply", commentAction(comment.PublicID, "reply"), url.Values{"body": {""}}},
		{"update comment", commentAction(comment.PublicID, "edit"), url.Values{"body": {""}}},
		{"create thread", "/threads", url.Values{"title": {""}}},
		{"update thread", threadPath(thread.PublicID) + "/edit", url.Values{"title": {""}}},
	}

	for _, post := range posts {
		t.Run(post.name, func(t *testing.T) {
			response := postForm(t, server, post.path, post.form, cookie)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
			}
			if posted := findCookie(response, draftPostedCookieName); posted != nil {
				t.Errorf("failed post set the draft_posted cookie to %q", posted.Value)
			}
		})
	}
}

// TestDraftPostedCookieSecure checks the Secure flag follows the base URL.
func TestDraftPostedCookieSecure(t *testing.T) {
	server, err := NewServer(newTestDatabase(t), config.Config{BaseURL: "https://forum.test"})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(server.waitBackground)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)

	response := doRequest(t, server, http.MethodPost, "/threads", url.Values{
		"title": {"Secure thread"},
		"body":  {"body"},
	}, cookie, "https://forum.test")
	if response.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	posted := findCookie(response, draftPostedCookieName)
	if posted == nil {
		t.Fatal("response did not set the draft_posted cookie")
	}
	if !posted.Secure {
		t.Error("cookie is not Secure on an https base URL")
	}
}

// TestLayoutExposesCurrentUser checks the body carries the username the script
// uses to keep each account's drafts separate.
func TestLayoutExposesCurrentUser(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)

	page := doRequest(t, server, http.MethodGet, "/", nil, cookie, "")
	if !strings.Contains(page.Body.String(), `data-user="alice"`) {
		t.Error("page for a logged-in user has no data-user attribute")
	}

	login := doRequest(t, server, http.MethodGet, "/login", nil, nil, "")
	if strings.Contains(login.Body.String(), "data-user=") {
		t.Error("page for a visitor has a data-user attribute")
	}
}
