package web

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/skorokithakis/soliloquy/internal/db"
	"github.com/skorokithakis/soliloquy/internal/hub"
)

func TestCommentNotifications(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	aliceCookie := signIn(t, server, alice)
	bobCookie := signIn(t, server, bob)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())

	response := postForm(t, server, threadPath(thread.PublicID)+"/comments", url.Values{
		"body": {"top level"},
	}, bobCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("comment status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	rootID := anchorPublicID(t, response.Header().Get("Location"))

	notifications := mustListNotifications(t, server, alice.ID)
	if len(notifications) != 1 {
		t.Fatalf("alice notifications = %d, want 1", len(notifications))
	}
	notification := notifications[0]
	if notification.Kind != db.NotificationThreadComment || notification.ActorID != bob.ID {
		t.Errorf("alice notification = %+v", notification)
	}
	if notification.CommentID == nil || notification.CommentPublicID == nil || *notification.CommentPublicID != rootID {
		t.Errorf("alice notification comment = %v, want %s", notification.CommentPublicID, rootID)
	}
	if got := mustListNotifications(t, server, bob.ID); len(got) != 0 {
		t.Errorf("bob got notifications for his own comment: %+v", got)
	}

	response = postForm(t, server, "/comments/"+rootID+"/reply", url.Values{
		"body": {"a reply"},
	}, aliceCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("reply status = %d, want %d", response.Code, http.StatusSeeOther)
	}

	notifications = mustListNotifications(t, server, bob.ID)
	if len(notifications) != 1 {
		t.Fatalf("bob notifications = %d, want 1", len(notifications))
	}
	notification = notifications[0]
	if notification.Kind != db.NotificationReply || notification.ActorID != alice.ID {
		t.Errorf("bob notification = %+v", notification)
	}
	if got := mustListNotifications(t, server, alice.ID); len(got) != 1 {
		t.Errorf("alice got a notification for replying on her own thread: %+v", got)
	}
}

func TestMentionNotificationsOnCreateAndEdit(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	carol := createTestUser(t, server, "carol", false)
	dave := createTestUser(t, server, "dave", false)
	disabled := createTestUser(t, server, "disabled", false)
	cookie := signIn(t, server, alice)
	ctx := context.Background()

	if _, err := server.db.ExecContext(ctx, "UPDATE users SET disabled_at = ? WHERE id = ?", server.now().Unix(), disabled.ID); err != nil {
		t.Fatalf("disable user: %v", err)
	}

	response := postForm(t, server, "/threads", url.Values{
		"title": {"Thread"},
		"body":  {"Hello @bob, @disabled and @alice"},
	}, cookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("create thread status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	threadPublicID := strings.TrimPrefix(response.Header().Get("Location"), "/threads/")
	if threadPublicID == "" {
		t.Fatal("create thread Location has no thread public ID")
	}

	response = postForm(t, server, threadPath(threadPublicID)+"/edit", url.Values{
		"title": {"Thread"},
		"body":  {"Hello @bob, @disabled and @alice, welcome @carol"},
	}, cookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("edit thread status = %d, want %d", response.Code, http.StatusSeeOther)
	}

	if got := mustListNotifications(t, server, bob.ID); len(got) != 1 || got[0].Kind != db.NotificationMention || got[0].CommentID != nil {
		t.Errorf("bob thread mention notifications = %+v, want one mention without a comment", got)
	}
	if got := mustListNotifications(t, server, carol.ID); len(got) != 1 || got[0].Kind != db.NotificationMention {
		t.Errorf("carol notifications = %+v, want one mention", got)
	}
	if got := mustListNotifications(t, server, disabled.ID); len(got) != 0 {
		t.Errorf("disabled user got notifications: %+v", got)
	}
	if got := mustListNotifications(t, server, alice.ID); len(got) != 0 {
		t.Errorf("actor got notifications: %+v", got)
	}

	// A top-level comment on alice's own thread notifies mentioned users but
	// not alice as the thread's author.
	response = postForm(t, server, threadPath(threadPublicID)+"/comments", url.Values{
		"body": {"@dave and @bob"},
	}, cookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("comment status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	commentID := anchorPublicID(t, response.Header().Get("Location"))

	if got := mustListNotifications(t, server, dave.ID); len(got) != 1 || got[0].CommentPublicID == nil || *got[0].CommentPublicID != commentID {
		t.Errorf("dave notifications = %+v, want one mention on comment %s", got, commentID)
	}
	if got := mustListNotifications(t, server, bob.ID); len(got) != 2 {
		t.Errorf("bob notifications = %d, want the thread mention plus the comment mention", len(got))
	}

	// Editing without adding a name creates nothing; a newly added name does.
	response = postForm(t, server, commentAction(commentID, "edit"), url.Values{
		"body": {"@dave and @bob again"},
	}, cookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("edit comment status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	if got := mustListNotifications(t, server, dave.ID); len(got) != 1 {
		t.Errorf("dave got a duplicate mention: %d notifications", len(got))
	}

	response = postForm(t, server, commentAction(commentID, "edit"), url.Values{
		"body": {"@dave and @bob and @carol"},
	}, cookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("second edit status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	if got := mustListNotifications(t, server, carol.ID); len(got) != 2 {
		t.Errorf("carol notifications = %d, want the thread mention plus the comment edit mention", len(got))
	}
}

func TestFollowedNotifications(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	carol := createTestUser(t, server, "carol", false)
	dave := createTestUser(t, server, "dave", false)
	bobCookie := signIn(t, server, bob)
	carolCookie := signIn(t, server, carol)
	daveCookie := signIn(t, server, dave)
	ctx := context.Background()

	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	// The thread author follows too: a top-level comment must still produce
	// thread_comment, and a nested one must produce followed.
	for _, user := range []*db.User{alice, bob, carol, dave} {
		if err := server.db.FollowThread(ctx, user.ID, thread.ID, server.now()); err != nil {
			t.Fatalf("follow as %s: %v", user.Username, err)
		}
	}

	// A top-level comment notifies the thread author (who also follows), the
	// mentioned follower, and the other follower.
	response := postForm(t, server, threadPath(thread.PublicID)+"/comments", url.Values{
		"body": {"hello @carol"},
	}, bobCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("comment status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	rootID := anchorPublicID(t, response.Header().Get("Location"))

	assertNotificationKinds(t, mustListNotifications(t, server, alice.ID), db.NotificationThreadComment)
	assertNotificationKinds(t, mustListNotifications(t, server, carol.ID), db.NotificationMention)
	assertNotificationKinds(t, mustListNotifications(t, server, dave.ID), db.NotificationFollowed)
	assertNotificationKinds(t, mustListNotifications(t, server, bob.ID))
	for _, user := range []*db.User{alice, carol, dave} {
		for _, notification := range mustListNotifications(t, server, user.ID) {
			if notification.CommentPublicID == nil || *notification.CommentPublicID != rootID {
				t.Errorf("%s notification comment = %v, want %s", user.Username, notification.CommentPublicID, rootID)
			}
		}
	}

	// A nested reply notifies its parent's author, and every other follower.
	response = postForm(t, server, "/comments/"+rootID+"/reply", url.Values{
		"body": {"nested"},
	}, daveCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("reply status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	replyID := anchorPublicID(t, response.Header().Get("Location"))

	assertNotificationKinds(t, mustListNotifications(t, server, bob.ID), db.NotificationReply)
	assertNotificationKinds(t, mustListNotifications(t, server, alice.ID),
		db.NotificationFollowed, db.NotificationThreadComment)
	assertNotificationKinds(t, mustListNotifications(t, server, carol.ID),
		db.NotificationFollowed, db.NotificationMention)
	assertNotificationKinds(t, mustListNotifications(t, server, dave.ID), db.NotificationFollowed)
	// The parent author who follows gets only the reply for this comment.
	for _, notification := range mustListNotifications(t, server, bob.ID) {
		if notification.CommentPublicID == nil || *notification.CommentPublicID != replyID {
			t.Errorf("bob reply notification comment = %v, want %s", notification.CommentPublicID, replyID)
		}
	}

	// Following applies at every depth: a reply to the reply notifies the new
	// parent author and the followers, not the commenter.
	response = postForm(t, server, "/comments/"+replyID+"/reply", url.Values{
		"body": {"deeper"},
	}, carolCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("deeper reply status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	deeperID := anchorPublicID(t, response.Header().Get("Location"))

	assertNotificationKinds(t, mustListNotifications(t, server, dave.ID),
		db.NotificationReply, db.NotificationFollowed)
	assertNotificationKinds(t, mustListNotifications(t, server, alice.ID),
		db.NotificationFollowed, db.NotificationFollowed, db.NotificationThreadComment)
	assertNotificationKinds(t, mustListNotifications(t, server, bob.ID),
		db.NotificationFollowed, db.NotificationReply)
	assertNotificationKinds(t, mustListNotifications(t, server, carol.ID),
		db.NotificationFollowed, db.NotificationMention)
	for _, user := range []*db.User{dave, alice, bob} {
		notifications := mustListNotifications(t, server, user.ID)
		if notifications[0].CommentPublicID == nil || *notifications[0].CommentPublicID != deeperID {
			t.Errorf("%s newest notification comment = %v, want %s", user.Username, notifications[0].CommentPublicID, deeperID)
		}
	}

	// The page and the shared one-line text use the followed wording.
	body := doRequest(t, server, http.MethodGet, "/notifications", nil, daveCookie, "").Body.String()
	for _, want := range []string{
		`class="kind">followed</span>`,
		`class="kind-inline">followed</span>`,
		"commented in “Thread”",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("notifications page does not contain %q", want)
		}
	}
}

func TestFollowedNotificationWording(t *testing.T) {
	notification := db.Notification{
		Kind:          db.NotificationFollowed,
		ActorUsername: "bob",
		ThreadTitle:   "Thread",
	}
	if got := notificationText(&notification); got != "bob commented in Thread" {
		t.Errorf("notificationText = %q, want %q", got, "bob commented in Thread")
	}
	if got := notificationFeedTitle(notification); got != "bob commented in Thread" {
		t.Errorf("notificationFeedTitle = %q, want %q", got, "bob commented in Thread")
	}
	if got := notificationKindLabel(db.NotificationFollowed); got != "followed" {
		t.Errorf("notificationKindLabel = %q, want followed", got)
	}
}

func TestDisabledFollowerGetsNoFollowedNotification(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	carol := createTestUser(t, server, "carol", false)
	carolCookie := signIn(t, server, carol)
	ctx := context.Background()

	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	if err := server.db.FollowThread(ctx, bob.ID, thread.ID, server.now()); err != nil {
		t.Fatalf("FollowThread: %v", err)
	}
	if _, err := server.db.DisableUser(ctx, bob.ID, server.now()); err != nil {
		t.Fatalf("DisableUser: %v", err)
	}

	response := postForm(t, server, threadPath(thread.PublicID)+"/comments", url.Values{
		"body": {"a comment"},
	}, carolCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("comment status = %d, want %d", response.Code, http.StatusSeeOther)
	}

	assertNotificationKinds(t, mustListNotifications(t, server, alice.ID), db.NotificationThreadComment)
	assertNotificationKinds(t, mustListNotifications(t, server, bob.ID))
}

func TestNotifyNewCommentReturnsFollowers(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	carol := createTestUser(t, server, "carol", false)
	ctx := context.Background()

	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	if err := server.db.FollowThread(ctx, bob.ID, thread.ID, server.now()); err != nil {
		t.Fatalf("follow as bob: %v", err)
	}
	if err := server.db.FollowThread(ctx, carol.ID, thread.ID, server.now()); err != nil {
		t.Fatalf("follow as carol: %v", err)
	}

	comment := createCommentDirect(t, server, thread, bob, nil, "hello", server.now())
	recipients, err := server.notifyNewComment(ctx, thread.ID, thread.AuthorID, comment, nil)
	if err != nil {
		t.Fatalf("notifyNewComment: %v", err)
	}

	// The thread author and carol changed; bob wrote the comment.
	if len(recipients) != 2 {
		t.Fatalf("recipients = %v, want alice and carol", recipients)
	}
	for _, user := range []*db.User{alice, carol} {
		if !slices.Contains(recipients, user.ID) {
			t.Errorf("recipients = %v, missing %s", recipients, user.Username)
		}
	}
	if slices.Contains(recipients, bob.ID) {
		t.Errorf("recipients = %v include the comment's author", recipients)
	}
}

func TestNotificationsPageMarksReadAndShowsBadge(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	cookie := signIn(t, server, alice)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	comment := createCommentDirect(t, server, thread, bob, nil, "hello", server.now().Add(time.Minute))

	if _, err := server.db.CreateNotification(context.Background(), alice.ID, bob.ID, db.NotificationReply, thread.ID, &comment.ID, server.now()); err != nil {
		t.Fatalf("CreateNotification: %v", err)
	}

	index := doRequest(t, server, http.MethodGet, "/", nil, cookie, "")
	if !strings.Contains(index.Body.String(), `class="badge"`) {
		t.Error("header does not show the unread badge")
	}

	response := doRequest(t, server, http.MethodGet, "/notifications", nil, cookie, "")
	if response.Code != http.StatusOK {
		t.Fatalf("notifications page status = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	for _, want := range []string{
		`class="u">bob</span>`,
		"replied to your comment in",
		"Thread",
		`class="snip">hello</div>`,
		"unread · 1",
		commentPagePath(comment.PublicID),
	} {
		if !strings.Contains(body, want) {
			t.Errorf("notifications page does not contain %q", want)
		}
	}

	count, err := server.db.CountUnreadNotifications(context.Background(), alice.ID)
	if err != nil || count != 0 {
		t.Fatalf("unread count after visiting notifications = %d, %v; want 0, nil", count, err)
	}

	index = doRequest(t, server, http.MethodGet, "/", nil, cookie, "")
	if strings.Contains(index.Body.String(), `class="badge"`) {
		t.Error("header still shows a badge after notifications were read")
	}
}

func TestNotificationsPageSections(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	cookie := signIn(t, server, alice)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	comment := createCommentDirect(t, server, thread, bob, nil, "an older comment", server.now().Add(time.Minute))
	ctx := context.Background()

	if _, err := server.db.CreateNotification(ctx, alice.ID, bob.ID, db.NotificationReply, thread.ID, &comment.ID, server.now().Add(2*time.Minute)); err != nil {
		t.Fatalf("CreateNotification: %v", err)
	}
	if err := server.db.MarkNotificationsRead(ctx, alice.ID, server.now().Add(3*time.Minute)); err != nil {
		t.Fatalf("MarkNotificationsRead: %v", err)
	}
	if _, err := server.db.CreateNotification(ctx, alice.ID, bob.ID, db.NotificationMention, thread.ID, &comment.ID, server.now().Add(4*time.Minute)); err != nil {
		t.Fatalf("CreateNotification: %v", err)
	}

	body := doRequest(t, server, http.MethodGet, "/notifications", nil, cookie, "").Body.String()
	for _, want := range []string{
		"unread · 1",
		`earlier-label">earlier</div>`,
		"mentioned you in “Thread”",
		"replied to your comment in “Thread”",
		"an older comment",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("notifications page does not contain %q", want)
		}
	}
}

func TestNotificationsPagination(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	cookie := signIn(t, server, alice)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	comment := createCommentDirect(t, server, thread, bob, nil, "comment", server.now())
	ctx := context.Background()
	base := server.now()

	for i := 0; i < notificationsPerPage+1; i++ {
		if _, err := server.db.CreateNotification(ctx, alice.ID, bob.ID, db.NotificationReply, thread.ID, &comment.ID, base.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatalf("CreateNotification %d: %v", i, err)
		}
	}

	first := doRequest(t, server, http.MethodGet, "/notifications", nil, cookie, "")
	if first.Code != http.StatusOK {
		t.Fatalf("first page status = %d, want %d", first.Code, http.StatusOK)
	}
	body := first.Body.String()
	if count := strings.Count(body, `<li class="n`); count != notificationsPerPage {
		t.Errorf("first page rows = %d, want %d", count, notificationsPerPage)
	}
	if !strings.Contains(body, `href="/notifications?page=2"`) {
		t.Error("first page has no load older link")
	}
	if !strings.Contains(body, "unread · 30") {
		t.Error("first page does not show the unread count")
	}

	second := doRequest(t, server, http.MethodGet, "/notifications?page=2", nil, cookie, "")
	if second.Code != http.StatusOK {
		t.Fatalf("second page status = %d, want %d", second.Code, http.StatusOK)
	}
	body = second.Body.String()
	if count := strings.Count(body, `<li class="n`); count != 1 {
		t.Errorf("second page rows = %d, want 1", count)
	}
	if strings.Contains(body, "/notifications?page=3") {
		t.Error("second page has a load older link")
	}
	if !strings.Contains(body, `earlier-label">earlier</div>`) {
		t.Error("second page does not show the earlier section")
	}
}

func TestNotificationSnippet(t *testing.T) {
	tests := []struct {
		html string
		max  int
		want string
	}{
		{html: "<p>one</p>\n<p>two <strong>three</strong></p>", max: 100, want: "one two three"},
		{html: "<p>&lt;b&gt;not a tag&lt;/b&gt;</p>", max: 100, want: "<b>not a tag</b>"},
		{html: "<p>" + strings.Repeat("x", 10) + "</p>", max: 5, want: "xxxxx…"},
		{html: "\n<p> </p>", max: 10, want: ""},
	}
	for _, test := range tests {
		if got := notificationSnippet(test.html, test.max); got != test.want {
			t.Errorf("notificationSnippet(%q) = %q, want %q", test.html, got, test.want)
		}
	}
}

func TestMentionDoesNotDuplicatePrimaryNotification(t *testing.T) {
	t.Run("reply", func(t *testing.T) {
		server := newTestServer(t)
		alice := createTestUser(t, server, "alice", false)
		bob := createTestUser(t, server, "bob", false)
		aliceCookie := signIn(t, server, alice)
		thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
		root := createCommentDirect(t, server, thread, bob, nil, "root", server.now())

		response := postForm(t, server, "/comments/"+root.PublicID+"/reply", url.Values{
			"body": {"@bob thanks"},
		}, aliceCookie)
		if response.Code != http.StatusSeeOther {
			t.Fatalf("reply status = %d, want %d", response.Code, http.StatusSeeOther)
		}

		notifications := mustListNotifications(t, server, bob.ID)
		if len(notifications) != 1 {
			t.Fatalf("bob notifications = %d, want one reply notification", len(notifications))
		}
		if notifications[0].Kind != db.NotificationReply {
			t.Errorf("bob notification kind = %q, want %q", notifications[0].Kind, db.NotificationReply)
		}
	})

	t.Run("thread comment", func(t *testing.T) {
		server := newTestServer(t)
		alice := createTestUser(t, server, "alice", false)
		bob := createTestUser(t, server, "bob", false)
		bobCookie := signIn(t, server, bob)
		thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())

		response := postForm(t, server, threadPath(thread.PublicID)+"/comments", url.Values{
			"body": {"@alice hello"},
		}, bobCookie)
		if response.Code != http.StatusSeeOther {
			t.Fatalf("comment status = %d, want %d", response.Code, http.StatusSeeOther)
		}

		notifications := mustListNotifications(t, server, alice.ID)
		if len(notifications) != 1 {
			t.Fatalf("alice notifications = %d, want one thread comment notification", len(notifications))
		}
		if notifications[0].Kind != db.NotificationThreadComment {
			t.Errorf("alice notification kind = %q, want %q", notifications[0].Kind, db.NotificationThreadComment)
		}
	})
}

func TestEventsRejectsDeletedOrMissingThread(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())

	if _, err := server.db.DeleteThread(context.Background(), thread.ID, server.now()); err != nil {
		t.Fatalf("delete thread: %v", err)
	}

	for _, threadPublicID := range []string{thread.PublicID, "zzzzzzzz"} {
		path := "/events?thread_id=" + threadPublicID
		response := doRequest(t, server, http.MethodGet, path, nil, cookie, "")
		if response.Code != http.StatusNotFound {
			t.Errorf("%s status = %d, want %d", path, response.Code, http.StatusNotFound)
		}
	}
}

func TestNewCommentPublishesLiveUpdates(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	bobCookie := signIn(t, server, bob)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())

	comments, unsubscribeComments := server.hub.Subscribe(hub.ThreadTopic(thread.ID))
	defer unsubscribeComments()
	badges, unsubscribeBadges := server.hub.Subscribe(hub.UserTopic(alice.ID))
	defer unsubscribeBadges()

	response := postForm(t, server, threadPath(thread.PublicID)+"/comments", url.Values{
		"body": {"**live** comment"},
	}, bobCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("comment status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	commentID := anchorPublicID(t, response.Header().Get("Location"))

	select {
	case event := <-comments:
		if event.Name != "comment" {
			t.Fatalf("comment event name = %q, want comment", event.Name)
		}
		var payload commentEvent
		if err := json.Unmarshal(event.Data, &payload); err != nil {
			t.Fatalf("decode comment event: %v", err)
		}
		if payload.CommentID != commentID || payload.ThreadID != thread.PublicID || payload.ParentID != nil {
			t.Errorf("comment event = %+v", payload)
		}
		if !strings.Contains(payload.HTML, "<strong>live</strong> comment") {
			t.Errorf("comment event HTML = %q", payload.HTML)
		}
		// A new comment has no votes, so its vote button is safe for everyone.
		if !strings.Contains(payload.HTML, `action="/comments/`+commentID+`/vote"`) {
			t.Errorf("live comment HTML has no vote form: %s", payload.HTML)
		}
		for _, unwanted := range []string{"/edit", "/delete"} {
			if strings.Contains(payload.HTML, unwanted) {
				t.Errorf("live comment HTML contains viewer-specific control %q: %s", unwanted, payload.HTML)
			}
		}
		if payload.CommentCountText != "1 comment" {
			t.Errorf("comment event count = %q, want 1 comment", payload.CommentCountText)
		}
	case <-time.After(time.Second):
		t.Fatal("no comment event was published")
	}

	select {
	case event := <-badges:
		if event.Name != "notification" {
			t.Fatalf("first user event name = %q, want notification", event.Name)
		}
		var payload notificationEvent
		if err := json.Unmarshal(event.Data, &payload); err != nil {
			t.Fatalf("decode notification event: %v", err)
		}
		if payload.Text != "bob commented on your thread Thread" {
			t.Errorf("notification text = %q", payload.Text)
		}
		if payload.URL != commentPagePath(commentID) {
			t.Errorf("notification URL = %q", payload.URL)
		}
	case <-time.After(time.Second):
		t.Fatal("no notification event was published")
	}

	select {
	case event := <-badges:
		if event.Name != "badge" {
			t.Fatalf("badge event name = %q, want badge", event.Name)
		}
		var payload badgeEvent
		if err := json.Unmarshal(event.Data, &payload); err != nil {
			t.Fatalf("decode badge event: %v", err)
		}
		if payload.Count != 1 {
			t.Errorf("badge count = %d, want 1", payload.Count)
		}
	case <-time.After(time.Second):
		t.Fatal("no badge event was published")
	}
}

func TestCommentEditPublishesLiveUpdate(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	aliceCookie := signIn(t, server, alice)
	bobCookie := signIn(t, server, bob)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	comment := createCommentDirect(t, server, thread, bob, nil, "original", server.now())

	updates, unsubscribe := server.hub.Subscribe(hub.ThreadTopic(thread.ID))
	defer unsubscribe()

	response := postForm(t, server, commentAction(comment.PublicID, "edit"), url.Values{
		"body": {"edited **text**"},
	}, bobCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("edit status = %d, want %d", response.Code, http.StatusSeeOther)
	}

	select {
	case event := <-updates:
		if event.Name != "comment_edit" {
			t.Fatalf("event name = %q, want comment_edit", event.Name)
		}
		var payload commentEditEvent
		if err := json.Unmarshal(event.Data, &payload); err != nil {
			t.Fatalf("decode comment edit event: %v", err)
		}
		if payload.CommentID != comment.PublicID {
			t.Errorf("comment edit id = %q, want %q", payload.CommentID, comment.PublicID)
		}
		if !strings.Contains(payload.HTML, "edited <strong>text</strong>") {
			t.Errorf("comment edit HTML = %q", payload.HTML)
		}
		// The event carries only the body; the controls differ per viewer.
		for _, unwanted := range []string{"/vote", "/edit", "/delete", `class="ch"`} {
			if strings.Contains(payload.HTML, unwanted) {
				t.Errorf("comment edit HTML contains %q: %s", unwanted, payload.HTML)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("no comment edit event was published")
	}

	response = postForm(t, server, commentAction(comment.PublicID, "edit"), url.Values{"body": {"hijacked"}}, aliceCookie)
	if response.Code != http.StatusForbidden {
		t.Fatalf("edit by another user: status = %d, want %d", response.Code, http.StatusForbidden)
	}
	response = postForm(t, server, commentAction(comment.PublicID, "edit"), url.Values{"body": {"   "}}, bobCookie)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid edit status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	assertNoThreadEvent(t, updates)
}

func TestCommentDeletePublishesLiveUpdate(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	admin := createTestUser(t, server, "admin", true)
	aliceCookie := signIn(t, server, alice)
	bobCookie := signIn(t, server, bob)
	adminCookie := signIn(t, server, admin)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	comment := createCommentDirect(t, server, thread, bob, nil, "original **body**", server.now())
	reply := createCommentDirect(t, server, thread, bob, &comment.ID, "reply text", server.now())

	updates, unsubscribe := server.hub.Subscribe(hub.ThreadTopic(thread.ID))
	defer unsubscribe()

	response := postForm(t, server, commentAction(comment.PublicID, "delete"), nil, aliceCookie)
	if response.Code != http.StatusForbidden {
		t.Fatalf("delete by another user: status = %d, want %d", response.Code, http.StatusForbidden)
	}
	assertNoThreadEvent(t, updates)

	response = postForm(t, server, commentAction(comment.PublicID, "delete"), nil, bobCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("delete status = %d, want %d", response.Code, http.StatusSeeOther)
	}

	select {
	case event := <-updates:
		if event.Name != "comment_delete" {
			t.Fatalf("event name = %q, want comment_delete", event.Name)
		}
		var payload commentDeleteEvent
		if err := json.Unmarshal(event.Data, &payload); err != nil {
			t.Fatalf("decode comment delete event: %v", err)
		}
		if payload.CommentID != comment.PublicID {
			t.Errorf("comment delete id = %q, want %q", payload.CommentID, comment.PublicID)
		}
		for _, want := range []string{`class="gone"`, "[deleted]"} {
			if !strings.Contains(payload.HTML, want) {
				t.Errorf("comment delete HTML does not contain %q: %s", want, payload.HTML)
			}
		}
		// The deleted rendering has no actions and no replies; the page keeps
		// the replies it already shows. The reply still counts.
		for _, unwanted := range []string{"original", "reply text", "/vote", "/reply", "/edit", "/delete", `class="ca"`} {
			if strings.Contains(payload.HTML, unwanted) {
				t.Errorf("comment delete HTML contains %q: %s", unwanted, payload.HTML)
			}
		}
		if payload.CommentCountText != "1 comment" {
			t.Errorf("comment delete count = %q, want 1 comment", payload.CommentCountText)
		}
	case <-time.After(time.Second):
		t.Fatal("no comment delete event was published")
	}

	// A deleted reply keeps the parent link a page load shows for it.
	response = postForm(t, server, commentAction(reply.PublicID, "delete"), nil, bobCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("reply delete status = %d, want %d", response.Code, http.StatusSeeOther)
	}

	select {
	case event := <-updates:
		var payload commentDeleteEvent
		if err := json.Unmarshal(event.Data, &payload); err != nil {
			t.Fatalf("decode comment delete event: %v", err)
		}
		parentLink := `<a class="par" href="#c` + comment.PublicID + `">parent</a>`
		if payload.CommentID != reply.PublicID || !strings.Contains(payload.HTML, parentLink) {
			t.Errorf("deleted reply = %+v, HTML = %s", payload, payload.HTML)
		}
		if payload.CommentCountText != "0 comments" {
			t.Errorf("reply delete count = %q, want 0 comments", payload.CommentCountText)
		}
	case <-time.After(time.Second):
		t.Fatal("no reply delete event was published")
	}

	// The reply is gone, so a second delete fails and publishes nothing.
	response = postForm(t, server, commentAction(reply.PublicID, "delete"), nil, adminCookie)
	if response.Code != http.StatusNotFound {
		t.Fatalf("second delete status = %d, want %d", response.Code, http.StatusNotFound)
	}
	assertNoThreadEvent(t, updates)
}

func TestThreadEditPublishesLiveUpdate(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	aliceCookie := signIn(t, server, alice)
	bobCookie := signIn(t, server, bob)
	thread := createThreadDirect(t, server, alice, "Original", "", "old body", server.now())
	editPath := threadPath(thread.PublicID) + "/edit"

	updates, unsubscribe := server.hub.Subscribe(hub.ThreadTopic(thread.ID))
	defer unsubscribe()

	response := postForm(t, server, editPath, url.Values{
		"title": {"Edited title"},
		"url":   {"https://example.com/post"},
		"body":  {"new **body**"},
	}, aliceCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("edit status = %d, want %d", response.Code, http.StatusSeeOther)
	}

	select {
	case event := <-updates:
		if event.Name != "thread_edit" {
			t.Fatalf("event name = %q, want thread_edit", event.Name)
		}
		var payload threadUpdateEvent
		if err := json.Unmarshal(event.Data, &payload); err != nil {
			t.Fatalf("decode thread edit event: %v", err)
		}
		for _, want := range []string{"Edited title", `href="https://example.com/post"`, `class="dom"`, "example.com"} {
			if !strings.Contains(payload.TitleHTML, want) {
				t.Errorf("thread edit title HTML does not contain %q: %s", want, payload.TitleHTML)
			}
		}
		if !strings.Contains(payload.BodyHTML, "<strong>body</strong>") {
			t.Errorf("thread edit body HTML = %q", payload.BodyHTML)
		}
	case <-time.After(time.Second):
		t.Fatal("no thread edit event was published")
	}

	// A thread with a URL and no body has no body block to show.
	response = postForm(t, server, editPath, url.Values{
		"title": {"URL only"},
		"url":   {"https://example.com/"},
		"body":  {""},
	}, aliceCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("bodyless edit status = %d, want %d", response.Code, http.StatusSeeOther)
	}

	select {
	case event := <-updates:
		if event.Name != "thread_edit" {
			t.Fatalf("second event name = %q, want thread_edit", event.Name)
		}
		var payload threadUpdateEvent
		if err := json.Unmarshal(event.Data, &payload); err != nil {
			t.Fatalf("decode thread edit event: %v", err)
		}
		if payload.BodyHTML != "" {
			t.Errorf("bodyless thread edit body HTML = %q, want empty", payload.BodyHTML)
		}
	case <-time.After(time.Second):
		t.Fatal("no thread edit event was published")
	}

	response = postForm(t, server, editPath, url.Values{"title": {"Hijacked"}, "body": {"body"}}, bobCookie)
	if response.Code != http.StatusForbidden {
		t.Fatalf("edit by another user: status = %d, want %d", response.Code, http.StatusForbidden)
	}
	response = postForm(t, server, editPath, url.Values{"title": {""}, "body": {"body"}}, aliceCookie)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid edit status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	assertNoThreadEvent(t, updates)
}

func TestThreadVotePublishesLiveUpdate(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	aliceCookie := signIn(t, server, alice)
	bobCookie := signIn(t, server, bob)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())

	updates, unsubscribe := server.hub.Subscribe(hub.ThreadTopic(thread.ID))
	defer unsubscribe()

	response := doJSONPost(t, server, threadPath(thread.PublicID)+"/vote", bobCookie)
	if response.Code != http.StatusOK {
		t.Fatalf("vote status = %d, want %d", response.Code, http.StatusOK)
	}

	select {
	case event := <-updates:
		if event.Name != "vote" {
			t.Fatalf("event name = %q, want vote", event.Name)
		}
		var payload voteEvent
		if err := json.Unmarshal(event.Data, &payload); err != nil {
			t.Fatalf("decode vote event: %v", err)
		}
		if payload.CommentID != nil {
			t.Errorf("thread vote comment id = %v, want null", payload.CommentID)
		}
		if payload.Score != 2 || payload.ScoreText != "2 points" {
			t.Errorf("thread vote = %+v, want score 2", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("no thread vote event was published")
	}

	// The author may remove their own vote, which publishes the new score.
	response = doJSONPost(t, server, threadPath(thread.PublicID)+"/vote", aliceCookie)
	if response.Code != http.StatusOK {
		t.Fatalf("own vote status = %d, want %d", response.Code, http.StatusOK)
	}

	select {
	case event := <-updates:
		if event.Name != "vote" {
			t.Fatalf("event name = %q, want vote", event.Name)
		}
		var payload voteEvent
		if err := json.Unmarshal(event.Data, &payload); err != nil {
			t.Fatalf("decode own vote event: %v", err)
		}
		if payload.Score != 1 || payload.ScoreText != "1 point" {
			t.Errorf("own thread unvote = %+v, want score 1", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("no own thread vote event was published")
	}
}

func TestCommentVotePublishesLiveUpdate(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	aliceCookie := signIn(t, server, alice)
	bobCookie := signIn(t, server, bob)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	comment := createCommentDirect(t, server, thread, alice, nil, "comment", server.now())

	updates, unsubscribe := server.hub.Subscribe(hub.ThreadTopic(thread.ID))
	defer unsubscribe()

	response := doJSONPost(t, server, commentAction(comment.PublicID, "vote"), bobCookie)
	if response.Code != http.StatusOK {
		t.Fatalf("vote status = %d, want %d", response.Code, http.StatusOK)
	}

	select {
	case event := <-updates:
		if event.Name != "vote" {
			t.Fatalf("event name = %q, want vote", event.Name)
		}
		var payload voteEvent
		if err := json.Unmarshal(event.Data, &payload); err != nil {
			t.Fatalf("decode vote event: %v", err)
		}
		if payload.CommentID == nil || *payload.CommentID != comment.PublicID {
			t.Errorf("comment vote id = %v, want %s", payload.CommentID, comment.PublicID)
		}
		if payload.Score != 2 || payload.ScoreText != "2 points" {
			t.Errorf("comment vote = %+v, want score 2", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("no comment vote event was published")
	}

	// The author may remove their own vote, which publishes the new score.
	response = doJSONPost(t, server, commentAction(comment.PublicID, "vote"), aliceCookie)
	if response.Code != http.StatusOK {
		t.Fatalf("own vote status = %d, want %d", response.Code, http.StatusOK)
	}

	select {
	case event := <-updates:
		if event.Name != "vote" {
			t.Fatalf("event name = %q, want vote", event.Name)
		}
		var payload voteEvent
		if err := json.Unmarshal(event.Data, &payload); err != nil {
			t.Fatalf("decode own vote event: %v", err)
		}
		if payload.CommentID == nil || *payload.CommentID != comment.PublicID {
			t.Errorf("own comment vote id = %v, want %s", payload.CommentID, comment.PublicID)
		}
		if payload.Score != 1 || payload.ScoreText != "1 point" {
			t.Errorf("own comment unvote = %+v, want score 1", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("no own comment vote event was published")
	}
}

func TestCommentReactionPublishesLiveUpdate(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	bobCookie := signIn(t, server, bob)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	comment := createCommentDirect(t, server, thread, alice, nil, "comment", server.now())

	updates, unsubscribe := server.hub.Subscribe(hub.ThreadTopic(thread.ID))
	defer unsubscribe()

	response := doJSONFormPost(t, server, commentAction(comment.PublicID, "reaction"), url.Values{"reaction": {"party"}}, bobCookie)
	if response.Code != http.StatusOK {
		t.Fatalf("reaction status = %d, want %d", response.Code, http.StatusOK)
	}

	select {
	case event := <-updates:
		if event.Name != "vote" {
			t.Fatalf("event name = %q, want vote", event.Name)
		}
		var payload voteEvent
		if err := json.Unmarshal(event.Data, &payload); err != nil {
			t.Fatalf("decode vote event: %v", err)
		}
		if payload.CommentID == nil || *payload.CommentID != comment.PublicID {
			t.Errorf("reaction comment id = %v, want %s", payload.CommentID, comment.PublicID)
		}
		if payload.Score != 2 || payload.ScoreText != "2 points" {
			t.Errorf("reaction event = %+v, want score 2", payload)
		}
		if counts := reactionCountsByKey(payload.Reactions); counts["party"] != 1 || len(payload.Reactions) != len(db.CommentReactions) {
			t.Errorf("reaction event counts = %v, want every reaction with party 1", counts)
		}
	case <-time.After(time.Second):
		t.Fatal("no reaction event was published")
	}
}

// assertNoThreadEvent reports an event published on a topic that should have
// stayed quiet.
func assertNoThreadEvent(t *testing.T, events <-chan hub.Event) {
	t.Helper()

	select {
	case event := <-events:
		t.Fatalf("unexpected %q event: %s", event.Name, event.Data)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestNotificationEventsCarryTextAndURL(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	aliceCookie := signIn(t, server, alice)
	bobCookie := signIn(t, server, bob)

	aliceEvents, unsubscribeAlice := server.hub.Subscribe(hub.UserTopic(alice.ID))
	defer unsubscribeAlice()
	bobEvents, unsubscribeBob := server.hub.Subscribe(hub.UserTopic(bob.ID))
	defer unsubscribeBob()

	thread := createThreadDirect(t, server, alice, "A thread", "", "body", server.now())

	// A top-level comment notifies the thread's author.
	response := postForm(t, server, threadPath(thread.PublicID)+"/comments", url.Values{
		"body": {"top level"},
	}, bobCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("comment status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	rootID := anchorPublicID(t, response.Header().Get("Location"))
	assertNotificationEvent(t, aliceEvents, "bob commented on your thread A thread", commentPagePath(rootID))

	select {
	case event := <-bobEvents:
		t.Fatalf("bob got a notification event for his own comment: %+v", event)
	default:
	}

	// A reply notifies the parent comment's author.
	response = postForm(t, server, "/comments/"+rootID+"/reply", url.Values{
		"body": {"a reply"},
	}, aliceCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("reply status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	replyID := anchorPublicID(t, response.Header().Get("Location"))
	assertNotificationEvent(t, bobEvents, "alice replied to you in A thread", commentPagePath(replyID))

	// A mention in a thread body has no comment, so it links to the thread.
	response = postForm(t, server, "/threads", url.Values{
		"title": {"Mention thread"},
		"body":  {"hello @alice"},
	}, bobCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("create thread status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	mentionThreadID := strings.TrimPrefix(response.Header().Get("Location"), "/threads/")
	assertNotificationEvent(t, aliceEvents, "bob mentioned you in Mention thread", threadPath(mentionThreadID))
}

func assertNotificationEvent(t *testing.T, events <-chan hub.Event, wantText, wantURL string) {
	t.Helper()

	// Badge events share the user topic, so step over them until the
	// notification arrives.
	for {
		select {
		case event := <-events:
			if event.Name == "badge" {
				continue
			}
			if event.Name != "notification" {
				t.Fatalf("event name = %q, want notification", event.Name)
			}
			var payload notificationEvent
			if err := json.Unmarshal(event.Data, &payload); err != nil {
				t.Fatalf("decode notification event: %v", err)
			}
			if payload.Text != wantText {
				t.Errorf("notification text = %q, want %q", payload.Text, wantText)
			}
			if payload.URL != wantURL {
				t.Errorf("notification URL = %q, want %q", payload.URL, wantURL)
			}
			return
		case <-time.After(time.Second):
			t.Fatal("no notification event was published")
		}
	}
}

func TestReplyEventCarriesParentID(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	bobCookie := signIn(t, server, bob)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())
	root := createCommentDirect(t, server, thread, alice, nil, "root", server.now())

	comments, unsubscribe := server.hub.Subscribe(hub.ThreadTopic(thread.ID))
	defer unsubscribe()

	response := postForm(t, server, "/comments/"+root.PublicID+"/reply", url.Values{
		"body": {"reply"},
	}, bobCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("reply status = %d, want %d", response.Code, http.StatusSeeOther)
	}

	select {
	case event := <-comments:
		var payload commentEvent
		if err := json.Unmarshal(event.Data, &payload); err != nil {
			t.Fatalf("decode comment event: %v", err)
		}
		if payload.ParentID == nil || *payload.ParentID != root.PublicID {
			t.Errorf("reply event parent = %v, want %s", payload.ParentID, root.PublicID)
		}
	case <-time.After(time.Second):
		t.Fatal("no comment event was published")
	}
}

func TestEventsStreamSendsInitialBadgeAndNewComments(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())

	testServer := httptest.NewServer(server.Handler())
	defer testServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, testServer.URL+"/events?thread_id="+thread.PublicID, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	request.AddCookie(cookie)

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("open event stream: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("event stream status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if got := response.Header.Get("X-Accel-Buffering"); got != "no" {
		t.Errorf("X-Accel-Buffering = %q, want no", got)
	}
	if got := response.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", got)
	}

	reader := bufio.NewReader(response.Body)
	name, data := readSSEEvent(t, reader)
	if name != "badge" {
		t.Fatalf("first event = %q, want badge", name)
	}
	var badge badgeEvent
	if err := json.Unmarshal([]byte(data), &badge); err != nil || badge.Count != 0 {
		t.Fatalf("initial badge event = %q, %v", data, err)
	}

	comment := createCommentDirect(t, server, thread, alice, nil, "streamed", server.now())
	server.publishNewComment(context.Background(), thread.ID, comment)

	name, data = readSSEEvent(t, reader)
	if name != "comment" {
		t.Fatalf("second event = %q, want comment", name)
	}
	var payload commentEvent
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		t.Fatalf("decode comment event: %v", err)
	}
	if payload.CommentID != comment.PublicID || !strings.Contains(payload.HTML, "streamed") {
		t.Errorf("comment event = %+v", payload)
	}
}

func TestEventsStreamReplaysMissedThreadChanges(t *testing.T) {
	server := newTestServer(t)
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	server.now = func() time.Time { return base }

	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	bobCookie := signIn(t, server, bob)
	thread := createThreadDirect(t, server, alice, "Original", "", "old body", base.Add(-time.Hour))

	// Two comments older than the page, one edited and one deleted after it.
	edited := createCommentDirect(t, server, thread, alice, nil, "old text", base.Add(-time.Minute))
	deleted := createCommentDirect(t, server, thread, alice, nil, "gone text", base.Add(-time.Minute))
	created := createCommentDirect(t, server, thread, alice, nil, "new **comment**", base.Add(time.Second))
	reply := createCommentDirect(t, server, thread, alice, &created.ID, "reply text", base.Add(2*time.Second))
	// These arrive live after the page, then change while the stream is
	// disconnected, so each needs a second replay event.
	newEdited := createCommentDirect(t, server, thread, alice, nil, "first version", base.Add(6*time.Second))
	newDeleted := createCommentDirect(t, server, thread, alice, nil, "short lived", base.Add(8*time.Second))

	if _, err := server.db.UpdateComment(context.Background(), edited.ID, alice.ID, "edited text", "<p>edited text</p>", base.Add(3*time.Second)); err != nil {
		t.Fatalf("UpdateComment: %v", err)
	}
	if _, err := server.db.DeleteComment(context.Background(), deleted.ID, base.Add(4*time.Second)); err != nil {
		t.Fatalf("DeleteComment: %v", err)
	}
	if _, err := server.db.UpdateThread(context.Background(), thread.ID, alice.ID, "New title", "https://example.com/", "new body", "<p>new body</p>", base.Add(5*time.Second)); err != nil {
		t.Fatalf("UpdateThread: %v", err)
	}
	if _, err := server.db.UpdateComment(context.Background(), newEdited.ID, alice.ID, "edited **after**", "<p>edited <strong>after</strong></p>", base.Add(7*time.Second)); err != nil {
		t.Fatalf("UpdateComment: %v", err)
	}
	if _, err := server.db.DeleteComment(context.Background(), newDeleted.ID, base.Add(9*time.Second)); err != nil {
		t.Fatalf("DeleteComment: %v", err)
	}
	// The viewer's own vote must show in the replayed rendering.
	if _, err := server.db.ToggleCommentVote(context.Background(), bob.ID, created.ID); err != nil {
		t.Fatalf("ToggleCommentVote: %v", err)
	}

	// The page passes the time it rendered, before it loaded the comments.
	page := doRequest(t, server, http.MethodGet, threadPath(thread.PublicID), nil, bobCookie, "").Body.String()
	wantURL := `data-sse-url="/events?thread_id=` + thread.PublicID + `&since=` + strconv.FormatInt(base.Unix(), 10) + `"`
	if !strings.Contains(page, wantURL) {
		t.Errorf("thread page does not contain %q", wantURL)
	}

	testServer := httptest.NewServer(server.Handler())
	defer testServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, testServer.URL+"/events?thread_id="+thread.PublicID+"&since="+strconv.FormatInt(base.Unix(), 10), nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	request.AddCookie(bobCookie)

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("open event stream: %v", err)
	}
	defer response.Body.Close()

	reader := bufio.NewReader(response.Body)
	if name, _ := readSSEEvent(t, reader); name != "badge" {
		t.Fatalf("first event = %q, want badge", name)
	}

	name, data := readSSEEvent(t, reader)
	if name != "comment_edit" {
		t.Fatalf("first replay event = %q, want comment_edit", name)
	}
	var edit commentEditEvent
	if err := json.Unmarshal([]byte(data), &edit); err != nil {
		t.Fatalf("decode comment edit: %v", err)
	}
	if edit.CommentID != edited.PublicID || !strings.Contains(edit.HTML, "edited text") {
		t.Errorf("replayed comment edit = %+v", edit)
	}

	name, data = readSSEEvent(t, reader)
	if name != "comment_delete" {
		t.Fatalf("second replay event = %q, want comment_delete", name)
	}
	var deletion commentDeleteEvent
	if err := json.Unmarshal([]byte(data), &deletion); err != nil {
		t.Fatalf("decode comment delete: %v", err)
	}
	if deletion.CommentID != deleted.PublicID || !strings.Contains(deletion.HTML, "[deleted]") {
		t.Errorf("replayed comment delete = %+v", deletion)
	}
	if deletion.CommentCountText != "4 comments" {
		t.Errorf("replayed comment delete count = %q, want 4 comments", deletion.CommentCountText)
	}

	name, data = readSSEEvent(t, reader)
	if name != "comment" {
		t.Fatalf("third replay event = %q, want comment", name)
	}
	var parent commentEvent
	if err := json.Unmarshal([]byte(data), &parent); err != nil {
		t.Fatalf("decode comment: %v", err)
	}
	if parent.CommentID != created.PublicID || parent.ParentID != nil {
		t.Errorf("replayed comment = %+v", parent)
	}
	if !strings.Contains(parent.HTML, "<strong>comment</strong>") {
		t.Errorf("replayed comment HTML = %q, want rendered body", parent.HTML)
	}
	// The replay is rendered for this viewer, including their own vote.
	for _, want := range []string{`class="vote on"`, `data-voted="true"`} {
		if !strings.Contains(parent.HTML, want) {
			t.Errorf("replayed comment HTML does not contain %q: %s", want, parent.HTML)
		}
	}
	if parent.CommentCountText != "4 comments" {
		t.Errorf("replayed comment count = %q, want 4 comments", parent.CommentCountText)
	}

	name, data = readSSEEvent(t, reader)
	if name != "comment" {
		t.Fatalf("fourth replay event = %q, want comment", name)
	}
	var child commentEvent
	if err := json.Unmarshal([]byte(data), &child); err != nil {
		t.Fatalf("decode reply: %v", err)
	}
	if child.CommentID != reply.PublicID || child.ParentID == nil || *child.ParentID != created.PublicID {
		t.Errorf("replayed reply = %+v", child)
	}

	// A comment that arrived live and was edited while disconnected is sent
	// twice: the comment in its current state, then the edit.
	name, data = readSSEEvent(t, reader)
	if name != "comment" {
		t.Fatalf("fifth replay event = %q, want comment", name)
	}
	var newComment commentEvent
	if err := json.Unmarshal([]byte(data), &newComment); err != nil {
		t.Fatalf("decode comment: %v", err)
	}
	if newComment.CommentID != newEdited.PublicID || !strings.Contains(newComment.HTML, "<strong>after</strong>") {
		t.Errorf("replayed edited comment = %+v", newComment)
	}

	name, data = readSSEEvent(t, reader)
	if name != "comment_edit" {
		t.Fatalf("sixth replay event = %q, want comment_edit", name)
	}
	var newEdit commentEditEvent
	if err := json.Unmarshal([]byte(data), &newEdit); err != nil {
		t.Fatalf("decode comment edit: %v", err)
	}
	if newEdit.CommentID != newEdited.PublicID || !strings.Contains(newEdit.HTML, "<strong>after</strong>") {
		t.Errorf("replayed edit of a new comment = %+v", newEdit)
	}

	// A comment that arrived live and was deleted while disconnected is also
	// sent twice: the deleted comment, then the delete.
	name, data = readSSEEvent(t, reader)
	if name != "comment" {
		t.Fatalf("seventh replay event = %q, want comment", name)
	}
	var goneComment commentEvent
	if err := json.Unmarshal([]byte(data), &goneComment); err != nil {
		t.Fatalf("decode comment: %v", err)
	}
	if goneComment.CommentID != newDeleted.PublicID || !strings.Contains(goneComment.HTML, "[deleted]") {
		t.Errorf("replayed deleted comment = %+v", goneComment)
	}

	name, data = readSSEEvent(t, reader)
	if name != "comment_delete" {
		t.Fatalf("eighth replay event = %q, want comment_delete", name)
	}
	var newDeletion commentDeleteEvent
	if err := json.Unmarshal([]byte(data), &newDeletion); err != nil {
		t.Fatalf("decode comment delete: %v", err)
	}
	if newDeletion.CommentID != newDeleted.PublicID || !strings.Contains(newDeletion.HTML, "[deleted]") {
		t.Errorf("replayed delete of a new comment = %+v", newDeletion)
	}

	name, data = readSSEEvent(t, reader)
	if name != "thread_edit" {
		t.Fatalf("last replay event = %q, want thread_edit", name)
	}
	var threadEdit threadUpdateEvent
	if err := json.Unmarshal([]byte(data), &threadEdit); err != nil {
		t.Fatalf("decode thread edit: %v", err)
	}
	if !strings.Contains(threadEdit.TitleHTML, "New title") || !strings.Contains(threadEdit.TitleHTML, `href="https://example.com/"`) {
		t.Errorf("replayed thread title = %q", threadEdit.TitleHTML)
	}
	if !strings.Contains(threadEdit.BodyHTML, "<p>new body</p>") {
		t.Errorf("replayed thread body = %q", threadEdit.BodyHTML)
	}
}

func TestEventsStreamDoesNotReplayWithoutSince(t *testing.T) {
	server := newTestServer(t)
	base := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	server.now = func() time.Time { return base }

	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)
	thread := createThreadDirect(t, server, alice, "Thread", "", "body", base.Add(-time.Hour))
	createCommentDirect(t, server, thread, alice, nil, "after the page", base.Add(time.Minute))

	testServer := httptest.NewServer(server.Handler())
	defer testServer.Close()

	for _, test := range []struct {
		name  string
		query string
	}{
		{name: "no since", query: "/events?thread_id=" + thread.PublicID},
		{name: "invalid since", query: "/events?thread_id=" + thread.PublicID + "&since=nope"},
		{name: "no thread", query: "/events?since=" + strconv.FormatInt(base.Unix(), 10)},
		{name: "changes before since", query: "/events?thread_id=" + thread.PublicID + "&since=" + strconv.FormatInt(base.Add(time.Hour).Unix(), 10)},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			request, err := http.NewRequestWithContext(ctx, http.MethodGet, testServer.URL+test.query, nil)
			if err != nil {
				t.Fatalf("create request: %v", err)
			}
			request.AddCookie(cookie)

			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatalf("open event stream: %v", err)
			}
			defer response.Body.Close()

			reader := bufio.NewReader(response.Body)
			if name, _ := readSSEEvent(t, reader); name != "badge" {
				t.Fatalf("first event = %q, want badge", name)
			}

			// Nothing must follow the badge. The read unblocks when the
			// request ends, which happens after the timeout below.
			readDone := make(chan string, 1)
			go func() {
				line, _ := reader.ReadString('\n')
				readDone <- strings.TrimSpace(line)
			}()
			select {
			case line := <-readDone:
				t.Fatalf("stream replayed %q", line)
			case <-time.After(200 * time.Millisecond):
			}
		})
	}
}

func TestEventStreamClosesWhenSessionInvalidated(t *testing.T) {
	for _, test := range []struct {
		name       string
		invalidate func(t *testing.T, server *Server, user *db.User, cookie *http.Cookie)
		publish    func(server *Server, user *db.User, threadID int64)
	}{
		{
			name: "logout badge",
			invalidate: func(t *testing.T, server *Server, user *db.User, cookie *http.Cookie) {
				t.Helper()
				response := postForm(t, server, "/logout", nil, cookie)
				if response.Code != http.StatusSeeOther {
					t.Fatalf("logout status = %d, want %d", response.Code, http.StatusSeeOther)
				}
			},
			publish: func(server *Server, user *db.User, threadID int64) {
				server.hub.Publish(hub.UserTopic(user.ID), hub.Event{Name: "badge", Data: []byte(`{"count":1}`)})
			},
		},
		{
			name: "disabled comment",
			invalidate: func(t *testing.T, server *Server, user *db.User, cookie *http.Cookie) {
				t.Helper()
				if _, err := server.db.DisableUser(context.Background(), user.ID, server.now()); err != nil {
					t.Fatalf("DisableUser: %v", err)
				}
			},
			publish: func(server *Server, user *db.User, threadID int64) {
				server.hub.Publish(hub.ThreadTopic(threadID), hub.Event{Name: "comment", Data: []byte(`{}`)})
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := newTestServer(t)
			alice := createTestUser(t, server, "alice", false)
			cookie := signIn(t, server, alice)
			thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())

			testServer := httptest.NewServer(server.Handler())
			defer testServer.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			request, err := http.NewRequestWithContext(ctx, http.MethodGet, testServer.URL+"/events?thread_id="+thread.PublicID, nil)
			if err != nil {
				t.Fatalf("create request: %v", err)
			}
			request.AddCookie(cookie)

			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatalf("open event stream: %v", err)
			}
			defer response.Body.Close()

			reader := bufio.NewReader(response.Body)
			if name, _ := readSSEEvent(t, reader); name != "badge" {
				t.Fatalf("first event = %q, want badge", name)
			}

			test.invalidate(t, server, alice, cookie)

			// The next event makes the handler revalidate the session before
			// it would send the event to the client.
			test.publish(server, alice, thread.ID)

			readDone := make(chan error, 1)
			go func() {
				_, err := reader.ReadString('\n')
				readDone <- err
			}()

			select {
			case err := <-readDone:
				if err == nil {
					t.Fatal("stream sent an event after its session was invalidated")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("stream stayed open after its session was invalidated")
			}
		})
	}
}

func mustListNotifications(t *testing.T, server *Server, userID int64) []db.Notification {
	t.Helper()

	notifications, err := server.db.ListNotifications(context.Background(), userID, 1000, 0)
	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	return notifications
}

// assertNotificationKinds compares a recipient's notifications newest first.
func assertNotificationKinds(t *testing.T, notifications []db.Notification, want ...string) {
	t.Helper()

	got := make([]string, len(notifications))
	for i, notification := range notifications {
		got[i] = notification.Kind
	}
	if !slices.Equal(got, want) {
		t.Errorf("notification kinds = %v, want %v", got, want)
	}
}

func readSSEEvent(t *testing.T, reader *bufio.Reader) (string, string) {
	t.Helper()

	var name, data string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read event stream: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			return name, data
		}
		if strings.HasPrefix(line, "event: ") {
			name = strings.TrimPrefix(line, "event: ")
		}
		if strings.HasPrefix(line, "data: ") {
			data = strings.TrimPrefix(line, "data: ")
		}
	}
}
