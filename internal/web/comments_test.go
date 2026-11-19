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

func TestThreadPageMarksCommentsNewSinceLastVisit(t *testing.T) {
	server := newTestServer(t)
	base := time.Unix(1_700_000_000, 0).UTC()
	server.now = func() time.Time { return base }

	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	cookie := signIn(t, server, bob)

	thread := createThreadDirect(t, server, alice, "Thread", "", "body", base.Add(time.Minute))
	fromAlice := createCommentDirect(t, server, thread, alice, nil, "from alice", base.Add(2*time.Minute))
	fromBob := createCommentDirect(t, server, thread, bob, nil, "from bob", base.Add(3*time.Minute))
	deleted := createCommentDirect(t, server, thread, alice, nil, "gone", base.Add(4*time.Minute))
	if _, err := server.db.DeleteComment(context.Background(), deleted.ID, base.Add(5*time.Minute)); err != nil {
		t.Fatalf("delete comment: %v", err)
	}

	// Bob has never opened the thread, so his cutoff is his unread_since at
	// signup: alice's comments are new, his own and the deleted one are not.
	openedAt := base.Add(10 * time.Minute)
	server.now = func() time.Time { return openedAt }
	body := doRequest(t, server, http.MethodGet, threadPath(thread.PublicID), nil, cookie, "").Body.String()

	assertNewComment(t, body, fromAlice.PublicID, true)
	assertNewComment(t, body, fromBob.PublicID, false)
	assertNewComment(t, body, deleted.PublicID, false)

	// Opening the page stores the visit as the thread's read time.
	var lastReadAt int64
	if err := server.db.QueryRow(
		"SELECT last_read_at FROM thread_reads WHERE user_id = ? AND thread_id = ?", bob.ID, thread.ID,
	).Scan(&lastReadAt); err != nil {
		t.Fatalf("read thread_reads row: %v", err)
	}
	if lastReadAt != openedAt.Unix() {
		t.Errorf("last_read_at = %d, want the visit time %d", lastReadAt, openedAt.Unix())
	}

	// The next visit has nothing new: the earlier visit moved the cutoff past
	// every comment.
	server.now = func() time.Time { return openedAt.Add(time.Minute) }
	body = doRequest(t, server, http.MethodGet, threadPath(thread.PublicID), nil, cookie, "").Body.String()
	for _, comment := range []*db.Comment{fromAlice, fromBob} {
		assertNewComment(t, body, comment.PublicID, false)
	}
}

func TestInvalidCommentDoesNotMarkThreadRead(t *testing.T) {
	server := newTestServer(t)
	base := time.Unix(1_700_000_000, 0).UTC()
	server.now = func() time.Time { return base }

	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	cookie := signIn(t, server, bob)

	thread := createThreadDirect(t, server, alice, "Thread", "", "body", base.Add(time.Minute))
	fromAlice := createCommentDirect(t, server, thread, alice, nil, "from alice", base.Add(2*time.Minute))

	// The rejected comment re-renders the page. That must not consume the new
	// markers or write read state.
	server.now = func() time.Time { return base.Add(5 * time.Minute) }
	response := postForm(t, server, threadPath(thread.PublicID)+"/comments", url.Values{"body": {"  \n\t"}}, cookie)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	assertNewComment(t, response.Body.String(), fromAlice.PublicID, false)

	var rows int
	if err := server.db.QueryRow(
		"SELECT COUNT(*) FROM thread_reads WHERE user_id = ? AND thread_id = ?", bob.ID, thread.ID,
	).Scan(&rows); err != nil {
		t.Fatalf("count thread_reads: %v", err)
	}
	if rows != 0 {
		t.Error("a rejected comment marked the thread read")
	}
}

