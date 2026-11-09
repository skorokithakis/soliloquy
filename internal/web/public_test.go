package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/skorokithakis/soliloquy/internal/db"
)

func setPublic(t *testing.T, server *Server, public bool) {
	t.Helper()

	if err := server.db.SetSettings(context.Background(), db.Settings{
		Public:    public,
		ForumName: db.DefaultForumName,
	}); err != nil {
		t.Fatalf("set settings: %v", err)
	}
}

func TestVisitorReadsNothingWhenForumIsPrivate(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	comment := createCommentDirect(t, server, thread, alice, nil, "comment", server.now().Add(time.Minute))

	gets := []string{
		"/",
		threadPath(thread.PublicID),
		"/search?q=Thread",
		"/threads/new",
		"/invites",
		"/tree",
		"/notifications",
		"/account",
		"/admin/users",
		"/admin/settings",
		threadPath(thread.PublicID) + "/edit",
		commentAction(comment.PublicID, "reply"),
	}
	for _, path := range gets {
		response := doRequest(t, server, http.MethodGet, path, nil, nil, "")
		if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login" {
			t.Errorf("%s: status = %d, Location = %q, want a redirect to /login", path, response.Code, response.Header().Get("Location"))
		}
	}

	posts := []struct {
		path string
		form url.Values
	}{
		{"/threads", url.Values{"title": {"Hijacked"}, "body": {"nope"}}},
		{"/preview", url.Values{"body": {"nope"}}},
		{threadPath(thread.PublicID) + "/vote", nil},
		{threadPath(thread.PublicID) + "/comments", url.Values{"body": {"nope"}}},
		{commentAction(comment.PublicID, "vote"), nil},
		{"/admin/settings", url.Values{"public": {"on"}, "forum_name": {"Hijacked"}}},
	}
	for _, post := range posts {
		response := postForm(t, server, post.path, post.form, nil)
		if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login" {
			t.Errorf("POST %s: status = %d, Location = %q, want a redirect to /login", post.path, response.Code, response.Header().Get("Location"))
		}
	}

	threadCount, err := server.db.CountThreads(context.Background())
	if err != nil {
		t.Fatalf("CountThreads: %v", err)
	}
	if threadCount != 1 {
		t.Errorf("thread count = %d, want 1; a visitor created a thread", threadCount)
	}
	storedThread, err := server.db.ThreadByID(context.Background(), thread.ID)
	if err != nil || storedThread == nil {
		t.Fatalf("ThreadByID: thread = %v, err = %v", storedThread, err)
	}
	if storedThread.Score != 1 {
		t.Errorf("thread score = %d, want 1; a visitor voted", storedThread.Score)
	}
	comments, err := server.db.ListComments(context.Background(), thread.ID, 0)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(comments) != 1 {
		t.Errorf("comment count = %d, want 1; a visitor commented", len(comments))
	}
	settings, err := server.db.Settings(context.Background())
	if err != nil {
		t.Fatalf("Settings: %v", err)
	}
	if settings.Public || settings.ForumName != db.DefaultForumName {
		t.Errorf("visitor changed settings: %+v", settings)
	}
}

func TestVisitorReadsPublicForumWithoutControls(t *testing.T) {
	server := newTestServer(t)
	setPublic(t, server, true)

	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	thread := createThreadDirect(t, server, alice, "Public thread", "", "the body", server.now())
	createCommentDirect(t, server, thread, alice, nil, "visible comment", server.now().Add(time.Minute))

	index := doRequest(t, server, http.MethodGet, "/", nil, nil, "")
	if index.Code != http.StatusOK {
		t.Fatalf("front page status = %d, want %d", index.Code, http.StatusOK)
	}
	indexBody := index.Body.String()
	for _, want := range []string{
		"Public thread",
		`<span class="sc">1</span>`,
		`class="top"`,
		`class="wrap"`,
		`aria-label="Search threads and comments"`,
		`<a class="me" href="/login">log in</a>`,
	} {
		if !strings.Contains(indexBody, want) {
			t.Errorf("front page does not contain %q", want)
		}
	}
	for _, unwanted := range []string{
		"data-sse-url",
		`href="/threads/new"`,
		`href="/notifications"`,
		`href="/account"`,
		`href="/invites"`,
		`class="vote-form"`,
		`aria-label="Upvote"`,
	} {
		if strings.Contains(indexBody, unwanted) {
			t.Errorf("front page contains visitor-forbidden control %q", unwanted)
		}
	}

	page := doRequest(t, server, http.MethodGet, threadPath(thread.PublicID), nil, nil, "")
	if page.Code != http.StatusOK {
		t.Fatalf("thread page status = %d, want %d", page.Code, http.StatusOK)
	}
	pageBody := page.Body.String()
	for _, want := range []string{
		"Public thread",
		"the body",
		"visible comment",
		`<span class="score">1 point</span>`,
		`<a class="btn" href="/login">Log in to comment</a>`,
	} {
		if !strings.Contains(pageBody, want) {
			t.Errorf("thread page does not contain %q", want)
		}
	}
	for _, unwanted := range []string{
		"data-sse-url",
		`id="comment-form"`,
		`action="` + threadPath(thread.PublicID) + `/comments"`,
		`name="body"`,
		`class="vote-form"`,
		`aria-label="Upvote"`,
		`class="reply-link"`,
		`href="` + threadPath(thread.PublicID) + `/edit"`,
		`action="` + threadPath(thread.PublicID) + `/delete"`,
		`href="/account"`,
	} {
		if strings.Contains(pageBody, unwanted) {
			t.Errorf("thread page contains visitor-forbidden control %q", unwanted)
		}
	}

	search := doRequest(t, server, http.MethodGet, "/search?q=Public", nil, nil, "")
	if search.Code != http.StatusOK {
		t.Fatalf("search status = %d, want %d", search.Code, http.StatusOK)
	}
	if !strings.Contains(search.Body.String(), "Public thread") {
		t.Error("search results do not include the public thread")
	}
	if strings.Contains(search.Body.String(), "data-sse-url") {
		t.Error("search page opens a live-update stream for a visitor")
	}

	paged := doRequest(t, server, http.MethodGet, "/?page=2", nil, nil, "")
	if paged.Code != http.StatusOK {
		t.Errorf("paged front page status = %d, want %d", paged.Code, http.StatusOK)
	}

	// The login page keeps the bare layout even while the forum is public.
	login := doRequest(t, server, http.MethodGet, "/login", nil, nil, "")
	for _, unwanted := range []string{`class="top"`, `class="wrap"`} {
		if strings.Contains(login.Body.String(), unwanted) {
			t.Errorf("login page contains %q instead of the bare layout", unwanted)
		}
	}
	if !strings.Contains(login.Body.String(), `class="bare"`) {
		t.Error("login page does not use the bare layout")
	}

	// A logged-in member still gets the normal controls.
	cookie := signIn(t, server, bob)
	member := doRequest(t, server, http.MethodGet, threadPath(thread.PublicID), nil, cookie, "")
	for _, want := range []string{
		`id="comment-form"`,
		`class="vote-form"`,
		`href="/threads/new"`,
	} {
		if !strings.Contains(member.Body.String(), want) {
			t.Errorf("member thread page does not contain %q", want)
		}
	}
}

