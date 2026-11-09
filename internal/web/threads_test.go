package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/skorokithakis/soliloquy/internal/db"
	"github.com/skorokithakis/soliloquy/internal/markdown"
)

func TestCreateThreadWithURL(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)

	response := postForm(t, server, "/threads", url.Values{
		"title": {"A link"},
		"url":   {"https://example.com/path"},
	}, cookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("create status = %d, want %d", response.Code, http.StatusSeeOther)
	}

	location := response.Header().Get("Location")
	if !strings.HasPrefix(location, "/threads/") {
		t.Fatalf("Location = %q, want a thread path", location)
	}

	page := doRequest(t, server, http.MethodGet, location, nil, cookie, "")
	if page.Code != http.StatusOK {
		t.Fatalf("thread page status = %d, want %d", page.Code, http.StatusOK)
	}
	for _, want := range []string{`href="https://example.com/path"`, "example.com", "alice", "1 point"} {
		if !strings.Contains(page.Body.String(), want) {
			t.Errorf("thread page does not contain %q", want)
		}
	}

	index := doRequest(t, server, http.MethodGet, "/", nil, cookie, "")
	body := index.Body.String()
	for _, want := range []string{"A link", `href="https://example.com/path"`, "example.com", "alice", "1 point", "0 comments"} {
		if !strings.Contains(body, want) {
			t.Errorf("front page does not contain %q", want)
		}
	}
}

func TestCreateThreadRendersAndStoresBody(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)

	body := "**bold** text\n\n<script>alert('x')</script>"
	response := postForm(t, server, "/threads", url.Values{
		"title": {"Body thread"},
		"body":  {body},
	}, cookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("create status = %d, want %d", response.Code, http.StatusSeeOther)
	}

	threads, err := server.db.ListThreads(context.Background(), 10, 0)
	if err != nil || len(threads) != 1 {
		t.Fatalf("ListThreads: threads = %v, err = %v", threads, err)
	}
	thread := threads[0]
	if thread.Body != body {
		t.Errorf("stored body = %q, want source", thread.Body)
	}
	if !strings.Contains(thread.BodyHTML, "<strong>bold</strong>") {
		t.Errorf("stored HTML = %q, want rendered Markdown", thread.BodyHTML)
	}
	if strings.Contains(thread.BodyHTML, "<script") {
		t.Errorf("stored HTML is not sanitized: %q", thread.BodyHTML)
	}

	page := doRequest(t, server, http.MethodGet, threadPath(thread.ID), nil, cookie, "")
	if page.Code != http.StatusOK {
		t.Fatalf("thread page status = %d, want %d", page.Code, http.StatusOK)
	}
	if !strings.Contains(page.Body.String(), "<strong>bold</strong>") {
		t.Error("thread page did not render the Markdown body")
	}
	if strings.Contains(page.Body.String(), "alert('x')") {
		t.Error("thread page contains unsanitized script content")
	}

	index := doRequest(t, server, http.MethodGet, "/", nil, cookie, "")
	if !strings.Contains(index.Body.String(), `href="`+threadPath(thread.ID)+`"`) {
		t.Error("a thread without a URL should link to its thread page")
	}
}

func TestThreadValidation(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)

	tests := []struct {
		name  string
		title string
		url   string
		body  string
		want  string
	}{
		{name: "missing title", title: "", url: "https://example.com/", want: "Title is required"},
		{name: "long title", title: strings.Repeat("a", maxThreadTitleLength+1), body: "body", want: "200 characters"},
		{name: "javascript URL", title: "Thread", url: "javascript:alert(1)", want: "http:// or https://"},
		{name: "data URL", title: "Thread", url: "data:text/html,hello", want: "http:// or https://"},
		{name: "ftp URL", title: "Thread", url: "ftp://example.com/", want: "http:// or https://"},
		{name: "protocol-relative URL", title: "Thread", url: "//example.com/", want: "http:// or https://"},
		{name: "URL without host", title: "Thread", url: "https://", want: "http:// or https://"},
		{name: "no URL or body", title: "Thread", want: "URL, a body, or both"},
		{name: "whitespace-only body", title: "Thread", body: "  \n\t", want: "URL, a body, or both"},
		{name: "long body", title: "Thread", body: strings.Repeat("x", maxThreadBodyLength+1), want: "20000 characters"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			form := url.Values{"title": {test.title}}
			if test.url != "" {
				form.Set("url", test.url)
			}
			if test.body != "" {
				form.Set("body", test.body)
			}

			response := postForm(t, server, "/threads", form, cookie)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
			}
			if !strings.Contains(response.Body.String(), test.want) {
				t.Errorf("body does not contain %q: %q", test.want, response.Body.String())
			}
		})
	}

	var count int
	if err := server.db.QueryRow("SELECT COUNT(*) FROM threads").Scan(&count); err != nil {
		t.Fatalf("count threads: %v", err)
	}
	if count != 0 {
		t.Errorf("invalid submissions stored %d threads", count)
	}
}

