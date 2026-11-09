package web

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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
		commentAnchorPath(thread.PublicID, comment.PublicID),
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
		for _, unwanted := range []string{"/vote", "/edit", "/delete"} {
			if strings.Contains(payload.HTML, unwanted) {
				t.Errorf("live comment HTML contains viewer-specific control %q: %s", unwanted, payload.HTML)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("no comment event was published")
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
	server.publishNewComment(thread.ID, comment)

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
