package web

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/skorokithakis/soliloquy/internal/db"
)

func feedURLWithToken(path, token string) string {
	return path + "?token=" + url.QueryEscape(token)
}

func parseFeed(t *testing.T, response *httptest.ResponseRecorder) rssFeed {
	t.Helper()

	var feed rssFeed
	if err := xml.Unmarshal(response.Body.Bytes(), &feed); err != nil {
		t.Fatalf("parse feed: %v\n%s", err, response.Body.String())
	}
	return feed
}

func TestThreadsFeedListsNewestLiveThreads(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	base := time.Unix(1_700_000_000, 0).UTC()

	createThreadDirect(t, server, alice, "Old thread", "", "old body", base)
	fresh := createThreadDirect(t, server, alice, "Fresh thread", "https://example.com/article", "fresh **body**", base.Add(time.Hour))
	deleted := createThreadDirect(t, server, alice, "Deleted thread", "", "gone", base.Add(2*time.Hour))
	if _, err := server.db.DeleteThread(context.Background(), deleted.ID, base.Add(3*time.Hour)); err != nil {
		t.Fatalf("delete thread: %v", err)
	}

	if err := server.db.SetFeedToken(context.Background(), alice.ID, "alice-token"); err != nil {
		t.Fatalf("set feed token: %v", err)
	}

	response := doRequest(t, server, http.MethodGet, feedURLWithToken("/feeds/threads", "alice-token"), nil, nil, "")
	if response.Code != http.StatusOK {
		t.Fatalf("feed status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Content-Type"); got != "application/rss+xml; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if cookies := response.Result().Cookies(); len(cookies) != 0 {
		t.Errorf("feed set cookies: %v", cookies)
	}

	feed := parseFeed(t, response)
	if feed.Version != "2.0" {
		t.Errorf("feed version = %q, want 2.0", feed.Version)
	}
	if feed.Channel.Title != "soliloquy threads" {
		t.Errorf("channel title = %q", feed.Channel.Title)
	}
	if len(feed.Channel.Items) != 2 {
		t.Fatalf("feed has %d items, want 2", len(feed.Channel.Items))
	}

	item := feed.Channel.Items[0]
	if item.Title != "Fresh thread" {
		t.Errorf("first item title = %q, want the newest thread", item.Title)
	}
	wantLink := testBaseURL + threadPath(fresh.PublicID)
	if item.Link != wantLink {
		t.Errorf("item link = %q, want %q", item.Link, wantLink)
	}
	if item.GUID.Value != wantLink || item.GUID.IsPermaLink != "true" {
		t.Errorf("item GUID = %+v, want the thread URL marked as a permalink", item.GUID)
	}
	if !strings.Contains(response.Body.String(), `xmlns:dc="http://purl.org/dc/elements/1.1/"`) {
		t.Error("feed does not declare the Dublin Core namespace")
	}
	if !strings.Contains(response.Body.String(), "<dc:creator>alice</dc:creator>") {
		t.Error("feed does not use dc:creator for the author")
	}
	if strings.Contains(response.Body.String(), "<author>") {
		t.Error("feed still uses the RSS author element")
	}
	if _, err := time.Parse(time.RFC1123Z, item.PubDate); err != nil {
		t.Errorf("pubDate %q is not RFC 1123: %v", item.PubDate, err)
	}
	if !strings.HasPrefix(item.Description, `<p><a href="https://example.com/article">`) {
		t.Errorf("description does not start with the external link: %q", item.Description)
	}
	if !strings.Contains(item.Description, "<strong>body</strong>") {
		t.Errorf("description does not contain the thread HTML: %q", item.Description)
	}

	for _, item := range feed.Channel.Items {
		if item.Title == "Deleted thread" {
			t.Error("feed contains a deleted thread")
		}
	}
}