func TestVisitorGetsNoReadState(t *testing.T) {
	server := newTestServer(t)
	base := time.Unix(1_700_000_000, 0).UTC()
	server.now = func() time.Time { return base }

	alice := createTestUser(t, server, "alice", false)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", base.Add(time.Minute))
	createCommentDirect(t, server, thread, alice, nil, "comment", base.Add(2*time.Minute))
	setPublic(t, server, true)

	server.now = func() time.Time { return base.Add(3 * time.Minute) }
	page := doRequest(t, server, http.MethodGet, threadPath(thread.PublicID), nil, nil, "")
	if page.Code != http.StatusOK {
		t.Fatalf("thread page status = %d, want %d", page.Code, http.StatusOK)
	}
	if body := page.Body.String(); strings.Contains(body, `class="c new"`) || strings.Contains(body, `class="tag">new</span>`) {
		t.Error("thread page shows new markers to a visitor")
	}

	index := doRequest(t, server, http.MethodGet, "/", nil, nil, "")
	if index.Code != http.StatusOK {
		t.Fatalf("front page status = %d, want %d", index.Code, http.StatusOK)
	}
	if body := index.Body.String(); strings.Contains(body, "unread-dot") || strings.Contains(body, "new-count") {
		t.Error("front page shows read-state markers to a visitor")
	}

	var rows int
	if err := server.db.QueryRow("SELECT COUNT(*) FROM thread_reads").Scan(&rows); err != nil {
		t.Fatalf("count thread_reads: %v", err)
	}
	if rows != 0 {
		t.Errorf("thread_reads rows = %d, want 0; a visitor wrote read state", rows)
	}
}

// assertNewComment checks the markup around one comment: the shaded .new list
// item and the header tag that app.js also builds for live comments, in the
// same place before the collapse toggle.
func assertNewComment(t *testing.T, body, publicID string, want bool) {
	t.Helper()

	if !want {
		if strings.Contains(body, `<li class="c new" id="c`+publicID+`"`) {
			t.Errorf("comment %s is marked new", publicID)
		}
		return
	}

	start := strings.Index(body, `<li class="c new" id="c`+publicID+`"`)
	if start < 0 {
		t.Fatalf("comment %s is not marked new", publicID)
	}

	tagIndex := strings.Index(body[start:], `<span class="tag">new</span>`)
	toggleIndex := strings.Index(body[start:], `class="tog"`)
	if tagIndex < 0 || toggleIndex < 0 || tagIndex > toggleIndex {
		t.Errorf("comment %s does not put the new tag before the collapse toggle", publicID)
	}
}

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
		`<a href="#comments" data-comment-count>2 comments</a>`,
		">2 comments</span> · best first",
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

