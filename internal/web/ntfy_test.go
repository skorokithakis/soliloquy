package web

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/skorokithakis/soliloquy/internal/db"
	"github.com/skorokithakis/soliloquy/internal/ntfy"
)

type pushRequest struct {
	URL     string
	Message ntfy.Message
}

// fakePushSender records pushes instead of posting them. It is set on the
// server so tests never touch ntfy.
type fakePushSender struct {
	mu       sync.Mutex
	requests []pushRequest
	errs     map[string]error
	sent     chan pushRequest
}

func newFakePushSender() *fakePushSender {
	return &fakePushSender{
		errs: map[string]error{},
		sent: make(chan pushRequest, 32),
	}
}

func (f *fakePushSender) Send(_ context.Context, topicURL string, message ntfy.Message) error {
	request := pushRequest{URL: topicURL, Message: message}
	f.mu.Lock()
	f.requests = append(f.requests, request)
	f.mu.Unlock()
	f.sent <- request
	return f.errs[topicURL]
}

func (f *fakePushSender) received(t *testing.T, count int) []pushRequest {
	t.Helper()

	requests := make([]pushRequest, 0, count)
	for len(requests) < count {
		select {
		case request := <-f.sent:
			requests = append(requests, request)
		case <-time.After(2 * time.Second):
			t.Fatalf("received %d push requests, want %d", len(requests), count)
		}
	}
	return requests
}

func saveNtfySettings(t *testing.T, server *Server, user *db.User, topicURL string, replies, threads bool) {
	t.Helper()

	if err := server.db.SetNtfySettings(context.Background(), user.ID, topicURL, replies, threads); err != nil {
		t.Fatalf("SetNtfySettings: %v", err)
	}
}