func TestCommentsFeedListsNewestLiveComments(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	base := time.Unix(1_700_000_000, 0).UTC()

	thread := createThreadDirect(t, server, alice, "Thread title", "", "body", base)
	first := createCommentDirect(t, server, thread, alice, nil, "first comment", base.Add(time.Minute))
	newest := createCommentDirect(t, server, thread, bob, nil, "second **comment**", base.Add(2*time.Minute))

	deleted := createCommentDirect(t, server, thread, alice, nil, "deleted comment", base.Add(3*time.Minute))
	if _, err := server.db.DeleteComment(context.Background(), deleted.ID, base.Add(4*time.Minute)); err != nil {
		t.Fatalf("delete comment: %v", err)
	}

	deletedThread := createThreadDirect(t, server, alice, "Deleted thread", "", "", base)
	createCommentDirect(t, server, deletedThread, alice, nil, "comment on a deleted thread", base.Add(5*time.Minute))
	if _, err := server.db.DeleteThread(context.Background(), deletedThread.ID, base.Add(6*time.Minute)); err != nil {
		t.Fatalf("delete thread: %v", err)
	}

	if err := server.db.SetFeedToken(context.Background(), alice.ID, "alice-token"); err != nil {
		t.Fatalf("set feed token: %v", err)
	}

	response := doRequest(t, server, http.MethodGet, feedURLWithToken("/feeds/comments", "alice-token"), nil, nil, "")
	if response.Code != http.StatusOK {
		t.Fatalf("feed status = %d, want %d", response.Code, http.StatusOK)
	}

	feed := parseFeed(t, response)
	if feed.Channel.Title != "soliloquy comments" {
		t.Errorf("channel title = %q", feed.Channel.Title)
	}
	if len(feed.Channel.Items) != 2 {
		t.Fatalf("feed has %d items, want 2", len(feed.Channel.Items))
	}

	item := feed.Channel.Items[0]
	if item.Title != "bob on Thread title" {
		t.Errorf("first item title = %q, want the newest comment", item.Title)
	}
	wantLink := testBaseURL + threadPath(thread.PublicID) + "#c" + newest.PublicID
	if item.Link != wantLink || item.GUID.Value != wantLink {
		t.Errorf("item link = %q, guid = %q, want %q", item.Link, item.GUID.Value, wantLink)
	}
	if !strings.Contains(item.Description, "<strong>comment</strong>") {
		t.Errorf("description does not contain the comment HTML: %q", item.Description)
	}

	wantOldLink := testBaseURL + threadPath(thread.PublicID) + "#c" + first.PublicID
	if feed.Channel.Items[1].Link != wantOldLink {
		t.Errorf("second item link = %q, want %q", feed.Channel.Items[1].Link, wantOldLink)
	}
}

func TestFeedAccessOnPrivateForum(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	if err := server.db.SetFeedToken(context.Background(), alice.ID, "valid-token"); err != nil {
		t.Fatalf("set feed token: %v", err)
	}

	tests := map[string]struct {
		path string
		want int
	}{
		"threads with a valid token":  {feedURLWithToken("/feeds/threads", "valid-token"), http.StatusOK},
		"comments with a valid token": {feedURLWithToken("/feeds/comments", "valid-token"), http.StatusOK},
		"threads with a bad token":    {feedURLWithToken("/feeds/threads", "wrong-token"), http.StatusNotFound},
		"comments with a bad token":   {feedURLWithToken("/feeds/comments", "wrong-token"), http.StatusNotFound},
		"threads without a token":     {"/feeds/threads", http.StatusNotFound},
		"comments without a token":    {"/feeds/comments", http.StatusNotFound},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			response := doRequest(t, server, http.MethodGet, test.path, nil, nil, "")
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d", response.Code, test.want)
			}
			if test.want == http.StatusNotFound {
				if location := response.Header().Get("Location"); location != "" {
					t.Errorf("refused feed redirected to %q", location)
				}
			}
		})
	}

	t.Run("session cookie", func(t *testing.T) {
		cookie := signIn(t, server, alice)
		response := doRequest(t, server, http.MethodGet, "/feeds/threads", nil, cookie, "")
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
		}
	})
}