func TestThreadTitleLengthCountsCharacters(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)

	response := postForm(t, server, "/threads", url.Values{
		"title": {strings.Repeat("é", maxThreadTitleLength)},
		"body":  {"body"},
	}, cookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("title at the limit: status = %d, want %d", response.Code, http.StatusSeeOther)
	}

	response = postForm(t, server, "/threads", url.Values{
		"title": {strings.Repeat("é", maxThreadTitleLength+1)},
		"body":  {"body"},
	}, cookie)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("title over the limit: status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestAuthorEditsThread(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)
	createdAt := server.now()
	thread := createThreadDirect(t, server, alice, "Original", "", "old body", createdAt)

	response := doRequest(t, server, http.MethodGet, threadPath(thread.ID)+"/edit", nil, cookie, "")
	if response.Code != http.StatusOK {
		t.Fatalf("edit page status = %d, want %d", response.Code, http.StatusOK)
	}
	for _, want := range []string{"Edit thread", "Original", "old body", `action="` + threadPath(thread.ID) + `/edit"`} {
		if !strings.Contains(response.Body.String(), want) {
			t.Errorf("edit page does not contain %q", want)
		}
	}

	response = postForm(t, server, threadPath(thread.ID)+"/edit", url.Values{
		"title": {"Edited"},
		"url":   {"https://example.com/"},
		"body":  {"new **body**"},
	}, cookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("edit status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	if location := response.Header().Get("Location"); location != threadPath(thread.ID) {
		t.Errorf("Location = %q, want %q", location, threadPath(thread.ID))
	}

	page := doRequest(t, server, http.MethodGet, threadPath(thread.ID), nil, cookie, "")
	if page.Code != http.StatusOK {
		t.Fatalf("thread page status = %d, want %d", page.Code, http.StatusOK)
	}
	for _, want := range []string{"Edited", "<strong>body</strong>", "edited", `href="https://example.com/"`} {
		if !strings.Contains(page.Body.String(), want) {
			t.Errorf("thread page does not contain %q", want)
		}
	}

	updated, err := server.db.ThreadByID(context.Background(), thread.ID)
	if err != nil || updated == nil {
		t.Fatalf("ThreadByID: thread = %v, err = %v", updated, err)
	}
	if updated.EditedAt == nil {
		t.Error("edited_at was not set")
	}
	if updated.LastActivityAt.Unix() != createdAt.Unix() {
		t.Errorf("LastActivityAt = %v, want editing not to change it (%v)", updated.LastActivityAt, createdAt)
	}
}

func TestThreadWritesRequireAuthor(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	bobCookie := signIn(t, server, bob)
	thread := createThreadDirect(t, server, alice, "Original", "", "body", server.now())

	response := doRequest(t, server, http.MethodGet, threadPath(thread.ID)+"/edit", nil, bobCookie, "")
	if response.Code != http.StatusForbidden {
		t.Errorf("edit page as another user: status = %d, want %d", response.Code, http.StatusForbidden)
	}

	response = postForm(t, server, threadPath(thread.ID)+"/edit", url.Values{
		"title": {"Hijacked"},
		"body":  {"changed"},
	}, bobCookie)
	if response.Code != http.StatusForbidden {
		t.Errorf("edit as another user: status = %d, want %d", response.Code, http.StatusForbidden)
	}

	response = postForm(t, server, threadPath(thread.ID)+"/delete", nil, bobCookie)
	if response.Code != http.StatusForbidden {
		t.Errorf("delete as another user: status = %d, want %d", response.Code, http.StatusForbidden)
	}

	stored, err := server.db.ThreadByID(context.Background(), thread.ID)
	if err != nil || stored == nil {
		t.Fatalf("ThreadByID: thread = %v, err = %v", stored, err)
	}
	if stored.Title != "Original" || stored.Body != "body" || stored.DeletedAt != nil {
		t.Errorf("another user changed the thread: %+v", stored)
	}

	response = doRequest(t, server, http.MethodGet, threadPath(thread.ID), nil, bobCookie, "")
	if response.Code != http.StatusOK {
		t.Errorf("signed-in user cannot view a thread: status = %d", response.Code)
	}
}

func TestAuthorDeletesThread(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)
	thread := createThreadDirect(t, server, alice, "Delete me", "", "body", server.now())

	response := postForm(t, server, threadPath(thread.ID)+"/delete", nil, cookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("delete status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	if location := response.Header().Get("Location"); location != "/" {
		t.Errorf("Location = %q, want /", location)
	}

	index := doRequest(t, server, http.MethodGet, "/", nil, cookie, "")
	if strings.Contains(index.Body.String(), "Delete me") {
		t.Error("deleted thread still shows on the front page")
	}

	page := doRequest(t, server, http.MethodGet, threadPath(thread.ID), nil, cookie, "")
	if page.Code != http.StatusNotFound {
		t.Errorf("deleted thread page status = %d, want %d", page.Code, http.StatusNotFound)
	}

	stored, err := server.db.ThreadByID(context.Background(), thread.ID)
	if err != nil || stored == nil {
		t.Fatalf("ThreadByID: thread = %v, err = %v", stored, err)
	}
	if stored.DeletedAt == nil {
		t.Error("delete did not set deleted_at; the row should stay")
	}
}

func TestFrontPagePagination(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)
	base := time.Unix(1_700_000_000, 0).UTC()

	for i := 0; i < threadsPerPage+1; i++ {
		createThreadDirect(t, server, alice, fmt.Sprintf("Thread %02d", i), "", "body", base.Add(time.Duration(i)*time.Minute))
	}

	first := doRequest(t, server, http.MethodGet, "/", nil, cookie, "")
	if first.Code != http.StatusOK {
		t.Fatalf("first page status = %d", first.Code)
	}
	body := first.Body.String()
	if count := strings.Count(body, `class="thread-row"`); count != threadsPerPage {
		t.Errorf("first page rows = %d, want %d", count, threadsPerPage)
	}
	if !strings.Contains(body, "Thread 30") || strings.Contains(body, "Thread 00") {
		t.Error("first page does not show the newest threads only")
	}
	if !strings.Contains(body, `href="/?page=2"`) {
		t.Error("first page has no older-page link")
	}

	second := doRequest(t, server, http.MethodGet, "/?page=2", nil, cookie, "")
	if second.Code != http.StatusOK {
		t.Fatalf("second page status = %d", second.Code)
	}
	body = second.Body.String()
	if count := strings.Count(body, `class="thread-row"`); count != 1 {
		t.Errorf("second page rows = %d, want 1", count)
	}
	if !strings.Contains(body, "Thread 00") || strings.Contains(body, "Thread 30") {
		t.Error("second page does not show the oldest thread only")
	}
	if !strings.Contains(body, `href="/?page=1"`) {
		t.Error("second page has no newer-page link")
	}
}