func TestValidateNtfyURL(t *testing.T) {
	tests := map[string]struct {
		input   string
		want    string
		wantErr bool
	}{
		"empty turns pushes off": {input: "", want: ""},
		"spaces are empty":       {input: "   ", want: ""},
		"https is trimmed":       {input: "  https://ntfy.sh/topic  ", want: "https://ntfy.sh/topic"},
		"any host is allowed":    {input: "https://ntfy.example.com/topic", want: "https://ntfy.example.com/topic"},
		"http is rejected":       {input: "http://ntfy.sh/topic", wantErr: true},
		"no host":                {input: "https:///topic", wantErr: true},
		"relative":               {input: "/topic", wantErr: true},
		"garbage":                {input: "not a url", wantErr: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := validateNtfyURL(test.input)
			if test.wantErr {
				if err == nil {
					t.Fatalf("validateNtfyURL(%q) = %q, want an error", test.input, got)
				}
				if strings.Contains(err.Error(), strings.TrimSpace(test.input)) && strings.TrimSpace(test.input) != "" {
					t.Errorf("error %q repeats the topic URL", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateNtfyURL(%q): %v", test.input, err)
			}
			if got != test.want {
				t.Errorf("validateNtfyURL(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestAccountNtfySettings(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)

	body := doRequest(t, server, http.MethodGet, "/account", nil, cookie, "").Body.String()
	for _, want := range []string{
		`action="/account/notifications"`,
		`name="ntfy_url"`,
		`name="ntfy_replies" checked`,
		`name="ntfy_threads"`,
		"hard-to-guess name",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("account page does not contain %q", want)
		}
	}
	if strings.Contains(body, `name="ntfy_threads" checked`) {
		t.Error("new-thread pushes are on by default")
	}

	// An invalid URL is rejected and does not change the stored settings.
	response := postForm(t, server, "/account/notifications", url.Values{
		"ntfy_url":     {"http://ntfy.sh/topic"},
		"ntfy_replies": {"on"},
	}, cookie)
	if response.Code != http.StatusBadRequest {
		t.Errorf("invalid URL status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	body = response.Body.String()
	if !strings.Contains(body, "absolute https ntfy topic URL") {
		t.Error("invalid URL error is missing")
	}
	if !strings.Contains(body, `value="http://ntfy.sh/topic"`) {
		t.Error("invalid URL is not shown back in the form")
	}
	stored, err := server.db.UserByID(context.Background(), alice.ID)
	if err != nil {
		t.Fatalf("UserByID: %v", err)
	}
	if stored.NtfyURL != "" || !stored.NtfyReplies || stored.NtfyThreads {
		t.Errorf("invalid save changed settings to %+v", stored)
	}

	// A valid save stores the URL and both options.
	response = postForm(t, server, "/account/notifications", url.Values{
		"ntfy_url":     {"https://ntfy.sh/a-secret-topic"},
		"ntfy_replies": {"on"},
		"ntfy_threads": {"on"},
	}, cookie)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/account" {
		t.Fatalf("save status = %d, Location = %q", response.Code, response.Header().Get("Location"))
	}
	stored, err = server.db.UserByID(context.Background(), alice.ID)
	if err != nil {
		t.Fatalf("UserByID: %v", err)
	}
	if stored.NtfyURL != "https://ntfy.sh/a-secret-topic" || !stored.NtfyReplies || !stored.NtfyThreads {
		t.Errorf("stored settings = %+v", stored)
	}

	body = doRequest(t, server, http.MethodGet, "/account", nil, cookie, "").Body.String()
	if !strings.Contains(body, `value="https://ntfy.sh/a-secret-topic"`) {
		t.Error("account page does not show the saved topic URL")
	}
	for _, want := range []string{`name="ntfy_replies" checked`, `name="ntfy_threads" checked`} {
		if !strings.Contains(body, want) {
			t.Errorf("account page does not mark %q", want)
		}
	}

	// An empty URL turns pushes off.
	response = postForm(t, server, "/account/notifications", url.Values{
		"ntfy_url":     {"  "},
		"ntfy_replies": {"on"},
	}, cookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("clear status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	stored, err = server.db.UserByID(context.Background(), alice.ID)
	if err != nil {
		t.Fatalf("UserByID: %v", err)
	}
	if stored.NtfyURL != "" || !stored.NtfyReplies || stored.NtfyThreads {
		t.Errorf("settings after clearing = %+v", stored)
	}
}

func TestAccountNtfyTest(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)
	sender := newFakePushSender()
	server.pushes = sender

	response := postForm(t, server, "/account/notifications/test", nil, cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if !strings.Contains(response.Body.String(), "No ntfy topic URL is saved") {
		t.Error("result does not say that no URL is saved")
	}

	topicURL := "https://ntfy.sh/a-secret-topic"
	saveNtfySettings(t, server, alice, topicURL, true, false)

	response = postForm(t, server, "/account/notifications/test", nil, cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if !strings.Contains(response.Body.String(), "Test push sent to your ntfy topic.") {
		t.Errorf("result does not report success: %s", response.Body.String())
	}
	request := sender.received(t, 1)[0]
	if request.URL != topicURL {
		t.Errorf("push URL = %q, want the saved topic URL", request.URL)
	}
	if request.Message.Text != ntfyTestMessage {
		t.Errorf("push text = %q, want the test message", request.Message.Text)
	}
	if request.Message.Title != db.DefaultForumName {
		t.Errorf("push title = %q, want the forum name", request.Message.Title)
	}
	if request.Message.Click != testBaseURL+"/account" {
		t.Errorf("push click = %q, want the account page", request.Message.Click)
	}

	// A rejected push names the status but never repeats the URL.
	sender.errs[topicURL] = &ntfy.StatusError{Code: http.StatusInternalServerError}
	response = postForm(t, server, "/account/notifications/test", nil, cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	if !strings.Contains(body, "Test push failed: HTTP 500.") {
		t.Errorf("result does not report the status: %s", body)
	}
	result := body[strings.Index(body, "ntfy test"):]
	result = result[:strings.Index(result, "</div>")]
	if strings.Contains(result, topicURL) {
		t.Errorf("result repeats the topic URL: %s", result)
	}

	// A transport failure uses an allowlisted description, never the raw
	// error text, which can contain the URL.
	sender.errs[topicURL] = &ntfy.RequestError{Err: &url.Error{
		Op:  "Post",
		URL: topicURL,
		Err: errors.New("tls: failed to verify certificate: x509: certificate signed by unknown authority"),
	}}
	response = postForm(t, server, "/account/notifications/test", nil, cookie)
	body = response.Body.String()
	result = body[strings.Index(body, "ntfy test"):]
	result = result[:strings.Index(result, "</div>")]
	if !strings.Contains(result, "Test push failed: request failed.") {
		t.Errorf("result does not use the allowlisted failure description: %s", result)
	}
	if strings.Contains(result, topicURL) {
		t.Errorf("result repeats the topic URL: %s", result)
	}
}

func TestNtfyPersonalNotificationPushes(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	carol := createTestUser(t, server, "carol", false)
	aliceCookie := signIn(t, server, alice)
	bobCookie := signIn(t, server, bob)
	sender := newFakePushSender()
	server.pushes = sender

	saveNtfySettings(t, server, alice, "https://ntfy.sh/alice", true, false)
	saveNtfySettings(t, server, bob, "https://ntfy.sh/bob", true, false)
	saveNtfySettings(t, server, carol, "https://ntfy.sh/carol", true, false)

	thread := createThreadDirect(t, server, alice, "Thread", "", "body", server.now())

	// A top-level comment notifies the thread author and mentions carol.
	response := postForm(t, server, threadPath(thread.PublicID)+"/comments", url.Values{
		"body": {"hello @carol"},
	}, bobCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("comment status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	commentID := anchorPublicID(t, response.Header().Get("Location"))

	byURL := map[string]pushRequest{}
	for _, request := range sender.received(t, 2) {
		byURL[request.URL] = request
	}
	if request, ok := byURL["https://ntfy.sh/alice"]; !ok {
		t.Error("alice did not get a push for a comment on her thread")
	} else if !strings.Contains(request.Message.Text, "bob commented on your thread Thread") {
		t.Errorf("alice push text = %q", request.Message.Text)
	} else if !strings.HasSuffix(request.Message.Click, commentAnchorPath(thread.PublicID, commentID)) {
		t.Errorf("alice push click = %q, want the comment anchor", request.Message.Click)
	}
	if request, ok := byURL["https://ntfy.sh/carol"]; !ok {
		t.Error("carol did not get a push for the mention")
	} else if !strings.Contains(request.Message.Text, "bob mentioned you in Thread") {
		t.Errorf("carol push text = %q", request.Message.Text)
	}
	if _, ok := byURL["https://ntfy.sh/bob"]; ok {
		t.Error("bob got a push for his own comment")
	}

	// A reply notifies the comment's author.
	response = postForm(t, server, "/comments/"+commentID+"/reply", url.Values{
		"body": {"a reply"},
	}, aliceCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("reply status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	request := sender.received(t, 1)[0]
	if request.URL != "https://ntfy.sh/bob" {
		t.Errorf("reply push URL = %q, want bob", request.URL)
	}
	if !strings.Contains(request.Message.Text, "alice replied to you in Thread") {
		t.Errorf("reply push text = %q", request.Message.Text)
	}
}

func TestNtfyNewThreadPush(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)
	cookie := signIn(t, server, alice)
	sender := newFakePushSender()
	server.pushes = sender

	saveNtfySettings(t, server, alice, "https://ntfy.sh/alice", true, true)
	saveNtfySettings(t, server, bob, "https://ntfy.sh/bob", false, true)

	response := postForm(t, server, "/threads", url.Values{
		"title": {"A new thread"},
		"body":  {"body"},
	}, cookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("create status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	location := response.Header().Get("Location")

	request := sender.received(t, 1)[0]
	if request.URL != "https://ntfy.sh/bob" {
		t.Errorf("thread push URL = %q, want bob's topic", request.URL)
	}
	if request.Message.Text != "New thread: A new thread\nby alice" {
		t.Errorf("thread push text = %q", request.Message.Text)
	}
	if request.Message.Click != testBaseURL+location {
		t.Errorf("thread push click = %q, want %q", request.Message.Click, testBaseURL+location)
	}
	if request.Message.Title != db.DefaultForumName {
		t.Errorf("thread push title = %q, want the forum name", request.Message.Title)
	}
}

func TestNtfyPushRecipientRules(t *testing.T) {
	now := time.Now()
	active := &db.User{ID: 2, NtfyURL: "https://ntfy.sh/topic", NtfyReplies: true, NtfyThreads: true}
	disabled := &db.User{ID: 2, NtfyURL: "https://ntfy.sh/topic", NtfyReplies: true, NtfyThreads: true, DisabledAt: &now}
	deleted := &db.User{ID: 2, NtfyURL: "https://ntfy.sh/topic", NtfyReplies: true, NtfyThreads: true, DeletedAt: &now}
	noURL := &db.User{ID: 2, NtfyReplies: true, NtfyThreads: true}
	repliesOff := &db.User{ID: 2, NtfyURL: "https://ntfy.sh/topic", NtfyThreads: true}
	threadsOff := &db.User{ID: 2, NtfyURL: "https://ntfy.sh/topic", NtfyReplies: true}

	if !wantsNotificationPush(active) {
		t.Error("active user with a URL and replies on does not get personal pushes")
	}
	for name, user := range map[string]*db.User{
		"nil":         nil,
		"disabled":    disabled,
		"deleted":     deleted,
		"no URL":      noURL,
		"replies off": repliesOff,
	} {
		if wantsNotificationPush(user) {
			t.Errorf("%s user gets personal pushes", name)
		}
	}

	if !wantsThreadPush(active, 1) {
		t.Error("active user with a URL and threads on does not get new-thread pushes")
	}
	for name, user := range map[string]*db.User{
		"nil":         nil,
		"disabled":    disabled,
		"deleted":     deleted,
		"no URL":      noURL,
		"threads off": threadsOff,
	} {
		if wantsThreadPush(user, 1) {
			t.Errorf("%s user gets new-thread pushes", name)
		}
	}
	if wantsThreadPush(active, active.ID) {
		t.Error("the thread author gets a push for their own thread")
	}
}

func TestNtfyFailureLogHidesTopicURL(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)

	// A closed server makes the real client fail without any test fake
	// deciding what error to report.
	closed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	topicURL := closed.URL
	closed.Close()
	saveNtfySettings(t, server, alice, topicURL, true, false)

	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	server.pushNotification(&db.Notification{
		RecipientID:    alice.ID,
		Kind:           db.NotificationReply,
		ActorUsername:  "bob",
		ThreadTitle:    "Thread",
		ThreadPublicID: "abc",
	})

	if !strings.Contains(logs.String(), "send push notification") {
		t.Errorf("failed push was not logged: %s", logs.String())
	}
	if strings.Contains(logs.String(), topicURL) {
		t.Errorf("log contains the topic URL: %s", logs.String())
	}
}