func TestFeedTokenSurvivesVisibilityChanges(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	if err := server.db.SetFeedToken(context.Background(), alice.ID, "stable-token"); err != nil {
		t.Fatalf("set feed token: %v", err)
	}
	paths := []string{
		feedURLWithToken("/feeds/threads", "stable-token"),
		feedURLWithToken("/feeds/comments", "stable-token"),
	}

	setPublic(t, server, false)
	for _, path := range paths {
		if response := doRequest(t, server, http.MethodGet, path, nil, nil, ""); response.Code != http.StatusOK {
			t.Errorf("private forum %s status = %d, want %d", path, response.Code, http.StatusOK)
		}
	}

	setPublic(t, server, true)
	for _, path := range paths {
		if response := doRequest(t, server, http.MethodGet, path, nil, nil, ""); response.Code != http.StatusOK {
			t.Errorf("public forum %s status = %d, want %d", path, response.Code, http.StatusOK)
		}
	}
	for _, path := range []string{"/feeds/threads", "/feeds/comments"} {
		if response := doRequest(t, server, http.MethodGet, path, nil, nil, ""); response.Code != http.StatusOK {
			t.Errorf("public forum without a token %s status = %d, want %d", path, response.Code, http.StatusOK)
		}
	}

	setPublic(t, server, false)
	for _, path := range paths {
		if response := doRequest(t, server, http.MethodGet, path, nil, nil, ""); response.Code != http.StatusOK {
			t.Errorf("forum made private again %s status = %d, want %d; the token stopped working", path, response.Code, http.StatusOK)
		}
	}
}

