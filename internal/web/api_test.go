package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/skorokithakis/soliloquy/internal/db"
)

func TestAPIThreadsPagesTheFrontPage(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)
	base := time.Unix(1_700_000_000, 0).UTC()

	var newest *db.Thread
	for i := 0; i < threadsPerPage+1; i++ {
		rawURL := ""
		if i == threadsPerPage {
			rawURL = "https://example.com/article"
		}
		thread := createThreadDirect(t, server, alice, fmt.Sprintf("Thread %d", i), rawURL, "body", base.Add(time.Duration(i)*time.Minute))
		newest = thread
	}

	response := doRequest(t, server, http.MethodGet, "/api/threads", nil, cookie, "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}

	var first apiThreadsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &first); err != nil {
		t.Fatalf("parse response: %v\n%s", err, response.Body.String())
	}
	if first.Page != 1 {
		t.Errorf("page = %d, want 1", first.Page)
	}
	if !first.HasNext {
		t.Error("has_next = false on the first page")
	}
	if len(first.Threads) != threadsPerPage {
		t.Fatalf("first page has %d threads, want %d", len(first.Threads), threadsPerPage)
	}

	thread := first.Threads[0]
	wantLink := testBaseURL + threadPath(newest.PublicID)
	if thread.ID != newest.PublicID {
		t.Errorf("first thread ID = %q, want the newest %q", thread.ID, newest.PublicID)
	}
	if thread.Title != newest.Title {
		t.Errorf("first thread title = %q, want %q", thread.Title, newest.Title)
	}
	if thread.URL != "https://example.com/article" {
		t.Errorf("first thread URL = %q", thread.URL)
	}
	if thread.Link != wantLink {
		t.Errorf("first thread link = %q, want %q", thread.Link, wantLink)
	}
	if thread.Author != "alice" {
		t.Errorf("first thread author = %q", thread.Author)
	}
	if thread.Score != 1 {
		t.Errorf("first thread score = %d, want 1", thread.Score)
	}
	if thread.CommentCount != 0 {
		t.Errorf("first thread comment_count = %d, want 0", thread.CommentCount)
	}
	if thread.CreatedAt != "2023-11-14T22:43:20Z" {
		t.Errorf("first thread created_at = %q, want RFC 3339 UTC", thread.CreatedAt)
	}
	if thread.LastActivityAt != "2023-11-14T22:43:20Z" {
		t.Errorf("first thread last_activity_at = %q, want RFC 3339 UTC", thread.LastActivityAt)
	}

	if last := first.Threads[len(first.Threads)-1]; last.URL != "" {
		t.Errorf("thread without a URL has url = %q, want an empty string", last.URL)
	}
	if last := first.Threads[len(first.Threads)-1]; last.Title != "Thread 1" {
		t.Errorf("last thread on the first page = %q, want Thread 1", last.Title)
	}

	response = doRequest(t, server, http.MethodGet, "/api/threads?page=2", nil, cookie, "")
	if response.Code != http.StatusOK {
		t.Fatalf("page 2 status = %d, want %d", response.Code, http.StatusOK)
	}
	var second apiThreadsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &second); err != nil {
		t.Fatalf("parse page 2: %v\n%s", err, response.Body.String())
	}
	if second.Page != 2 {
		t.Errorf("page = %d, want 2", second.Page)
	}
	if second.HasNext {
		t.Error("has_next = true on the last page")
	}
	if len(second.Threads) != 1 {
		t.Fatalf("second page has %d threads, want 1", len(second.Threads))
	}
	if second.Threads[0].Title != "Thread 0" {
		t.Errorf("second page thread = %q, want Thread 0", second.Threads[0].Title)
	}
}

