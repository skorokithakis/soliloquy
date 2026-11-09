package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/skorokithakis/soliloquy/internal/db"
	"github.com/skorokithakis/soliloquy/internal/markdown"
)

func TestThreadPageRendersCommentTree(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	cookie := signIn(t, server, alice)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())

	now := server.now()
	root := createCommentDirect(t, server, thread, bob, nil, "root **bold**", now.Add(time.Minute))
	reply := createCommentDirect(t, server, thread, alice, &root.ID, "child reply", now.Add(2*time.Minute))

	response := doRequest(t, server, http.MethodGet, threadPath(thread.PublicID), nil, cookie, "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}

	body := response.Body.String()
	for _, want := range []string{
		`id="c` + root.PublicID + `"`,
		`id="c` + reply.PublicID + `"`,
		`data-parent-id="` + root.PublicID + `"`,
		"<strong>bold</strong>",
		"child reply",
		`action="/comments/` + root.PublicID + `/vote"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("thread page does not contain %q", want)
		}
	}

	// The reply must be nested inside the root's reply container, not just
	// present somewhere on the page.
	rootStart := strings.Index(body, `id="c`+root.PublicID+`"`)
	replyStart := strings.Index(body, `id="c`+reply.PublicID+`"`)
	containerStart := strings.Index(body, `data-parent-id="`+root.PublicID+`"`)
	if rootStart < 0 || containerStart < 0 || replyStart < 0 || replyStart < containerStart {
		t.Errorf("reply is not nested in the root's reply container")
	}

	index := doRequest(t, server, http.MethodGet, "/", nil, cookie, "")
	if !strings.Contains(index.Body.String(), "2 comments") {
		t.Error("front page does not show the comment count")
	}
}

func TestThreadPageCommentControls(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	bobCookie := signIn(t, server, bob)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	root := createCommentDirect(t, server, thread, alice, nil, "from the author", server.now().Add(time.Minute))
	reply := createCommentDirect(t, server, thread, bob, &root.ID, "a reply", server.now().Add(2*time.Minute))

	body := doRequest(t, server, http.MethodGet, threadPath(thread.PublicID), nil, bobCookie, "").Body.String()

	for _, want := range []string{
		`id="comment-form"`,
		`action="` + threadPath(thread.PublicID) + `/comments"`,
		"2 comments · best first",
		`class="live" data-live hidden`,
		`class="op"`,
		`<a class="par" href="#c` + root.PublicID + `">parent</a>`,
		`action="/comments/` + root.PublicID + `/vote"`,
		`class="tog" type="button" hidden`,
		`<a class="reply-link" href="/comments/` + root.PublicID + `/reply">reply</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("thread page does not contain %q", want)
		}
	}

	if formIndex, commentsIndex := strings.Index(body, `id="comment-form"`), strings.Index(body, `id="comments"`); formIndex < 0 || commentsIndex < 0 || formIndex > commentsIndex {
		t.Error("the comment box is not above the comment list")
	}

	// The viewer's own comment has no vote form, but the score is still shown.
	if strings.Contains(body, `action="/comments/`+reply.PublicID+`/vote"`) {
		t.Error("the viewer's own comment has a vote form")
	}
}