func TestFeedTokenStopsWorkingForInactiveUsers(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	for user, token := range map[*db.User]string{alice: "alice-token", bob: "bob-token"} {
		if err := server.db.SetFeedToken(context.Background(), user.ID, token); err != nil {
			t.Fatalf("set feed token: %v", err)
		}
	}

	if _, err := server.db.DisableUser(context.Background(), bob.ID, server.now()); err != nil {
		t.Fatalf("disable bob: %v", err)
	}
	if response := doRequest(t, server, http.MethodGet, feedURLWithToken("/feeds/threads", "bob-token"), nil, nil, ""); response.Code != http.StatusNotFound {
		t.Errorf("disabled user token status = %d, want %d", response.Code, http.StatusNotFound)
	}
	if response := doRequest(t, server, http.MethodGet, feedURLWithToken("/feeds/threads", "alice-token"), nil, nil, ""); response.Code != http.StatusOK {
		t.Errorf("active user token status = %d, want %d", response.Code, http.StatusOK)
	}

	if err := server.db.DeleteAccount(context.Background(), alice.ID, server.now()); err != nil {
		t.Fatalf("delete alice: %v", err)
	}
	if response := doRequest(t, server, http.MethodGet, feedURLWithToken("/feeds/threads", "alice-token"), nil, nil, ""); response.Code != http.StatusNotFound {
		t.Errorf("deleted user token status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestNotificationsFeedListsOnlyOwnNotifications(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	carol := createTestUser(t, server, "carol", false)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()

	thread := createThreadDirect(t, server, alice, "Thread title", "", "thread body", base)
	otherThread := createThreadDirect(t, server, alice, "Other thread", "", "other body", base.Add(time.Minute))
	reply := createCommentDirect(t, server, thread, bob, nil, "reply **body**", base.Add(2*time.Minute))
	topLevel := createCommentDirect(t, server, otherThread, carol, nil, "top level body", base.Add(3*time.Minute))

	// Newest first: mention, thread comment, reply.
	replyNotification := mustCreateNotification(t, server, alice.ID, bob.ID, db.NotificationReply, thread.ID, &reply.ID, base.Add(4*time.Minute))
	commentNotification := mustCreateNotification(t, server, alice.ID, carol.ID, db.NotificationThreadComment, otherThread.ID, &topLevel.ID, base.Add(5*time.Minute))
	mentionNotification := mustCreateNotification(t, server, alice.ID, bob.ID, db.NotificationMention, thread.ID, nil, base.Add(6*time.Minute))

	// Bob's own notification must not leak into Alice's feed.
	if _, err := server.db.CreateNotification(ctx, bob.ID, alice.ID, db.NotificationReply, thread.ID, &reply.ID, base.Add(7*time.Minute)); err != nil {
		t.Fatalf("CreateNotification for bob: %v", err)
	}

	for _, user := range []*db.User{alice, bob} {
		if err := server.db.SetFeedToken(ctx, user.ID, user.Username+"-token"); err != nil {
			t.Fatalf("set feed token: %v", err)
		}
	}

	response := doRequest(t, server, http.MethodGet, feedURLWithToken("/feeds/notifications", "alice-token"), nil, nil, "")
	if response.Code != http.StatusOK {
		t.Fatalf("feed status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Content-Type"); got != "application/rss+xml; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if cookies := response.Result().Cookies(); len(cookies) != 0 {
		t.Errorf("feed set cookies: %v", cookies)
	}

	feed := parseFeed(t, response)
	if feed.Channel.Title != "soliloquy notifications" {
		t.Errorf("channel title = %q", feed.Channel.Title)
	}
	if len(feed.Channel.Items) != 3 {
		t.Fatalf("feed has %d items, want 3:\n%s", len(feed.Channel.Items), response.Body.String())
	}

	want := []struct {
		notification *db.Notification
		title        string
		link         string
		body         string
	}{
		{
			notification: mentionNotification,
			title:        "bob mentioned you in Thread title",
			link:         testBaseURL + threadPath(thread.PublicID),
			body:         "thread body",
		},
		{
			notification: commentNotification,
			title:        "carol commented on your thread Other thread",
			link:         testBaseURL + commentAnchorPath(otherThread.PublicID, topLevel.PublicID),
			body:         "top level body",
		},
		{
			notification: replyNotification,
			title:        "bob replied to you in Thread title",
			link:         testBaseURL + commentAnchorPath(thread.PublicID, reply.PublicID),
			body:         "reply <strong>body</strong>",
		},
	}

	guids := make(map[string]struct{}, len(feed.Channel.Items))
	for i, item := range feed.Channel.Items {
		expected := want[i]
		if item.Title != expected.title {
			t.Errorf("item %d title = %q, want %q", i, item.Title, expected.title)
		}
		if item.Link != expected.link {
			t.Errorf("item %d link = %q, want %q", i, item.Link, expected.link)
		}
		if !strings.Contains(item.Description, expected.body) {
			t.Errorf("item %d description = %q, want it to contain %q", i, item.Description, expected.body)
		}
		if wantTime := rssTime(expected.notification.CreatedAt); item.PubDate != wantTime {
			t.Errorf("item %d pubDate = %q, want %q", i, item.PubDate, wantTime)
		}
		if item.GUID.IsPermaLink != "false" {
			t.Errorf("item %d GUID is a permalink: %+v", i, item.GUID)
		}
		if item.GUID.Value == item.Link {
			t.Errorf("item %d GUID is the item link %q, want a notification-specific GUID", i, item.GUID.Value)
		}
		if _, seen := guids[item.GUID.Value]; seen {
			t.Errorf("item %d repeats a GUID: %q", i, item.GUID.Value)
		}
		guids[item.GUID.Value] = struct{}{}
	}

	for _, want := range []string{"<dc:creator>bob</dc:creator>", "<dc:creator>carol</dc:creator>"} {
		if !strings.Contains(response.Body.String(), want) {
			t.Errorf("feed does not contain %q", want)
		}
	}
	if strings.Contains(response.Body.String(), "<dc:creator>alice</dc:creator>") {
		t.Error("feed contains the recipient as an actor")
	}

	// Reading the feed must not mark anything read.
	unread, err := server.db.CountUnreadNotifications(ctx, alice.ID)
	if err != nil {
		t.Fatalf("CountUnreadNotifications: %v", err)
	}
	if unread != 3 {
		t.Errorf("unread notifications after reading the feed = %d, want 3", unread)
	}
}

func TestNotificationsFeedAccessRequiresUser(t *testing.T) {
	for _, public := range []bool{false, true} {
		name := "private forum"
		if public {
			name = "public forum"
		}
		t.Run(name, func(t *testing.T) {
			server := newTestServer(t)
			alice := createTestUser(t, server, "alice", false)
			disabled := createTestUser(t, server, "disabled", false)
			deleted := createTestUser(t, server, "deleted", false)
			createThreadDirect(t, server, alice, "Thread", "", "body", server.now())

			ctx := context.Background()
			for user, token := range map[*db.User]string{
				alice:    "alice-token",
				disabled: "disabled-token",
				deleted:  "deleted-token",
			} {
				if err := server.db.SetFeedToken(ctx, user.ID, token); err != nil {
					t.Fatalf("set feed token: %v", err)
				}
			}
			if _, err := server.db.DisableUser(ctx, disabled.ID, server.now()); err != nil {
				t.Fatalf("disable user: %v", err)
			}
			if err := server.db.DeleteAccount(ctx, deleted.ID, server.now()); err != nil {
				t.Fatalf("delete account: %v", err)
			}

			setPublic(t, server, public)
			cookie := signIn(t, server, alice)

			tests := map[string]struct {
				path   string
				cookie *http.Cookie
				want   int
			}{
				"active user token":   {feedURLWithToken("/feeds/notifications", "alice-token"), nil, http.StatusOK},
				"session":             {"/feeds/notifications", cookie, http.StatusOK},
				"invalid token":       {feedURLWithToken("/feeds/notifications", "wrong-token"), nil, http.StatusNotFound},
				"disabled user token": {feedURLWithToken("/feeds/notifications", "disabled-token"), nil, http.StatusNotFound},
				"deleted user token":  {feedURLWithToken("/feeds/notifications", "deleted-token"), nil, http.StatusNotFound},
				"no token":            {"/feeds/notifications", nil, http.StatusNotFound},
			}
			for testName, test := range tests {
				t.Run(testName, func(t *testing.T) {
					response := doRequest(t, server, http.MethodGet, test.path, nil, test.cookie, "")
					if response.Code != test.want {
						t.Fatalf("status = %d, want %d", response.Code, test.want)
					}
					if test.want == http.StatusNotFound && response.Header().Get("Location") != "" {
						t.Errorf("refused feed redirected to %q", response.Header().Get("Location"))
					}
				})
			}

			// The public-forum rule still applies to the shared feeds.
			wantShared := http.StatusOK
			if !public {
				wantShared = http.StatusNotFound
			}
			for _, path := range []string{"/feeds/threads", "/feeds/comments"} {
				if response := doRequest(t, server, http.MethodGet, path, nil, nil, ""); response.Code != wantShared {
					t.Errorf("%s without a token status = %d, want %d", path, response.Code, wantShared)
				}
			}
		})
	}
}

func mustCreateNotification(t *testing.T, server *Server, recipientID, actorID int64, kind string, threadID int64, commentID *int64, at time.Time) *db.Notification {
	t.Helper()

	notification, err := server.db.CreateNotification(context.Background(), recipientID, actorID, kind, threadID, commentID, at)
	if err != nil {
		t.Fatalf("CreateNotification: %v", err)
	}
	return notification
}

func TestAccountFeedsSectionCreatesAndResetsToken(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)

	body := doRequest(t, server, http.MethodGet, "/account", nil, cookie, "").Body.String()
	for _, want := range []string{
		"feeds",
		"/feeds/threads?token=",
		"/feeds/comments?token=",
		"/feeds/notifications?token=",
		`action="/account/feeds/reset"`,
		`data-confirm="Make a new token? Your current feed links and API calls with the old token stop working."`,
		"make a new token",
		`data-copy="` + testBaseURL + `/feeds/threads?token=`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("account page does not contain %q", want)
		}
	}

	stored, err := server.db.UserByID(context.Background(), alice.ID)
	if err != nil || stored == nil {
		t.Fatalf("load user: %v", err)
	}
	token := stored.FeedToken
	if token == "" {
		t.Fatal("viewing the account page did not create a feed token")
	}

	// Viewing the page again must keep the same link.
	doRequest(t, server, http.MethodGet, "/account", nil, cookie, "")
	stored, err = server.db.UserByID(context.Background(), alice.ID)
	if err != nil || stored == nil || stored.FeedToken != token {
		t.Fatalf("feed token changed on a second view: %+v, %v", stored, err)
	}

	response := postForm(t, server, "/account/feeds/reset", nil, cookie)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/account" {
		t.Fatalf("reset status = %d, Location = %q", response.Code, response.Header().Get("Location"))
	}

	stored, err = server.db.UserByID(context.Background(), alice.ID)
	if err != nil || stored == nil {
		t.Fatalf("load user after reset: %v", err)
	}
	newToken := stored.FeedToken
	if newToken == "" || newToken == token {
		t.Fatalf("reset did not replace the token: %q -> %q", token, newToken)
	}

	if response := doRequest(t, server, http.MethodGet, feedURLWithToken("/feeds/threads", token), nil, nil, ""); response.Code != http.StatusNotFound {
		t.Errorf("old token status = %d, want %d", response.Code, http.StatusNotFound)
	}
	if response := doRequest(t, server, http.MethodGet, feedURLWithToken("/feeds/notifications", token), nil, nil, ""); response.Code != http.StatusNotFound {
		t.Errorf("old token notifications status = %d, want %d", response.Code, http.StatusNotFound)
	}
	if response := doRequest(t, server, http.MethodGet, feedURLWithToken("/feeds/threads", newToken), nil, nil, ""); response.Code != http.StatusOK {
		t.Errorf("new token status = %d, want %d", response.Code, http.StatusOK)
	}
	if response := doRequest(t, server, http.MethodGet, feedURLWithToken("/feeds/notifications", newToken), nil, nil, ""); response.Code != http.StatusOK {
		t.Errorf("new token notifications status = %d, want %d", response.Code, http.StatusOK)
	}

	// The reset is a POST, so the origin check applies to it.
	if response := doRequest(t, server, http.MethodPost, "/account/feeds/reset", nil, cookie, ""); response.Code != http.StatusForbidden {
		t.Errorf("reset without Origin status = %d, want %d", response.Code, http.StatusForbidden)
	}
	stored, err = server.db.UserByID(context.Background(), alice.ID)
	if err != nil || stored == nil || stored.FeedToken != newToken {
		t.Errorf("a rejected reset changed the token: %+v, %v", stored, err)
	}
}

func TestLayoutAdvertisesFeeds(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)

	// The account page creates the token, so later pages can advertise it.
	doRequest(t, server, http.MethodGet, "/account", nil, cookie, "")
	body := doRequest(t, server, http.MethodGet, "/", nil, cookie, "").Body.String()
	for _, want := range []string{
		`rel="alternate" type="application/rss+xml"`,
		"/feeds/threads?token=",
		"/feeds/comments?token=",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("logged-in page does not contain %q", want)
		}
	}

	setPublic(t, server, true)
	visitor := doRequest(t, server, http.MethodGet, "/", nil, nil, "").Body.String()
	for _, want := range []string{
		`href="` + testBaseURL + `/feeds/threads"`,
		`href="` + testBaseURL + `/feeds/comments"`,
	} {
		if !strings.Contains(visitor, want) {
			t.Errorf("public visitor page does not contain %q", want)
		}
	}
	if strings.Contains(visitor, "/feeds/threads?token=") {
		t.Error("public visitor page advertises a tokenized feed")
	}

	// The personal notifications feed stays on the account page.
	if strings.Contains(body, "/feeds/notifications") {
		t.Error("layout advertises the notifications feed")
	}
}