func TestAPIThreadReturnsNestedCommentsWithDeletedPlaceholder(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	cookie := signIn(t, server, alice)
	base := time.Unix(1_700_000_000, 0).UTC()

	thread := createThreadDirect(t, server, alice, "API thread", "https://example.com/article", "the **body**", base)

	root := createCommentDirect(t, server, thread, alice, nil, "root comment", base.Add(time.Minute))
	if _, err := server.db.UpdateComment(context.Background(), root.ID, alice.ID, "root edited", "<p>root edited</p>", base.Add(2*time.Minute)); err != nil {
		t.Fatalf("edit comment: %v", err)
	}
	deletedParent := createCommentDirect(t, server, thread, bob, &root.ID, "gone", base.Add(3*time.Minute))
	reply := createCommentDirect(t, server, thread, alice, &deletedParent.ID, "reply **here**", base.Add(4*time.Minute))
	hidden := createCommentDirect(t, server, thread, bob, nil, "hidden", base.Add(5*time.Minute))
	for _, comment := range []*db.Comment{deletedParent, hidden} {
		if _, err := server.db.DeleteComment(context.Background(), comment.ID, base.Add(6*time.Minute)); err != nil {
			t.Fatalf("delete comment: %v", err)
		}
	}

	response := doRequest(t, server, http.MethodGet, "/api/threads/"+thread.PublicID, nil, cookie, "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}

	var payload apiThreadResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("parse response: %v\n%s", err, response.Body.String())
	}
	if payload.Thread.ID != thread.PublicID {
		t.Errorf("thread ID = %q, want %q", payload.Thread.ID, thread.PublicID)
	}
	if payload.Thread.Body != "the **body**" {
		t.Errorf("thread body = %q, want the Markdown source", payload.Thread.Body)
	}
	if !strings.Contains(payload.Thread.BodyHTML, "<strong>body</strong>") {
		t.Errorf("thread body_html = %q", payload.Thread.BodyHTML)
	}
	if payload.Thread.EditedAt != nil {
		t.Errorf("thread edited_at = %q, want null", *payload.Thread.EditedAt)
	}
	if payload.Thread.Link != testBaseURL+threadPath(thread.PublicID) {
		t.Errorf("thread link = %q", payload.Thread.Link)
	}

	if len(payload.Comments) != 1 {
		t.Fatalf("comments = %d, want 1 root", len(payload.Comments))
	}
	if payload.Comments[0].ID != root.PublicID {
		t.Errorf("root comment ID = %q, want %q", payload.Comments[0].ID, root.PublicID)
	}
	if payload.Comments[0].Body != "root edited" {
		t.Errorf("root body = %q", payload.Comments[0].Body)
	}
	if payload.Comments[0].EditedAt == nil {
		t.Error("edited comment has edited_at null")
	}

	if len(payload.Comments[0].Replies) != 1 {
		t.Fatalf("root replies = %d, want 1", len(payload.Comments[0].Replies))
	}
	placeholder := payload.Comments[0].Replies[0]
	if placeholder.ID != deletedParent.PublicID {
		t.Errorf("placeholder ID = %q, want %q", placeholder.ID, deletedParent.PublicID)
	}
	if !placeholder.Deleted {
		t.Error("deleted parent placeholder has deleted = false")
	}
	if placeholder.Author != "" || placeholder.Body != "" || placeholder.BodyHTML != "" {
		t.Errorf("deleted placeholder leaks content: %+v", placeholder)
	}
	if len(placeholder.Replies) != 1 || placeholder.Replies[0].ID != reply.PublicID {
		t.Fatalf("placeholder replies = %+v, want the live reply", placeholder.Replies)
	}
	if placeholder.Replies[0].Body != "reply **here**" {
		t.Errorf("reply body = %q", placeholder.Replies[0].Body)
	}
	if !strings.Contains(placeholder.Replies[0].BodyHTML, "<strong>here</strong>") {
		t.Errorf("reply body_html = %q", placeholder.Replies[0].BodyHTML)
	}

	body := response.Body.String()
	if strings.Contains(body, "hidden") {
		t.Error("response contains a deleted comment that has no visible replies")
	}
	if strings.Contains(body, `"replies":null`) {
		t.Error("response encodes replies as null instead of an empty array")
	}
}