func TestFrontPageShowsDeletedUser(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	cookie := signIn(t, server, bob)
	createThreadDirect(t, server, alice, "By alice", "", "body", server.now())

	if _, err := server.db.Exec(
		"UPDATE users SET deleted_at = ? WHERE id = ?", server.now().Unix(), alice.ID,
	); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	response := doRequest(t, server, http.MethodGet, "/", nil, cookie, "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if !strings.Contains(response.Body.String(), "[deleted user]") {
		t.Error("front page does not show [deleted user] for a deleted author")
	}
}

func TestThreadNotFound(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)

	for _, path := range []string{"/threads/999", "/threads/0", "/threads/not-a-number"} {
		response := doRequest(t, server, http.MethodGet, path, nil, cookie, "")
		if response.Code != http.StatusNotFound {
			t.Errorf("%s status = %d, want %d", path, response.Code, http.StatusNotFound)
		}
	}
}

func TestRelativeTime(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()

	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{name: "future", t: now.Add(time.Minute), want: "just now"},
		{name: "seconds", t: now.Add(-30 * time.Second), want: "just now"},
		{name: "one minute", t: now.Add(-time.Minute), want: "1 minute ago"},
		{name: "minutes", t: now.Add(-5 * time.Minute), want: "5 minutes ago"},
		{name: "one hour", t: now.Add(-time.Hour), want: "1 hour ago"},
		{name: "hours", t: now.Add(-3 * time.Hour), want: "3 hours ago"},
		{name: "one day", t: now.Add(-24 * time.Hour), want: "1 day ago"},
		{name: "days", t: now.Add(-5 * 24 * time.Hour), want: "5 days ago"},
		{name: "one month", t: now.Add(-30 * 24 * time.Hour), want: "1 month ago"},
		{name: "months", t: now.Add(-60 * 24 * time.Hour), want: "2 months ago"},
		{name: "one year", t: now.Add(-365 * 24 * time.Hour), want: "1 year ago"},
		{name: "years", t: now.Add(-3 * 365 * 24 * time.Hour), want: "3 years ago"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := relativeTime(test.t, now); got != test.want {
				t.Errorf("relativeTime = %q, want %q", got, test.want)
			}
		})
	}
}

func createThreadDirect(t *testing.T, server *Server, user *db.User, title, rawURL, body string, at time.Time) *db.Thread {
	t.Helper()

	bodyHTML := ""
	if body != "" {
		rendered, err := markdown.Render(body)
		if err != nil {
			t.Fatalf("render body: %v", err)
		}
		bodyHTML = string(rendered)
	}

	thread, err := server.db.CreateThread(context.Background(), user.ID, title, rawURL, body, bodyHTML, at)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	return thread
}
