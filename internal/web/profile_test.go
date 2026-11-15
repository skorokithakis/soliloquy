package web

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestProfilePageShowsMemberActivity(t *testing.T) {
	server := newTestServer(t)
	now := server.now()
	admin := createTestUser(t, server, "admin", true)
	bob := createInvitedTestUser(t, server, "bob", false, admin)
	cookie := signIn(t, server, admin)

	olderThread := createThreadDirect(t, server, bob, "Older thread", "", "older body", now.Add(-3*time.Hour))
	newerThread := createThreadDirect(t, server, bob, "Newer thread", "", "newer body", now.Add(-2*time.Hour))
	deletedThread := createThreadDirect(t, server, bob, "Deleted thread", "", "deleted body", now.Add(-90*time.Minute))
	createThreadDirect(t, server, admin, "Admin thread", "", "admin body", now.Add(-80*time.Minute))

	olderComment := createCommentDirect(t, server, olderThread, bob, nil, "first **formatted** comment", now.Add(-100*time.Minute))
	newerComment := createCommentDirect(t, server, newerThread, bob, nil, "second comment", now.Add(-70*time.Minute))
	deletedComment := createCommentDirect(t, server, newerThread, bob, nil, "deleted comment body", now.Add(-60*time.Minute))
	commentOnDeletedThread := createCommentDirect(t, server, deletedThread, bob, nil, "comment on a deleted thread", now.Add(-50*time.Minute))
	createCommentDirect(t, server, newerThread, admin, nil, "admin comment body", now.Add(-40*time.Minute))

	ctx := context.Background()
	if _, err := server.db.DeleteComment(ctx, deletedComment.ID, now); err != nil {
		t.Fatalf("delete comment: %v", err)
	}
	if _, err := server.db.DeleteThread(ctx, deletedThread.ID, now); err != nil {
		t.Fatalf("delete thread: %v", err)
	}

	body := doRequest(t, server, http.MethodGet, "/u/bob", nil, cookie, "").Body.String()

	for _, want := range []string{
		`<h1 class="pt"><span class="p">~/</span>users/bob</h1>`,
		"joined " + bob.CreatedAt.Format("2006-01-02"),
		`invited by <a href="/u/admin">admin</a>`,
		"Older thread",
		"Newer thread",
		`in <a href="/threads/` + newerThread.PublicID + `">Newer thread</a>`,
		`href="/threads/` + newerThread.PublicID + `#c` + newerComment.PublicID + `"`,
		`href="/threads/` + olderThread.PublicID + `#c` + olderComment.PublicID + `"`,
		"open comment ›",
		"first **formatted** comment",
		"second comment",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("profile page does not contain %q", want)
		}
	}

	for _, unwanted := range []string{
		"Deleted thread",
		"comment on a deleted thread",
		"deleted comment body",
		"Admin thread",
		"admin comment body",
		`#c` + commentOnDeletedThread.PublicID,
	} {
		if strings.Contains(body, unwanted) {
			t.Errorf("profile page contains excluded content %q", unwanted)
		}
	}

	// Threads and comments are both newest first.
	newerThreadIndex := strings.Index(body, "Newer thread")
	olderThreadIndex := strings.Index(body, "Older thread")
	if newerThreadIndex < 0 || olderThreadIndex < 0 || newerThreadIndex > olderThreadIndex {
		t.Error("profile threads are not newest first")
	}
	newerCommentIndex := strings.Index(body, newerComment.PublicID)
	olderCommentIndex := strings.Index(body, olderComment.PublicID)
	if newerCommentIndex < 0 || olderCommentIndex < 0 || newerCommentIndex > olderCommentIndex {
		t.Error("profile comments are not newest first")
	}
}

func TestProfilePageShowsFlagsAndInviter(t *testing.T) {
	server := newTestServer(t)
	root := createTestUser(t, server, "root", true)
	bob := createInvitedTestUser(t, server, "bob", false, root)
	carol := createInvitedTestUser(t, server, "carol", false, bob)
	dave := createInvitedTestUser(t, server, "dave", false, carol)
	eve := createInvitedTestUser(t, server, "eve", false, root)
	cookie := signIn(t, server, root)

	ctx := context.Background()
	if _, err := server.db.DisableUser(ctx, carol.ID, server.now()); err != nil {
		t.Fatalf("disable carol: %v", err)
	}
	if err := server.db.DeleteAccount(ctx, carol.ID, server.now()); err != nil {
		t.Fatalf("delete carol: %v", err)
	}
	if _, err := server.db.DisableUser(ctx, eve.ID, server.now()); err != nil {
		t.Fatalf("disable eve: %v", err)
	}

	rootBody := doRequest(t, server, http.MethodGet, "/u/root", nil, cookie, "").Body.String()
	if !strings.Contains(rootBody, `<span class="flag a">admin</span>`) {
		t.Error("root profile does not show the admin flag")
	}
	if strings.Contains(rootBody, "invited by") {
		t.Error("root profile shows an inviter, but the first admin has none")
	}

	bobBody := doRequest(t, server, http.MethodGet, "/u/bob", nil, cookie, "").Body.String()
	if !strings.Contains(bobBody, `invited by <a href="/u/root">root</a>`) {
		t.Error("bob profile does not link to his inviter")
	}
	if strings.Contains(bobBody, `class="flag a"`) {
		t.Error("bob profile shows an admin flag")
	}

	carolBody := doRequest(t, server, http.MethodGet, "/u/carol", nil, cookie, "")
	if carolBody.Code != http.StatusNotFound {
		t.Errorf("disabled and deleted carol profile status = %d, want %d", carolBody.Code, http.StatusNotFound)
	}

	eveResponse := doRequest(t, server, http.MethodGet, "/u/"+eve.Username, nil, cookie, "")
	if eveResponse.Code != http.StatusOK {
		t.Errorf("disabled eve profile status = %d, want %d", eveResponse.Code, http.StatusOK)
	}
	if !strings.Contains(eveResponse.Body.String(), `<span class="flag">disabled</span>`) {
		t.Error("disabled eve profile does not show the disabled flag")
	}

	daveBody := doRequest(t, server, http.MethodGet, "/u/"+dave.Username, nil, cookie, "").Body.String()
	if !strings.Contains(daveBody, "invited by [deleted user]") {
		t.Error("dave profile does not show the deleted inviter as plain text")
	}
	if strings.Contains(daveBody, "/u/carol") {
		t.Error("dave profile links to a deleted inviter")
	}
}