func TestAPIAccessFollowsTheFeedRule(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())

	for user, token := range map[*db.User]string{alice: "alice-token", bob: "bob-token"} {
		if err := server.db.SetFeedToken(context.Background(), user.ID, token); err != nil {
			t.Fatalf("set feed token: %v", err)
		}
	}
	if _, err := server.db.DisableUser(context.Background(), bob.ID, server.now()); err != nil {
		t.Fatalf("disable bob: %v", err)
	}

	paths := []string{"/api/threads", "/api/threads/" + thread.PublicID}

	t.Run("private forum without credentials", func(t *testing.T) {
		for _, path := range paths {
			response := doRequest(t, server, http.MethodGet, path, nil, nil, "")
			if response.Code != http.StatusNotFound {
				t.Errorf("%s status = %d, want %d", path, response.Code, http.StatusNotFound)
			}
			if location := response.Header().Get("Location"); location != "" {
				t.Errorf("%s redirected to %q instead of a 404", path, location)
			}
			if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
				t.Errorf("%s Content-Type = %q", path, got)
			}
			var failure apiErrorResponse
			if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil || failure.Error == "" {
				t.Errorf("%s error body = %q, err = %v", path, response.Body.String(), err)
			}
		}
	})

	t.Run("private forum with a feed token", func(t *testing.T) {
		for _, path := range paths {
			response := doRequest(t, server, http.MethodGet, feedURLWithToken(path, "alice-token"), nil, nil, "")
			if response.Code != http.StatusOK {
				t.Errorf("%s status = %d, want %d", path, response.Code, http.StatusOK)
			}
			if cookies := response.Result().Cookies(); len(cookies) != 0 {
				t.Errorf("%s set cookies: %v", path, cookies)
			}
		}
	})

	t.Run("private forum with a session", func(t *testing.T) {
		cookie := signIn(t, server, alice)
		for _, path := range paths {
			response := doRequest(t, server, http.MethodGet, path, nil, cookie, "")
			if response.Code != http.StatusOK {
				t.Errorf("%s status = %d, want %d", path, response.Code, http.StatusOK)
			}
		}
	})

	t.Run("disabled user's token", func(t *testing.T) {
		for _, path := range paths {
			response := doRequest(t, server, http.MethodGet, feedURLWithToken(path, "bob-token"), nil, nil, "")
			if response.Code != http.StatusNotFound {
				t.Errorf("%s status = %d, want %d", path, response.Code, http.StatusNotFound)
			}
		}
	})

	t.Run("public forum without credentials", func(t *testing.T) {
		setPublic(t, server, true)
		for _, path := range paths {
			response := doRequest(t, server, http.MethodGet, path, nil, nil, "")
			if response.Code != http.StatusOK {
				t.Errorf("%s status = %d, want %d", path, response.Code, http.StatusOK)
			}
		}
	})
}

func TestAPIAccessFailuresAreJSON(t *testing.T) {
	server := newTestServer(t)
	// Closing the database makes the access check's Settings lookup fail.
	if err := server.db.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	response := doRequest(t, server, http.MethodGet, "/api/threads", nil, nil, "")
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	var failure apiErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil || failure.Error == "" {
		t.Errorf("error body = %q, err = %v", response.Body.String(), err)
	}

	// The RSS feeds keep their plain-text error.
	feed := doRequest(t, server, http.MethodGet, "/feeds/threads", nil, nil, "")
	if feed.Code != http.StatusInternalServerError {
		t.Errorf("feed status = %d, want %d", feed.Code, http.StatusInternalServerError)
	}
	if got := feed.Header().Get("Content-Type"); strings.HasPrefix(got, "application/json") {
		t.Errorf("feed Content-Type = %q, want plain text", got)
	}
}

func TestAPIThreadNotFound(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)

	deleted := createThreadDirect(t, server, alice, "Deleted", "", "body", server.now())
	if _, err := server.db.DeleteThread(context.Background(), deleted.ID, server.now()); err != nil {
		t.Fatalf("delete thread: %v", err)
	}

	for name, path := range map[string]string{
		"unknown ID":   "/api/threads/ZZZZZZZZ",
		"malformed ID": "/api/threads/not-a-real-id",
		"deleted":      "/api/threads/" + deleted.PublicID,
	} {
		t.Run(name, func(t *testing.T) {
			response := doRequest(t, server, http.MethodGet, path, nil, cookie, "")
			if response.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
			}
			var failure apiErrorResponse
			if err := json.Unmarshal(response.Body.Bytes(), &failure); err != nil || failure.Error == "" {
				t.Errorf("error body = %q, err = %v", response.Body.String(), err)
			}
		})
	}
}
