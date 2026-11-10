package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/skorokithakis/soliloquy/internal/discord"
)

type webhookMessage struct {
	URL     string
	Content string
}

// fakeWebhookSender records messages instead of posting them. It is set on the
// server so tests never touch Discord.
type fakeWebhookSender struct {
	mu       sync.Mutex
	messages []webhookMessage
	errs     map[string]error
	sent     chan webhookMessage
}

func newFakeWebhookSender() *fakeWebhookSender {
	return &fakeWebhookSender{
		errs: map[string]error{},
		sent: make(chan webhookMessage, 16),
	}
}

func (f *fakeWebhookSender) Send(_ context.Context, webhookURL, content string) error {
	message := webhookMessage{URL: webhookURL, Content: content}
	f.mu.Lock()
	f.messages = append(f.messages, message)
	f.mu.Unlock()
	f.sent <- message
	return f.errs[webhookURL]
}

func (f *fakeWebhookSender) received(t *testing.T, count int) []webhookMessage {
	t.Helper()

	messages := make([]webhookMessage, 0, count)
	for len(messages) < count {
		select {
		case message := <-f.sent:
			messages = append(messages, message)
		case <-time.After(2 * time.Second):
			t.Fatalf("received %d webhook messages, want %d", len(messages), count)
		}
	}
	return messages
}

// blockingWebhookSender records calls and holds every send until release, so a
// test can tell whether the handler waited for it. It records the context each
// send was given.
type blockingWebhookSender struct {
	mu        sync.Mutex
	calls     []webhookMessage
	contexts  chan context.Context
	completed chan struct{}
	release   chan struct{}
	once      sync.Once
	err       error
}

func newBlockingWebhookSender() *blockingWebhookSender {
	return &blockingWebhookSender{
		contexts:  make(chan context.Context, 16),
		completed: make(chan struct{}, 16),
		release:   make(chan struct{}),
	}
}

func (b *blockingWebhookSender) Send(ctx context.Context, webhookURL, content string) error {
	b.mu.Lock()
	b.calls = append(b.calls, webhookMessage{URL: webhookURL, Content: content})
	b.mu.Unlock()
	b.contexts <- ctx
	<-b.release
	b.completed <- struct{}{}
	return b.err
}

func (b *blockingWebhookSender) releaseAll() {
	b.once.Do(func() { close(b.release) })
}

func (b *blockingWebhookSender) called() []webhookMessage {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]webhookMessage(nil), b.calls...)
}

func saveDiscordWebhooks(t *testing.T, server *Server, webhooks []string) {
	t.Helper()

	settings, err := server.db.Settings(context.Background())
	if err != nil {
		t.Fatalf("Settings: %v", err)
	}
	settings.DiscordWebhooks = webhooks
	if err := server.db.SetSettings(context.Background(), settings); err != nil {
		t.Fatalf("SetSettings: %v", err)
	}
}

func TestCreateThreadPostsToDiscordWebhooks(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)

	webhooks := []string{
		"https://discord.com/api/webhooks/1/aaa",
		"https://discord.com/api/webhooks/2/bbb",
	}
	sender := newBlockingWebhookSender()
	sender.err = errors.New("discord is down")
	server.webhooks = sender
	saveDiscordWebhooks(t, server, webhooks)

	// A real server cancels the request context when the handler returns, so
	// this checks that the sends do not use it.
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	// Registered after the server so it runs first, unblocking a send even
	// when the test fails before the explicit release.
	t.Cleanup(sender.releaseAll)

	form := url.Values{
		"title": {"Discord thread"},
		"url":   {"https://example.com/story"},
		"body":  {strings.Repeat("b", 350)},
	}.Encode()
	request, err := http.NewRequest(http.MethodPost, httpServer.URL+"/threads", strings.NewReader(form))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", testBaseURL)
	request.AddCookie(cookie)

	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("create thread: %v", err) // a handler waiting for the send times out here
	}
	defer response.Body.Close()

	// The handler returned its redirect while both sends are still blocked.
	if response.StatusCode != http.StatusSeeOther {
		t.Errorf("create status = %d, want %d", response.StatusCode, http.StatusSeeOther)
	}
	location := response.Header.Get("Location")
	if !strings.HasPrefix(location, "/threads/") {
		t.Fatalf("Location = %q, want a thread path", location)
	}

	for i := range webhooks {
		var sendCtx context.Context
		select {
		case sendCtx = <-sender.contexts:
		case <-time.After(2 * time.Second):
			t.Fatalf("only %d of %d sends started", i, len(webhooks))
		}
		select {
		case <-sendCtx.Done():
			t.Error("send context was cancelled when the request finished")
		default:
		}
	}

	gotURLs := map[string]bool{}
	for _, message := range sender.called() {
		gotURLs[message.URL] = true
		for _, want := range []string{
			"New thread: Discord thread",
			testBaseURL + location,
			"by alice",
			"https://example.com/story",
			strings.Repeat("b", discord.MaxBodyExcerpt) + "…",
		} {
			if !strings.Contains(message.Content, want) {
				t.Errorf("message for %s does not contain %q:\n%s", message.URL, want, message.Content)
			}
		}
	}
	for _, webhook := range webhooks {
		if !gotURLs[webhook] {
			t.Errorf("webhook %s did not receive a message", webhook)
		}
	}

	// The sends fail once released, but the redirect is unchanged.
	sender.releaseAll()
	for range webhooks {
		select {
		case <-sender.completed:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for the sends to finish")
		}
	}
	if response.StatusCode != http.StatusSeeOther || response.Header.Get("Location") != location {
		t.Error("a failed send changed the thread redirect")
	}
}