func TestProfilePageNotFoundAndCaseInsensitive(t *testing.T) {
	server := newTestServer(t)
	admin := createTestUser(t, server, "admin", true)
	bob := createTestUser(t, server, "bob", false)
	cookie := signIn(t, server, admin)

	response := doRequest(t, server, http.MethodGet, "/u/BOB", nil, cookie, "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "~/</span>users/bob") {
		t.Errorf("case-insensitive profile status = %d, body missing canonical username", response.Code)
	}

	if response := doRequest(t, server, http.MethodGet, "/u/nobody", nil, cookie, ""); response.Code != http.StatusNotFound {
		t.Errorf("unknown profile status = %d, want %d", response.Code, http.StatusNotFound)
	}

	if err := server.db.DeleteAccount(context.Background(), bob.ID, server.now()); err != nil {
		t.Fatalf("delete bob: %v", err)
	}
	if response := doRequest(t, server, http.MethodGet, "/u/bob", nil, cookie, ""); response.Code != http.StatusNotFound {
		t.Errorf("deleted profile status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestProfilePageRequiresLoginOnPublicForum(t *testing.T) {
	server := newTestServer(t)
	setPublic(t, server, true)
	createTestUser(t, server, "alice", false)

	response := doRequest(t, server, http.MethodGet, "/u/alice", nil, nil, "")
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login" {
		t.Errorf("status = %d, Location = %q; want a redirect to /login", response.Code, response.Header().Get("Location"))
	}
}

func TestAuthorNamesLinkToProfiles(t *testing.T) {
	server := newTestServer(t)
	now := server.now()
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	viewer := createTestUser(t, server, "viewer", false)
	cookie := signIn(t, server, viewer)

	thread := createThreadDirect(t, server, alice, "Findable thread", "", "profilelink thread body", now)
	createCommentDirect(t, server, thread, bob, nil, "profilelink comment body", now.Add(time.Minute))

	for _, path := range []string{"/", threadPath(thread.PublicID), "/search?q=profilelink"} {
		body := doRequest(t, server, http.MethodGet, path, nil, cookie, "").Body.String()
		if !strings.Contains(body, `href="/u/alice"`) {
			t.Errorf("%s does not link the thread author to their profile", path)
		}
	}
	threadBody := doRequest(t, server, http.MethodGet, threadPath(thread.PublicID), nil, cookie, "").Body.String()
	if !strings.Contains(threadBody, `<a class="u" href="/u/bob">bob</a>`) {
		t.Error("thread page does not link the comment author to their profile")
	}
	searchBody := doRequest(t, server, http.MethodGet, "/search?q=profilelink", nil, cookie, "").Body.String()
	if !strings.Contains(searchBody, `href="/u/bob"`) {
		t.Error("search page does not link the comment author to their profile")
	}

	// Visitors see the same names as plain text.
	setPublic(t, server, true)
	for _, path := range []string{"/", threadPath(thread.PublicID)} {
		body := doRequest(t, server, http.MethodGet, path, nil, nil, "").Body.String()
		if strings.Contains(body, "/u/") {
			t.Errorf("%s links a name for a visitor", path)
		}
		if !strings.Contains(body, "alice") {
			t.Errorf("%s does not show the author name", path)
		}
	}
}

func TestDeletedAuthorsAreNotLinkedToProfiles(t *testing.T) {
	server := newTestServer(t)
	now := server.now()
	alice := createTestUser(t, server, "alice", false)
	viewer := createTestUser(t, server, "viewer", false)
	cookie := signIn(t, server, viewer)

	thread := createThreadDirect(t, server, alice, "Findable thread", "", "profilelink thread body", now)
	createCommentDirect(t, server, thread, alice, nil, "profilelink comment body", now.Add(time.Minute))

	if err := server.db.DeleteAccount(context.Background(), alice.ID, server.now()); err != nil {
		t.Fatalf("delete alice: %v", err)
	}

	for _, path := range []string{"/", threadPath(thread.PublicID), "/search?q=profilelink"} {
		body := doRequest(t, server, http.MethodGet, path, nil, cookie, "").Body.String()
		if strings.Contains(body, "/u/alice") {
			t.Errorf("%s links a deleted author to their profile", path)
		}
		if !strings.Contains(body, "[deleted user]") {
			t.Errorf("%s does not show [deleted user]", path)
		}
	}
}

func TestProfileExcerpt(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "collapses whitespace", body: "first\n\n  second\tthird", want: "first second third"},
		{name: "keeps short text", body: "short comment", want: "short comment"},
		{name: "cuts at a word boundary", body: strings.Repeat("word ", 50), want: strings.Repeat("word ", 31) + "word…"},
	}
	for _, test := range tests {
		if got := profileExcerpt(test.body, 160); got != test.want {
			t.Errorf("%s: profileExcerpt = %q, want %q", test.name, got, test.want)
		}
	}
}
