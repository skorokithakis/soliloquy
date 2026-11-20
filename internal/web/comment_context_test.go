package web

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/skorokithakis/soliloquy/internal/db"
)

func TestCommentPageHasNoContextWithoutParents(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	comment := createCommentDirect(t, server, thread, alice, nil, "top level", server.now())

	body := doRequest(t, server, http.MethodGet, commentPagePath(comment.PublicID), nil, cookie, "").Body.String()
	if !strings.Contains(body, `<div class="tbar">`) {
		t.Error("the comment page has no thread bar")
	}
	for _, unwanted := range []string{`class="ctx"`, `class="up"`, `#ctx`, `class="irt"`} {
		if strings.Contains(body, unwanted) {
			t.Errorf("comment page with no parents contains %q", unwanted)
		}
	}
}

func TestCommentPageShowsOneParent(t *testing.T) {
	server := newTestServer(t)
	base := time.Unix(1_700_000_000, 0).UTC()
	server.now = func() time.Time { return base.Add(2 * time.Hour) }

	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	cookie := signIn(t, server, bob)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", base)
	parent := createCommentDirect(t, server, thread, alice, nil, "parent **bold** text", base)
	focused := createCommentDirect(t, server, thread, bob, &parent.ID, "focused", base.Add(time.Minute))

	body := doRequest(t, server, http.MethodGet, commentPagePath(focused.PublicID), nil, cookie, "").Body.String()
	ctx := contextSection(t, body)

	for _, want := range []string{
		`<div class="p">`,
		`<a class="u" href="/u/alice">alice</a>`,
		`<a class="ts" href="/comments/` + parent.PublicID + `" title="` + parent.CreatedAt.UTC().Format("2006-01-02 15:04") + ` UTC">`,
		`<div class="pb md"><p>parent <strong>bold</strong> text</p>`,
	} {
		if !strings.Contains(ctx, want) {
			t.Errorf("context area does not contain %q", want)
		}
	}

	// The bar links to the context area and says how many parents there are.
	if !strings.Contains(body, `<a class="up" href="#ctx"`) {
		t.Error("the thread bar has no link to the context area")
	}
	if !strings.Contains(body, `>1 earlier comment</a>`) {
		t.Error("the thread bar link does not count one earlier comment")
	}

	// One divider: between the parent and the focused comment, and none
	// before the first parent.
	if got := strings.Count(ctx, `class="irt"`); got != 0 {
		t.Errorf("the context area of one parent has %d dividers, want 0", got)
	}
	if got := strings.Count(body, `class="irt"`); got != 1 {
		t.Errorf("the page has %d dividers, want 1", got)
	}

	for _, unwanted := range []string{"data-context-toggle", `class="ex"`, "ctx-fold", `class="p last"`, " hidden"} {
		if strings.Contains(ctx, unwanted) {
			t.Errorf("context area contains %q", unwanted)
		}
	}
	if strings.Contains(ctx, `id="c`+parent.PublicID+`"`) {
		t.Error("a parent is rendered as a comment with a comment id")
	}
}

func TestCommentPageShowsEveryParentInFull(t *testing.T) {
	server := newTestServer(t)
	base := time.Unix(1_700_000_000, 0).UTC()
	server.now = func() time.Time { return base.Add(2 * time.Hour) }

	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	cookie := signIn(t, server, bob)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", base)
	parents := createParentChain(t, server, thread, alice, 4, base)
	focused := createCommentDirect(t, server, thread, bob, &parents[len(parents)-1].ID, "focused", base.Add(10*time.Minute))

	body := doRequest(t, server, http.MethodGet, commentPagePath(focused.PublicID), nil, cookie, "").Body.String()
	ctx := contextSection(t, body)

	for i := range parents {
		want := fmt.Sprintf("<p>parent %d text</p>", i+1)
		if !strings.Contains(ctx, want) {
			t.Errorf("context area does not contain %q", want)
		}
	}
	if got := strings.Count(ctx, `class="p"`); got != len(parents) {
		t.Errorf("context area has %d parent rows, want %d", got, len(parents))
	}
	// A divider between each pair of parents and one before the focused
	// comment, but none before the first parent.
	if got := strings.Count(ctx, `class="irt"`); got != len(parents)-1 {
		t.Errorf("context area has %d dividers, want %d", got, len(parents)-1)
	}
	if got := strings.Count(body, `class="irt"`); got != len(parents) {
		t.Errorf("the page has %d dividers, want %d", got, len(parents))
	}
	if !strings.Contains(body, `>4 earlier comments</a>`) {
		t.Error("the thread bar link does not count four earlier comments")
	}
	if strings.Contains(ctx, "ctx-fold") || strings.Contains(ctx, "<details") {
		t.Error("four parents are rendered with a fold control")
	}

	assertParentOrder(t, ctx, parents)
}

