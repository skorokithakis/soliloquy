package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/skorokithakis/soliloquy/internal/auth"
)

func TestCommentPageShowsFocusedBranchAsRoot(t *testing.T) {
	server := newTestServer(t)
	base := time.Unix(1_700_000_000, 0).UTC()
	server.now = func() time.Time { return base.Add(24 * time.Hour) }

	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	cookie := signIn(t, server, bob)

	thread := createThreadDirect(t, server, alice, "Thread", "", "thread body text", base)
	root := createCommentDirect(t, server, thread, alice, nil, "branch root", base.Add(time.Minute))
	focused := createCommentDirect(t, server, thread, alice, &root.ID, "focused **body**", base.Add(2*time.Minute))
	reply := createCommentDirect(t, server, thread, bob, &focused.ID, "direct reply", base.Add(3*time.Minute))
	nested := createCommentDirect(t, server, thread, alice, &reply.ID, "nested reply", base.Add(4*time.Minute))
	createCommentDirect(t, server, thread, alice, &root.ID, "sibling branch", base.Add(5*time.Minute))
	createCommentDirect(t, server, thread, alice, nil, "other top level", base.Add(6*time.Minute))

	response := doRequest(t, server, http.MethodGet, commentPagePath(focused.PublicID), nil, cookie, "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()

	for _, want := range []string{
		`<a class="tl" href="` + threadPath(thread.PublicID) + `#c` + focused.PublicID + `"`,
		`<li class="c focus" id="c` + focused.PublicID + `"`,
		"focused <strong>body</strong>",
		"direct reply",
		"nested reply",
		`<span class="label">2 replies · best first</span>`,
		`<a class="par" href="#c` + focused.PublicID + `">parent</a>`,
		`<a class="reply-link" href="/comments/` + focused.PublicID + `/reply?return_to=` + focused.PublicID + `">reply</a>`,
		`<a class="reply-link" href="/comments/` + reply.PublicID + `/reply?return_to=` + focused.PublicID + `">reply</a>`,
		// The parent is now shown in the context area as a read-only row.
		`<section class="ctx"`,
		"branch root",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("comment page does not contain %q", want)
		}
	}

	for _, unwanted := range []string{
		"sibling branch",
		"other top level",
		"thread body text",
		`id="comments"`,
		`id="c` + root.PublicID + `"`,
	} {
		if strings.Contains(body, unwanted) {
			t.Errorf("comment page contains %q", unwanted)
		}
	}

	// The focused comment's own header has no parent link. Replies still have
	// one, so only inspect the card's header.
	start := strings.Index(body, `<li class="c focus" id="c`+focused.PublicID+`"`)
	end := strings.Index(body[start:], `<div class="cb`)
	if start < 0 || end < 0 {
		t.Fatal("focused comment markup is incomplete")
	}
	if strings.Contains(body[start:start+end], `class="par"`) {
		t.Error("the focused comment header has a parent link")
	}

	if !strings.Contains(body, `data-parent-id="`+focused.PublicID+`"`) {
		t.Error("the replies are not in a container keyed by the focused comment")
	}
	// The nested reply lives inside its own reply's container, not directly
	// under the focused comment.
	nestedStart := strings.Index(body, `id="c`+nested.PublicID+`"`)
	replyContainer := strings.Index(body, `data-parent-id="`+reply.PublicID+`"`)
	if nestedStart < 0 || replyContainer < 0 || nestedStart < replyContainer {
		t.Error("the nested reply is not nested under its parent")
	}
}