func TestCommentFormsKeepPreviewHiddenWithoutJS(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	comment := createCommentDirect(t, server, thread, alice, nil, "a comment", server.now())

	pages := []struct {
		name string
		path string
	}{
		{name: "thread", path: threadPath(thread.PublicID)},
		{name: "reply", path: commentAction(comment.PublicID, "reply")},
		{name: "edit", path: commentAction(comment.PublicID, "edit")},
	}
	for _, page := range pages {
		t.Run(page.name, func(t *testing.T) {
			body := doRequest(t, server, http.MethodGet, page.path, nil, cookie, "").Body.String()
			for _, want := range []string{
				`role="tablist" hidden data-tabs`,
				`data-tab="write">write</button>`,
				`data-tab="preview">preview</button>`,
				`class="preview md" hidden data-preview`,
			} {
				if !strings.Contains(body, want) {
					t.Errorf("%s page does not contain %q", page.name, want)
				}
			}
		})
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
	replyPage := response.Body.String()
	if !strings.Contains(replyPage, `action="/comments/`+rootID+`/reply"`) {
		t.Error("reply page does not post to the reply route")
	}
	if !strings.Contains(replyPage, "<title>Reply · "+db.DefaultForumName+"</title>") {
		t.Error("reply page browser title changed")
	}
	for _, want := range []string{"~/</span>comments/reply", ">post reply</button>", "data-draft-cancel>cancel</a>"} {
		if !strings.Contains(replyPage, want) {
			t.Errorf("reply page does not contain %q", want)
		}
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
	editPage := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(editPage, "original") {
		t.Fatalf("author edit page: status = %d, body = %q", response.Code, editPage)
	}
	if !strings.Contains(editPage, "<title>Edit comment · "+db.DefaultForumName+"</title>") {
		t.Error("edit comment page browser title changed")
	}
	for _, want := range []string{"~/</span>comments/edit", ">save changes</button>"} {
		if !strings.Contains(editPage, want) {
			t.Errorf("edit comment page does not contain %q", want)
		}
	}

	response = postForm(t, server, commentAction(comment.PublicID, "edit"), url.Values{"body": {"edited **text**"}}, bobCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("edit status = %d, want %d", response.Code, http.StatusSeeOther)
	}

	page := doRequest(t, server, http.MethodGet, threadPath(thread.PublicID), nil, bobCookie, "")
	for _, want := range []string{
		"edited <strong>text</strong>",
		"edited",
		`data-confirm="Delete this comment? This cannot be undone. Replies to it stay."`,
	} {
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
		commentAction(comment.PublicID, "reaction"),
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

// TestCommentReactions covers the reaction route, its JSON response, the
// chips each kind of viewer sees, and the database rules the chips rely on.
func TestCommentReactions(t *testing.T) {
	server := newTestServer(t)
	// Visitors see the chips only when the forum is readable without an
	// account, which the last part of this test checks.
	setPublic(t, server, true)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	aliceCookie := signIn(t, server, alice)
	bobCookie := signIn(t, server, bob)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	comment := createCommentDirect(t, server, thread, alice, nil, "comment", server.now())

	reactionPath := commentAction(comment.PublicID, "reaction")

	// A plain form submission redirects to the comment anchor, like a vote.
	response := postForm(t, server, reactionPath, url.Values{"reaction": {"heart"}}, bobCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("reaction status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	if location := response.Header().Get("Location"); location != commentAnchorPath(thread.PublicID, comment.PublicID) {
		t.Errorf("reaction Location = %q, want %q", location, commentAnchorPath(thread.PublicID, comment.PublicID))
	}

	// The member who reacted sees a lit chip and the react control.
	body := doRequest(t, server, http.MethodGet, threadPath(thread.PublicID), nil, bobCookie, "").Body.String()
	for _, want := range []string{
		`class="rx"`,
		`class="chip on"`,
		`<span class="rx-emoji" aria-hidden="true">❤️</span><span class="rx-count">1</span>`,
		`aria-label="Love, 1 reaction"`,
		`<details class="react">`,
		`aria-label="React with Love"`,
		`action="` + reactionPath + `"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("reaction page does not contain %q", want)
		}
	}
	// Another reaction with no count gets no chip.
	if strings.Contains(body, `class="chip" data-reaction="laugh"`) {
		t.Error("a reaction with no count is shown as a chip")
	}

	// Changing the reaction relabels the vote and leaves the score alone.
	vote := doJSONFormPost(t, server, reactionPath, url.Values{"reaction": {"laugh"}}, bobCookie)
	if vote.Code != http.StatusOK {
		t.Fatalf("change reaction status = %d, want %d", vote.Code, http.StatusOK)
	}
	var payload voteResponse
	if err := json.Unmarshal(vote.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode reaction response: %v (body %q)", err, vote.Body.String())
	}
	if payload.Reaction != "laugh" || payload.Score != 2 || !payload.Voted || payload.ScoreText != "2 points" {
		t.Errorf("reaction response = %+v", payload)
	}
	counts := reactionCountsByKey(payload.Reactions)
	if counts["laugh"] != 1 || counts["heart"] != 0 {
		t.Errorf("reaction counts = %v", counts)
	}

	// The same reaction again removes both the emoji and the vote.
	vote = doJSONFormPost(t, server, reactionPath, url.Values{"reaction": {"laugh"}}, bobCookie)
	if vote.Code != http.StatusOK {
		t.Fatalf("remove reaction status = %d, want %d", vote.Code, http.StatusOK)
	}
	if err := json.Unmarshal(vote.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode reaction removal: %v", err)
	}
	if payload.Reaction != "" || payload.Voted || payload.Score != 1 {
		t.Errorf("reaction removal = %+v", payload)
	}
	body = doRequest(t, server, http.MethodGet, threadPath(thread.PublicID), nil, bobCookie, "").Body.String()
	if strings.Contains(body, `class="rx"`) {
		t.Error("a comment with no reactions still shows a chips row")
	}

	// A reaction key outside the fixed set is a bad request.
	for _, form := range []url.Values{{"reaction": {"shock"}}, {"reaction": {""}}} {
		response = postForm(t, server, reactionPath, form, bobCookie)
		if response.Code != http.StatusBadRequest {
			t.Errorf("unknown reaction status = %d, want %d", response.Code, http.StatusBadRequest)
		}
		response = doJSONFormPost(t, server, reactionPath, form, bobCookie)
		if response.Code != http.StatusBadRequest {
			t.Errorf("unknown reaction JSON status = %d, want %d", response.Code, http.StatusBadRequest)
		}
	}

	// The comment's author cannot vote, so they cannot react either.
	response = postForm(t, server, reactionPath, url.Values{"reaction": {"heart"}}, aliceCookie)
	if response.Code != http.StatusForbidden {
		t.Errorf("own comment reaction status = %d, want %d", response.Code, http.StatusForbidden)
	}

	// Put a reaction back for the viewer and visitor checks below.
	if response = postForm(t, server, reactionPath, url.Values{"reaction": {"thanks"}}, bobCookie); response.Code != http.StatusSeeOther {
		t.Fatalf("re-reaction status = %d, want %d", response.Code, http.StatusSeeOther)
	}

	// The author and a visitor see plain chips with no controls.
	for name, cookie := range map[string]*http.Cookie{"visitor": nil, "author": aliceCookie} {
		body = doRequest(t, server, http.MethodGet, threadPath(thread.PublicID), nil, cookie, "").Body.String()
		if !strings.Contains(body, `class="chip" role="img" data-reaction="thanks"`) {
			t.Errorf("%s does not see a plain Thanks chip", name)
		}
		if strings.Contains(body, `class="reaction-form"`) || strings.Contains(body, `<details class="react">`) {
			t.Errorf("%s gets reaction controls", name)
		}
	}

	// A live view of the comment keeps the viewer's own highlight, and the
	// delete rendering drops the chips entirely.
	stored, err := server.db.CommentByPublicID(context.Background(), comment.PublicID, bob.ID)
	if err != nil || stored == nil {
		t.Fatalf("CommentByPublicID: comment = %v, err = %v", stored, err)
	}
	live, err := server.commentPayload(thread, stored, bob)
	if err != nil {
		t.Fatalf("commentPayload: %v", err)
	}
	if !strings.Contains(live.HTML, `class="chip on"`) || !strings.Contains(live.HTML, `data-reaction="thanks"`) {
		t.Errorf("live comment does not keep the viewer's highlight: %s", live.HTML)
	}
	deleted, err := server.commentDeletePayload(thread, stored)
	if err != nil {
		t.Fatalf("commentDeletePayload: %v", err)
	}
	if strings.Contains(deleted.HTML, `class="rx"`) {
		t.Errorf("deleted comment markup still has reaction chips: %s", deleted.HTML)
	}

	// A deleted comment cannot take a reaction.
	if _, err := server.db.DeleteComment(context.Background(), comment.ID, server.now()); err != nil {
		t.Fatalf("delete comment: %v", err)
	}
	response = postForm(t, server, reactionPath, url.Values{"reaction": {"heart"}}, bobCookie)
	if response.Code != http.StatusNotFound {
		t.Errorf("reaction on a deleted comment: status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

// reactionCountsByKey turns the response's ordered list into a lookup.
func reactionCountsByKey(list []reactionCountResponse) map[string]int64 {
	counts := make(map[string]int64, len(list))
	for _, entry := range list {
		counts[entry.Key] = entry.Count
	}
	return counts
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

// doJSONFormPost submits a form value with the JSON Accept header, the way
// app.js submits a reaction.
func doJSONFormPost(t *testing.T, server *Server, path string, form url.Values, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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