func TestCommentPageShowsDeletedParent(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	cookie := signIn(t, server, bob)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	parent := createCommentDirect(t, server, thread, alice, nil, "deleted parent text", server.now())
	focused := createCommentDirect(t, server, thread, bob, &parent.ID, "focused", server.now().Add(time.Minute))
	if _, err := server.db.DeleteComment(context.Background(), parent.ID, server.now()); err != nil {
		t.Fatalf("delete parent: %v", err)
	}

	body := doRequest(t, server, http.MethodGet, commentPagePath(focused.PublicID), nil, cookie, "").Body.String()
	ctx := contextSection(t, body)

	for _, want := range []string{
		`<span class="gone">[deleted]</span>`,
		`href="/comments/` + parent.PublicID + `"`,
	} {
		if !strings.Contains(ctx, want) {
			t.Errorf("deleted parent row does not contain %q", want)
		}
	}
	for _, unwanted := range []string{`class="ex"`, `class="op"`, `class="pb"`, "deleted parent text"} {
		if strings.Contains(ctx, unwanted) {
			t.Errorf("deleted parent row contains %q", unwanted)
		}
	}
}

func TestCommentPageShowsDeletedAccountParent(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	cookie := signIn(t, server, bob)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	parent := createCommentDirect(t, server, thread, alice, nil, "parent text", server.now())
	focused := createCommentDirect(t, server, thread, bob, &parent.ID, "focused", server.now().Add(time.Minute))
	if _, err := server.db.ExecContext(context.Background(), `UPDATE users SET deleted_at = ? WHERE id = ?`, server.now().Unix(), alice.ID); err != nil {
		t.Fatalf("delete account: %v", err)
	}

	body := doRequest(t, server, http.MethodGet, commentPagePath(focused.PublicID), nil, cookie, "").Body.String()
	ctx := contextSection(t, body)

	// The account is gone but the comment is not, so the full text stays.
	for _, want := range []string{
		`<span class="gone">[deleted user]</span>`,
		`<div class="pb md"><p>parent text</p>`,
	} {
		if !strings.Contains(ctx, want) {
			t.Errorf("deleted-account parent row does not contain %q", want)
		}
	}
}

func TestCommentPageContextAsVisitor(t *testing.T) {
	server := newTestServer(t)
	setPublic(t, server, true)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	parent := createCommentDirect(t, server, thread, alice, nil, "parent text", server.now())
	focused := createCommentDirect(t, server, thread, bob, &parent.ID, "focused", server.now().Add(time.Minute))

	body := doRequest(t, server, http.MethodGet, commentPagePath(focused.PublicID), nil, nil, "").Body.String()
	ctx := contextSection(t, body)

	if !strings.Contains(ctx, `<span class="u">alice</span>`) {
		t.Error("visitor does not get a plain parent author name")
	}
	if strings.Contains(ctx, `href="/u/alice"`) {
		t.Error("visitor gets a profile link on a parent author")
	}
	if !strings.Contains(ctx, `href="/comments/`+parent.PublicID+`"`) {
		t.Error("visitor cannot open the parent's own page")
	}
	if !strings.Contains(ctx, `<div class="pb md"><p>parent text</p>`) {
		t.Error("visitor does not see the parent's full text")
	}
	if strings.Contains(ctx, "data-context-toggle") {
		t.Error("the parent row still has an expand control")
	}
}

// contextSection returns the rendered context area, so a test can check the
// parent rows without matching the focused comment or its replies.
func contextSection(t *testing.T, body string) string {
	t.Helper()

	const openTag = `<section class="ctx"`
	start := strings.Index(body, openTag)
	if start < 0 {
		t.Fatal("page has no context area")
	}
	end := strings.Index(body[start:], "</section>")
	if end < 0 {
		t.Fatal("context area is not closed")
	}
	return body[start : start+end+len("</section>")]
}

// createParentChain creates count nested parents and returns them oldest
// first, so the last one is the direct parent.
func createParentChain(t *testing.T, server *Server, thread *db.Thread, user *db.User, count int, base time.Time) []*db.Comment {
	t.Helper()

	chain := make([]*db.Comment, 0, count)
	var parentID *int64
	for i := 0; i < count; i++ {
		comment := createCommentDirect(t, server, thread, user, parentID, fmt.Sprintf("parent %d text", i+1), base.Add(time.Duration(i+1)*time.Minute))
		chain = append(chain, comment)
		id := comment.ID
		parentID = &id
	}
	return chain
}

func assertParentOrder(t *testing.T, ctx string, parents []*db.Comment) {
	t.Helper()

	previous := -1
	for i, parent := range parents {
		index := strings.Index(ctx, `/comments/`+parent.PublicID)
		if index < 0 {
			t.Fatalf("parent %d is missing from the context area", i+1)
		}
		if index <= previous {
			t.Errorf("parent %d is not after its parent", i+1)
		}
		previous = index
	}
}