func TestCommentPageVisitor(t *testing.T) {
	server := newTestServer(t)
	setPublic(t, server, true)

	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	focused := createCommentDirect(t, server, thread, alice, nil, "focused text", server.now().Add(time.Minute))
	createCommentDirect(t, server, thread, bob, &focused.ID, "a reply", server.now().Add(2*time.Minute))

	response := doRequest(t, server, http.MethodGet, commentPagePath(focused.PublicID), nil, nil, "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()

	for _, want := range []string{
		"focused text",
		"a reply",
		`<span class="u">alice</span>`,
		`<a class="lg" href="/login">log in to reply</a>`,
		`<span class="score">1 point</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("visitor comment page does not contain %q", want)
		}
	}

	for _, unwanted := range []string{
		"data-sse-url",
		`class="vote-form"`,
		`aria-label="Upvote"`,
		`class="reply-link"`,
		`href="/comments/` + focused.PublicID + `/edit"`,
		`action="/comments/` + focused.PublicID + `/delete"`,
		`data-reply-dock`,
	} {
		if strings.Contains(body, unwanted) {
			t.Errorf("visitor comment page contains %q", unwanted)
		}
	}
}

func TestCommentPageAuthorSeesOwnVote(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	focused := createCommentDirect(t, server, thread, alice, nil, "own comment", server.now())

	body := doRequest(t, server, http.MethodGet, commentPagePath(focused.PublicID), nil, cookie, "").Body.String()
	for _, want := range []string{
		`action="/comments/` + focused.PublicID + `/vote"`,
		`class="vote on" type="submit" aria-label="Remove upvote"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("author's comment page does not contain %q", want)
		}
	}
	if strings.Contains(body, `<details class="react">`) {
		t.Error("the author's own comment has a react control")
	}
}

func TestCommentPageNotFound(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	deleted := createCommentDirect(t, server, thread, alice, nil, "gone", server.now().Add(time.Minute))
	if _, err := server.db.DeleteComment(context.Background(), deleted.ID, server.now()); err != nil {
		t.Fatalf("delete comment: %v", err)
	}

	deletedThread := createThreadDirect(t, server, alice, "Deleted thread", "", "body", server.now())
	onDeletedThread := createCommentDirect(t, server, deletedThread, alice, nil, "comment", server.now())
	if _, err := server.db.DeleteThread(context.Background(), deletedThread.ID, server.now()); err != nil {
		t.Fatalf("delete thread: %v", err)
	}

	unknown, err := auth.GenerateShortID()
	if err != nil {
		t.Fatalf("generate short ID: %v", err)
	}

	for name, path := range map[string]string{
		"unknown comment":           commentPagePath(unknown),
		"malformed comment":         "/comments/nope",
		"deleted comment":           commentPagePath(deleted.PublicID),
		"comment on deleted thread": commentPagePath(onDeletedThread.PublicID),
	} {
		response := doRequest(t, server, http.MethodGet, path, nil, cookie, "")
		if response.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want %d", name, response.Code, http.StatusNotFound)
		}
	}
}

func TestCommentPageShowsDeletedCommentWithReplies(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	cookie := signIn(t, server, bob)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())

	focused := createCommentDirect(t, server, thread, alice, nil, "deleted body", server.now().Add(time.Minute))
	createCommentDirect(t, server, thread, bob, &focused.ID, "live reply", server.now().Add(2*time.Minute))
	if _, err := server.db.DeleteComment(context.Background(), focused.ID, server.now()); err != nil {
		t.Fatalf("delete comment: %v", err)
	}

	response := doRequest(t, server, http.MethodGet, commentPagePath(focused.PublicID), nil, cookie, "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	if !strings.Contains(body, "[deleted]") {
		t.Error("deleted focused comment is not shown as [deleted]")
	}
	if strings.Contains(body, "deleted body") {
		t.Error("deleted focused comment's body is still rendered")
	}
	if !strings.Contains(body, "live reply") {
		t.Error("the reply to the deleted focused comment is missing")
	}
	if !strings.Contains(body, `<span class="label">1 reply · best first</span>`) {
		t.Error("the reply header does not count only visible replies")
	}
	if strings.Contains(body, "data-reply-dock") {
		t.Error("the deleted focused comment still shows the phone reply bar")
	}
}

func TestCommentPageDoesNotMarkThreadRead(t *testing.T) {
	server := newTestServer(t)
	base := time.Unix(1_700_000_000, 0).UTC()
	server.now = func() time.Time { return base }

	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	cookie := signIn(t, server, bob)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", base)
	comment := createCommentDirect(t, server, thread, alice, nil, "comment", base.Add(time.Minute))

	server.now = func() time.Time { return base.Add(10 * time.Minute) }
	body := doRequest(t, server, http.MethodGet, commentPagePath(comment.PublicID), nil, cookie, "").Body.String()
	if strings.Contains(body, `class="c new"`) || strings.Contains(body, `class="tag">new</span>`) {
		t.Error("the comment page shows a new-since-last-visit marker")
	}

	var rows int
	if err := server.db.QueryRow(
		"SELECT COUNT(*) FROM thread_reads WHERE user_id = ? AND thread_id = ?", bob.ID, thread.ID,
	).Scan(&rows); err != nil {
		t.Fatalf("count thread_reads: %v", err)
	}
	if rows != 0 {
		t.Error("the comment page marked the thread read")
	}
}

