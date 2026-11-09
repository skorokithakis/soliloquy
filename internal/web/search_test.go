package web

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSearchPage(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)
	now := server.now()

	thread := createThreadDirect(t, server, alice, "Needle thread", "", "A thread body", now)
	comment := createCommentDirect(t, server, thread, alice, nil, "danger <script>alert(1)</script> needle text", now.Add(time.Minute))

	response := doRequest(t, server, http.MethodGet, "/search?q="+url.QueryEscape("needle"), nil, cookie, "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}

	body := response.Body.String()
	for _, want := range []string{
		`action="/search"`,
		`name="q"`,
		`value="needle"`,
		`href="` + threadPath(thread.ID) + `"`,
		`href="` + commentAnchorPath(thread.ID, comment.ID) + `"`,
		"<mark>needle</mark>",
		"&lt;script&gt;alert(1)&lt;/script&gt;",
		"alice",
		"1 comment",
		"open comment ›",
		"class=\"ty t\"",
		"class=\"ty\"",
		"class=\"ctx\"",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("search page does not contain %q", want)
		}
	}
	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Error("search page renders unescaped post content")
	}

	index := doRequest(t, server, http.MethodGet, "/", nil, cookie, "")
	if !strings.Contains(index.Body.String(), `action="/search"`) {
		t.Error("header has no search form")
	}
}

func TestSearchPageEmptyQuery(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)

	response := doRequest(t, server, http.MethodGet, "/search", nil, cookie, "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	if !strings.Contains(body, "Search thread titles, thread bodies and comments.") {
		t.Error("empty search page does not show the hint")
	}

	// FTS5 syntax characters must produce a normal page, not an error.
	response = doRequest(t, server, http.MethodGet, "/search?q="+url.QueryEscape(`"*`), nil, cookie, "")
	if response.Code != http.StatusOK {
		t.Fatalf("syntax search status = %d, want %d", response.Code, http.StatusOK)
	}
	if !strings.Contains(response.Body.String(), `class="bigsearch"`) {
		t.Error("syntax search page did not render")
	}
}

func TestSearchShowsDeletedAuthor(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	cookie := signIn(t, server, bob)
	createThreadDirect(t, server, alice, "Deleted author thread", "", "findable body", server.now())

	if err := server.db.DeleteAccount(context.Background(), alice.ID, server.now()); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}

	body := doRequest(t, server, http.MethodGet, "/search?q=findable", nil, cookie, "").Body.String()
	if !strings.Contains(body, "[deleted user]") {
		t.Error("search page does not show [deleted user] for a deleted author")
	}
	if strings.Contains(body, ">alice<") {
		t.Error("search page shows the deleted author's username")
	}
}

func TestSearchLinksCommentAnchor(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)
	thread := createThreadDirect(t, server, alice, "Thread", "", "", server.now())
	comment := createCommentDirect(t, server, thread, alice, nil, "findable comment", server.now().Add(time.Minute))

	response := doRequest(t, server, http.MethodGet, "/search?q=findable", nil, cookie, "")
	body := response.Body.String()
	want := `href="` + threadPath(thread.ID) + "#c" + strconv.FormatInt(comment.ID, 10) + `"`
	if !strings.Contains(body, want) {
		t.Errorf("comment result link %q is missing", want)
	}
}