func TestDeletedCommentVisibility(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	cookie := signIn(t, server, bob)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())

	now := server.now()
	root := createCommentDirect(t, server, thread, bob, nil, "root text", now.Add(time.Minute))
	child := createCommentDirect(t, server, thread, alice, &root.ID, "child text", now.Add(2*time.Minute))
	leaf := createCommentDirect(t, server, thread, bob, nil, "leaf text", now.Add(3*time.Minute))

	if _, err := server.db.DeleteComment(context.Background(), root.ID, now.Add(4*time.Minute)); err != nil {
		t.Fatalf("delete root: %v", err)
	}

	response := doRequest(t, server, http.MethodGet, threadPath(thread.PublicID), nil, cookie, "")
	body := response.Body.String()
	if !strings.Contains(body, "[deleted]") {
		t.Error("a deleted comment with replies is not shown as [deleted]")
	}
	if strings.Contains(body, "root text") {
		t.Error("a deleted comment's body is still rendered")
	}
	if !strings.Contains(body, "child text") {
		t.Error("the reply to a deleted comment is missing")
	}

	if _, err := server.db.DeleteComment(context.Background(), child.ID, now.Add(5*time.Minute)); err != nil {
		t.Fatalf("delete child: %v", err)
	}
	if _, err := server.db.DeleteComment(context.Background(), leaf.ID, now.Add(5*time.Minute)); err != nil {
		t.Fatalf("delete leaf: %v", err)
	}

	response = doRequest(t, server, http.MethodGet, threadPath(thread.PublicID), nil, cookie, "")
	body = response.Body.String()
	if strings.Contains(body, "[deleted]") {
		t.Error("a deleted subtree with no visible replies still renders placeholders")
	}
	if strings.Contains(body, "child text") || strings.Contains(body, "leaf text") {
		t.Error("deleted comments are still rendered")
	}

	index := doRequest(t, server, http.MethodGet, "/", nil, cookie, "")
	if !strings.Contains(index.Body.String(), "0 comments") {
		t.Error("front page counts deleted comments")
	}
}

