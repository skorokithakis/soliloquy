package ntfy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSendPostsPlainTextWithHeaders(t *testing.T) {
	var (
		method      string
		contentType string
		title       string
		click       string
		body        string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		contentType = r.Header.Get("Content-Type")
		title = r.Header.Get("Title")
		click = r.Header.Get("Click")
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		body = string(data)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	message := Message{
		Title: "soliloquy",
		Text:  "alice replied to you in Thread",
		Click: "https://forum.test/threads/abc#c1",
	}
	if err := NewClient().Send(context.Background(), server.URL, message); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if method != http.MethodPost {
		t.Errorf("method = %q, want POST", method)
	}
	if contentType != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/plain; charset=utf-8", contentType)
	}
	if title != message.Title {
		t.Errorf("Title = %q, want %q", title, message.Title)
	}
	if click != message.Click {
		t.Errorf("Click = %q, want %q", click, message.Click)
	}
	if body != message.Text {
		t.Errorf("body = %q, want %q", body, message.Text)
	}
}

func TestSendOmitsEmptyHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if value := r.Header.Get("Title"); value != "" {
			t.Errorf("Title = %q, want no header", value)
		}
		if value := r.Header.Get("Click"); value != "" {
			t.Errorf("Click = %q, want no header", value)
		}
	}))
	defer server.Close()

	if err := NewClient().Send(context.Background(), server.URL, Message{Text: "hello"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
}

func TestSendReportsStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer server.Close()

	err := NewClient().Send(context.Background(), server.URL, Message{Text: "hello"})
	var statusErr *StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("error = %v, want a StatusError", err)
	}
	if statusErr.Code != http.StatusNotFound {
		t.Errorf("status code = %d, want %d", statusErr.Code, http.StatusNotFound)
	}
	if strings.Contains(err.Error(), server.URL) {
		t.Errorf("error %q contains the topic URL", err)
	}
}

func TestSendTransportErrorHidesURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	topicURL := server.URL
	server.Close()

	err := NewClient().Send(context.Background(), topicURL, Message{Text: "hello"})
	if err == nil {
		t.Fatal("Send to a closed server succeeded")
	}
	if strings.Contains(err.Error(), topicURL) {
		t.Errorf("error %q contains the topic URL", err)
	}
	if !strings.Contains(err.Error(), "connection failed") {
		t.Errorf("error %q does not use the allowlisted description", err)
	}
	if strings.Contains(err.Error(), "connection refused") {
		t.Errorf("error %q repeats the transport error", err)
	}
	var requestErr *RequestError
	if !errors.As(err, &requestErr) {
		t.Errorf("error = %v, want a RequestError", err)
	}
}

// roundTripperFunc lets a test return a response without a network server.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestSendStatusErrorHidesStatusText(t *testing.T) {
	topicURL := "https://ntfy.sh/secret-topic-abc"
	client := &Client{HTTP: &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Status:     "500 Internal Server Error at " + topicURL,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    request,
		}, nil
	})}}

	err := client.Send(context.Background(), topicURL, Message{Text: "hello"})
	if err == nil {
		t.Fatal("Send accepted an error status")
	}
	if strings.Contains(err.Error(), topicURL) {
		t.Errorf("error %q contains the topic URL", err)
	}
	if err.Error() != "ntfy returned HTTP 500" {
		t.Errorf("error = %q, want only the numeric status code", err)
	}
}

func TestSendDoesNotFollowRedirects(t *testing.T) {
	topicURL := "https://ntfy.sh/secret-topic-abc"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, topicURL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()

	err := NewClient().Send(context.Background(), server.URL, Message{Text: "hello"})
	if err == nil {
		t.Fatal("Send accepted a redirect")
	}
	var statusErr *StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("error = %v, want a StatusError", err)
	}
	if statusErr.Code != http.StatusTemporaryRedirect {
		t.Errorf("status code = %d, want %d", statusErr.Code, http.StatusTemporaryRedirect)
	}
	if strings.Contains(err.Error(), topicURL) {
		t.Errorf("error %q contains the redirect Location", err)
	}
}

func TestNewThreadText(t *testing.T) {
	got := NewThreadText("A thread", "alice")
	want := "New thread: A thread\nby alice"
	if got != want {
		t.Errorf("NewThreadText = %q, want %q", got, want)
	}
}