func TestReplyRedirectsFromCommentPageAndThreadPage(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	cookie := signIn(t, server, bob)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	focused := createCommentDirect(t, server, thread, alice, nil, "focused", server.now().Add(time.Minute))

	// The standalone reply page keeps the return path in its form action.
	replyPage := doRequest(t, server, http.MethodGet, commentPagePath(focused.PublicID)+"/reply?return_to="+focused.PublicID, nil, cookie, "")
	if replyPage.Code != http.StatusOK {
		t.Fatalf("reply page status = %d, want %d", replyPage.Code, http.StatusOK)
	}
	wantAction := `action="/comments/` + focused.PublicID + `/reply?return_to=` + focused.PublicID + `"`
	if !strings.Contains(replyPage.Body.String(), wantAction) {
		t.Errorf("reply page does not contain %q", wantAction)
	}

	response := postForm(t, server, commentPagePath(focused.PublicID)+"/reply?return_to="+focused.PublicID, url.Values{
		"body": {"from the comment page"},
	}, cookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("reply status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	replyID := anchorPublicID(t, response.Header().Get("Location"))
	if location := response.Header().Get("Location"); location != commentPagePath(focused.PublicID)+"#c"+replyID {
		t.Errorf("Location = %q, want the focused comment page at the new reply", location)
	}

	response = postForm(t, server, commentAction(focused.PublicID, "reply"), url.Values{
		"body": {"from the thread page"},
	}, cookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("thread reply status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	threadReplyID := anchorPublicID(t, response.Header().Get("Location"))
	if location := response.Header().Get("Location"); location != threadPath(thread.PublicID)+"#c"+threadReplyID {
		t.Errorf("Location = %q, want the thread anchor from the thread page", location)
	}

	// An invalid return path falls back to the thread anchor.
	response = postForm(t, server, commentAction(focused.PublicID, "reply")+"?return_to=../../evil", url.Values{
		"body": {"with a bad return path"},
	}, cookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("invalid return path status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	badReplyID := anchorPublicID(t, response.Header().Get("Location"))
	if location := response.Header().Get("Location"); location != threadPath(thread.PublicID)+"#c"+badReplyID {
		t.Errorf("Location = %q, want the thread anchor for an invalid return path", location)
	}

	// A failed reply re-renders the page and keeps the return path.
	response = postForm(t, server, commentPagePath(focused.PublicID)+"/reply?return_to="+focused.PublicID, url.Values{
		"body": {"  \n\t"},
	}, cookie)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid reply status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if !strings.Contains(response.Body.String(), wantAction) {
		t.Error("the failed reply form lost its return path")
	}
}

func TestCommentTimestampsLinkToCommentPage(t *testing.T) {
	server := newTestServer(t)
	base := time.Unix(1_700_000_000, 0).UTC()
	server.now = func() time.Time { return base.Add(2 * time.Hour) }

	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", base)
	comment := createCommentDirect(t, server, thread, alice, nil, "comment", base)
	deleted := createCommentDirect(t, server, thread, alice, nil, "gone", base.Add(time.Minute))
	createCommentDirect(t, server, thread, alice, &deleted.ID, "reply", base.Add(2*time.Minute))
	if _, err := server.db.DeleteComment(context.Background(), deleted.ID, base.Add(3*time.Minute)); err != nil {
		t.Fatalf("delete comment: %v", err)
	}

	body := doRequest(t, server, http.MethodGet, threadPath(thread.PublicID), nil, cookie, "").Body.String()

	for _, want := range []string{
		`<a class="ts" href="/comments/` + comment.PublicID + `" title="` + comment.CreatedAt.UTC().Format("2006-01-02 15:04") + ` UTC">`,
		`<a class="ts" href="/comments/` + deleted.PublicID + `" title="` + deleted.CreatedAt.UTC().Format("2006-01-02 15:04") + ` UTC">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("thread page does not contain %q", want)
		}
	}
}