func TestVisitorCannotUsePrivateRoutesWhenPublic(t *testing.T) {
	server := newTestServer(t)
	setPublic(t, server, true)

	alice := createTestUser(t, server, "alice", false)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	comment := createCommentDirect(t, server, thread, alice, nil, "comment", server.now())

	gets := []string{
		"/threads/new",
		"/invites",
		"/tree",
		"/notifications",
		"/account",
		"/admin/users",
		"/admin/settings",
		"/events",
		"/password",
		threadPath(thread.PublicID) + "/edit",
		commentAction(comment.PublicID, "reply"),
	}
	for _, path := range gets {
		response := doRequest(t, server, http.MethodGet, path, nil, nil, "")
		if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login" {
			t.Errorf("%s: status = %d, Location = %q, want a redirect to /login", path, response.Code, response.Header().Get("Location"))
		}
	}

	posts := []struct {
		path string
		form url.Values
	}{
		{"/threads", url.Values{"title": {"Hijacked"}, "body": {"nope"}}},
		{"/preview", url.Values{"body": {"nope"}}},
		{threadPath(thread.PublicID) + "/vote", nil},
		{threadPath(thread.PublicID) + "/comments", url.Values{"body": {"nope"}}},
		{commentAction(comment.PublicID, "vote"), nil},
		{commentAction(comment.PublicID, "delete"), nil},
		{"/admin/settings", url.Values{"public": {"off"}, "forum_name": {"Hijacked"}}},
	}
	for _, post := range posts {
		response := postForm(t, server, post.path, post.form, nil)
		if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login" {
			t.Errorf("POST %s: status = %d, Location = %q, want a redirect to /login", post.path, response.Code, response.Header().Get("Location"))
		}
	}

	threadCount, err := server.db.CountThreads(context.Background())
	if err != nil {
		t.Fatalf("CountThreads: %v", err)
	}
	if threadCount != 1 {
		t.Errorf("thread count = %d, want 1; a visitor created a thread", threadCount)
	}
	storedThread, err := server.db.ThreadByID(context.Background(), thread.ID)
	if err != nil || storedThread == nil {
		t.Fatalf("ThreadByID: thread = %v, err = %v", storedThread, err)
	}
	if storedThread.Score != 1 {
		t.Errorf("thread score = %d, want 1; a visitor voted", storedThread.Score)
	}
	comments, err := server.db.ListComments(context.Background(), thread.ID, 0)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(comments) != 1 {
		t.Errorf("comment count = %d, want 1; a visitor commented or deleted", len(comments))
	}
	settings, err := server.db.Settings(context.Background())
	if err != nil {
		t.Fatalf("Settings: %v", err)
	}
	if !settings.Public || settings.ForumName != db.DefaultForumName {
		t.Errorf("visitor changed settings: %+v", settings)
	}
}

// TestPublicModeAllowsOnlyThreadPaths guards the thread path check, so a
// visitor cannot reach /threads/new through the public read-only rule, and old
// numeric thread URLs still get a 404.
func TestPublicModeAllowsOnlyThreadPaths(t *testing.T) {
	server := newTestServer(t)
	setPublic(t, server, true)

	alice := createTestUser(t, server, "alice", false)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())

	for _, path := range []string{"/", "/search", threadPath(thread.PublicID)} {
		response := doRequest(t, server, http.MethodGet, path, nil, nil, "")
		if response.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want %d", path, response.Code, http.StatusOK)
		}
	}

	for _, path := range []string{
		"/threads/new",
		"/threads/",
		"/threads/" + thread.PublicID + "/",
		"/threads/" + thread.PublicID + "/edit",
	} {
		response := doRequest(t, server, http.MethodGet, path, nil, nil, "")
		if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login" {
			t.Errorf("%s: status = %d, Location = %q, want a redirect to /login", path, response.Code, response.Header().Get("Location"))
		}
	}

	for _, path := range []string{
		"/threads/abc",
		"/threads/0",
		"/threads/-1",
		"/threads/999",
	} {
		response := doRequest(t, server, http.MethodGet, path, nil, nil, "")
		if response.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want %d", path, response.Code, http.StatusNotFound)
		}
	}
}