func TestCreateCommentAndReply(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	bobCookie := signIn(t, server, bob)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())

	response := postForm(t, server, threadPath(thread.PublicID)+"/comments", url.Values{
		"body": {"top level **comment**"},
	}, bobCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("comment status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	location := response.Header().Get("Location")
	if !strings.HasPrefix(location, threadPath(thread.PublicID)+"#c") {
		t.Fatalf("Location = %q, want a comment anchor", location)
	}
	rootID := anchorPublicID(t, location)

	response = doRequest(t, server, http.MethodGet, "/comments/"+rootID+"/reply", nil, bobCookie, "")
	if response.Code != http.StatusOK {
		t.Fatalf("reply page status = %d, want %d", response.Code, http.StatusOK)
	}
	if !strings.Contains(response.Body.String(), `action="/comments/`+rootID+`/reply"`) {
		t.Error("reply page does not post to the reply route")
	}

	response = postForm(t, server, "/comments/"+rootID+"/reply", url.Values{
		"body": {"a reply"},
	}, bobCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("reply status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	replyLocation := response.Header().Get("Location")
	replyID := anchorPublicID(t, replyLocation)
	if replyID == rootID {
		t.Fatal("reply reused the parent comment's ID")
	}

	page := doRequest(t, server, http.MethodGet, threadPath(thread.PublicID), nil, bobCookie, "")
	body := page.Body.String()
	if !strings.Contains(body, "top level <strong>comment</strong>") || !strings.Contains(body, "a reply") {
		t.Error("thread page does not render the new comment and reply")
	}
	if !strings.Contains(body, `id="c`+replyID+`"`) {
		t.Error("thread page has no anchor for the new reply")
	}
}

func TestCommentValidation(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	cookie := signIn(t, server, bob)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	comment := createCommentDirect(t, server, thread, alice, nil, "existing", server.now())

	tests := []struct {
		name string
		path string
		body string
		want string
	}{
		{name: "empty top level", path: threadPath(thread.PublicID) + "/comments", body: "  \n\t", want: "Comment is required"},
		{name: "long top level", path: threadPath(thread.PublicID) + "/comments", body: strings.Repeat("x", maxCommentBodyLength+1), want: "20000 characters"},
		{name: "empty reply", path: commentAction(comment.PublicID, "reply"), body: "", want: "Comment is required"},
		{name: "long reply", path: commentAction(comment.PublicID, "reply"), body: strings.Repeat("x", maxCommentBodyLength+1), want: "20000 characters"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := postForm(t, server, test.path, url.Values{"body": {test.body}}, cookie)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
			}
			if !strings.Contains(response.Body.String(), test.want) {
				t.Errorf("body does not contain %q", test.want)
			}
		})
	}

	var count int
	if err := server.db.QueryRow("SELECT COUNT(*) FROM comments").Scan(&count); err != nil {
		t.Fatalf("count comments: %v", err)
	}
	if count != 1 {
		t.Errorf("invalid submissions stored %d extra comments", count-1)
	}
}

func TestEditAndDeleteCommentPermissions(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	aliceCookie := signIn(t, server, alice)
	bobCookie := signIn(t, server, bob)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	comment := createCommentDirect(t, server, thread, bob, nil, "original", server.now())

	response := doRequest(t, server, http.MethodGet, commentAction(comment.PublicID, "edit"), nil, aliceCookie, "")
	if response.Code != http.StatusForbidden {
		t.Errorf("edit page as another user: status = %d, want %d", response.Code, http.StatusForbidden)
	}
	response = postForm(t, server, commentAction(comment.PublicID, "edit"), url.Values{"body": {"hijacked"}}, aliceCookie)
	if response.Code != http.StatusForbidden {
		t.Errorf("edit as another user: status = %d, want %d", response.Code, http.StatusForbidden)
	}
	response = postForm(t, server, commentAction(comment.PublicID, "delete"), nil, aliceCookie)
	if response.Code != http.StatusForbidden {
		t.Errorf("delete as another user: status = %d, want %d", response.Code, http.StatusForbidden)
	}

	response = doRequest(t, server, http.MethodGet, commentAction(comment.PublicID, "edit"), nil, bobCookie, "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "original") {
		t.Fatalf("author edit page: status = %d, body = %q", response.Code, response.Body.String())
	}

	response = postForm(t, server, commentAction(comment.PublicID, "edit"), url.Values{"body": {"edited **text**"}}, bobCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("edit status = %d, want %d", response.Code, http.StatusSeeOther)
	}

	page := doRequest(t, server, http.MethodGet, threadPath(thread.PublicID), nil, bobCookie, "")
	for _, want := range []string{"edited <strong>text</strong>", "edited"} {
		if !strings.Contains(page.Body.String(), want) {
			t.Errorf("thread page does not contain %q after edit", want)
		}
	}

	response = postForm(t, server, commentAction(comment.PublicID, "delete"), nil, bobCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("delete status = %d, want %d", response.Code, http.StatusSeeOther)
	}

	page = doRequest(t, server, http.MethodGet, threadPath(thread.PublicID), nil, bobCookie, "")
	if strings.Contains(page.Body.String(), "edited <strong>text</strong>") {
		t.Error("deleted comment is still rendered")
	}

	response = doRequest(t, server, http.MethodGet, commentAction(comment.PublicID, "edit"), nil, bobCookie, "")
	if response.Code != http.StatusNotFound {
		t.Errorf("edit page for a deleted comment: status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestCommentWritesOnDeletedThread(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	bobCookie := signIn(t, server, bob)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	comment := createCommentDirect(t, server, thread, bob, nil, "comment", server.now())

	if _, err := server.db.DeleteThread(context.Background(), thread.ID, server.now()); err != nil {
		t.Fatalf("delete thread: %v", err)
	}

	response := doRequest(t, server, http.MethodGet, commentAction(comment.PublicID, "reply"), nil, bobCookie, "")
	if response.Code != http.StatusNotFound {
		t.Errorf("reply page on a deleted thread: status = %d, want %d", response.Code, http.StatusNotFound)
	}
	for _, path := range []string{
		commentAction(comment.PublicID, "reply"),
		commentAction(comment.PublicID, "edit"),
		commentAction(comment.PublicID, "delete"),
		commentAction(comment.PublicID, "vote"),
	} {
		response = postForm(t, server, path, url.Values{"body": {"too late"}}, bobCookie)
		if response.Code != http.StatusNotFound {
			t.Errorf("%s on a deleted thread: status = %d, want %d", path, response.Code, http.StatusNotFound)
		}
	}
}

func TestVotes(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	aliceCookie := signIn(t, server, alice)
	bobCookie := signIn(t, server, bob)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	comment := createCommentDirect(t, server, thread, alice, nil, "comment", server.now())

	response := postForm(t, server, threadPath(thread.PublicID)+"/vote", nil, bobCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("thread vote status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	if location := response.Header().Get("Location"); location != threadPath(thread.PublicID) {
		t.Errorf("thread vote Location = %q, want %q", location, threadPath(thread.PublicID))
	}

	page := doRequest(t, server, http.MethodGet, threadPath(thread.PublicID), nil, bobCookie, "")
	if !strings.Contains(page.Body.String(), `data-score-format="number">2</span>`) {
		t.Error("thread page does not show the voted score")
	}

	response = postForm(t, server, threadPath(thread.PublicID)+"/vote", nil, bobCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("thread unvote status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	thread, err := server.db.ThreadByID(context.Background(), thread.ID)
	if err != nil || thread == nil {
		t.Fatalf("ThreadByID: thread = %v, err = %v", thread, err)
	}
	if thread.Score != 1 {
		t.Errorf("thread score after unvote = %d, want 1", thread.Score)
	}

	response = postForm(t, server, threadPath(thread.PublicID)+"/vote", nil, aliceCookie)
	if response.Code != http.StatusForbidden {
		t.Errorf("own thread vote status = %d, want %d", response.Code, http.StatusForbidden)
	}

	response = doJSONPost(t, server, commentAction(comment.PublicID, "vote"), bobCookie)
	if response.Code != http.StatusOK {
		t.Fatalf("comment vote status = %d, want %d", response.Code, http.StatusOK)
	}
	var vote voteResponse
	if err := json.Unmarshal(response.Body.Bytes(), &vote); err != nil {
		t.Fatalf("decode vote response: %v (body %q)", err, response.Body.String())
	}
	if vote.Score != 2 || !vote.Voted || vote.ScoreText != "2 points" || vote.ButtonText != "Unvote" {
		t.Errorf("vote response = %+v", vote)
	}

	response = doJSONPost(t, server, commentAction(comment.PublicID, "vote"), aliceCookie)
	if response.Code != http.StatusForbidden {
		t.Errorf("own comment vote status = %d, want %d", response.Code, http.StatusForbidden)
	}

	// The owner sees no vote form for their own comment.
	page = doRequest(t, server, http.MethodGet, threadPath(thread.PublicID), nil, aliceCookie, "")
	if strings.Contains(page.Body.String(), `action="/comments/`+comment.PublicID+`/vote"`) {
		t.Error("own comment has a vote form")
	}
}

func createCommentDirect(t *testing.T, server *Server, thread *db.Thread, user *db.User, parentID *int64, body string, at time.Time) *db.Comment {
	t.Helper()

	rendered, err := markdown.Render(body)
	if err != nil {
		t.Fatalf("render comment: %v", err)
	}

	comment, err := server.db.CreateComment(context.Background(), thread.ID, parentID, user.ID, body, string(rendered), at)
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}
	return comment
}

// doJSONPost submits a vote the way app.js does: a plain POST asking for JSON.
func doJSONPost(t *testing.T, server *Server, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(http.MethodPost, path, nil)
	request.Header.Set("Origin", testBaseURL)
	request.Header.Set("Accept", "application/json")
	request.AddCookie(cookie)

	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}

func anchorPublicID(t *testing.T, location string) string {
	t.Helper()

	_, anchor, found := strings.Cut(location, "#c")
	if !found || anchor == "" {
		t.Fatalf("Location %q has no comment anchor", location)
	}
	return anchor
}
