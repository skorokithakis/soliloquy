package discord

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSendPostsWebhookPayload(t *testing.T) {
	var (
		method      string
		contentType string
		body        map[string]any
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		contentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := NewClient().Send(context.Background(), server.URL, "hello discord"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if method != http.MethodPost {
		t.Errorf("method = %q, want POST", method)
	}
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}
	if body["content"] != "hello discord" {
		t.Errorf("content = %v, want %q", body["content"], "hello discord")
	}

	mentions, ok := body["allowed_mentions"].(map[string]any)
	if !ok {
		t.Fatalf("allowed_mentions = %v, want an object", body["allowed_mentions"])
	}
	parse, ok := mentions["parse"].([]any)
	if !ok || len(parse) != 0 {
		t.Errorf("allowed_mentions.parse = %v, want an empty array", mentions["parse"])
	}
	if body["flags"] != float64(4) {
		t.Errorf("flags = %v, want 4 (suppress embeds)", body["flags"])
	}
}

func TestSendReportsStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer server.Close()

	err := NewClient().Send(context.Background(), server.URL, "hello")
	var statusErr *StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("error = %v, want a StatusError", err)
	}
	if statusErr.Code != http.StatusForbidden {
		t.Errorf("status code = %d, want %d", statusErr.Code, http.StatusForbidden)
	}
	if strings.Contains(err.Error(), server.URL) {
		t.Errorf("error %q contains the webhook URL", err)
	}
}

func TestSendTransportErrorHidesURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	webhookURL := server.URL
	server.Close()

	err := NewClient().Send(context.Background(), webhookURL, "hello")
	if err == nil {
		t.Fatal("Send to a closed server succeeded")
	}
	if strings.Contains(err.Error(), webhookURL) {
		t.Errorf("error %q contains the webhook URL", err)
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("error %q does not explain the cause", err)
	}
	var requestErr *RequestError
	if !errors.As(err, &requestErr) {
		t.Errorf("error = %v, want a RequestError", err)
	}
}

func TestSendTLSFailureExplainsReason(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()

	// The default client does not trust the test server's certificate, so this
	// fails the way a scratch container without CA certificates fails.
	err := NewClient().Send(context.Background(), server.URL, "hello")
	if err == nil {
		t.Fatal("Send to an untrusted server succeeded")
	}
	if !strings.Contains(err.Error(), "certificate") {
		t.Errorf("error %q does not explain the TLS failure", err)
	}
	if strings.Contains(err.Error(), server.URL) {
		t.Errorf("error %q contains the webhook URL", err)
	}
}

func TestNewThreadContent(t *testing.T) {
	content := NewThreadContent(ThreadMessage{
		Title:  "A thread",
		Link:   "https://forum.test/threads/abc",
		Author: "alice",
		Body:   "Some **Markdown** body.",
	})

	for _, want := range []string{
		"New thread: A thread",
		"https://forum.test/threads/abc",
		"by alice",
		"Some **Markdown** body.",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("message does not contain %q:\n%s", want, content)
		}
	}
}

func TestNewThreadContentOmitsEmptyParts(t *testing.T) {
	content := NewThreadContent(ThreadMessage{
		Title:  "Link only",
		Link:   "https://forum.test/threads/abc",
		Author: "bob",
	})

	if strings.Contains(content, "\n\n") {
		t.Errorf("message has an empty body section: %q", content)
	}
	if strings.HasSuffix(content, "\n") {
		t.Errorf("message ends with a newline: %q", content)
	}
}

func TestNewThreadContentCutsBodyTo300Runes(t *testing.T) {
	body := strings.Repeat("é", 400)
	content := NewThreadContent(ThreadMessage{
		Title:  "Long body",
		Link:   "https://forum.test/threads/abc",
		Author: "alice",
		Body:   body,
	})

	if !strings.Contains(content, strings.Repeat("é", MaxBodyExcerpt)+"…") {
		t.Error("message does not contain the 300-rune excerpt with an ellipsis")
	}
	if strings.Contains(content, strings.Repeat("é", MaxBodyExcerpt+1)) {
		t.Error("message contains more than 300 runes of the body")
	}
}

func TestNewThreadContentStaysWithinLimit(t *testing.T) {
	content := NewThreadContent(ThreadMessage{
		Title:  strings.Repeat("t", 5000),
		Link:   "https://forum.test/threads/abc",
		Author: "alice",
		Body:   strings.Repeat("b", 5000),
	})

	if length := utf8.RuneCountInString(content); length > MaxContentLength {
		t.Errorf("message length = %d, want at most %d", length, MaxContentLength)
	}
}
