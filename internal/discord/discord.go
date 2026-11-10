// Package discord posts plain-text messages to Discord webhook URLs.
package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// MaxContentLength is Discord's limit on the content of a webhook message.
	MaxContentLength = 2000
	// MaxBodyExcerpt is how many runes of a thread body a message includes.
	MaxBodyExcerpt = 300

	requestTimeout = 10 * time.Second
)

// Client posts messages to webhook URLs.
type Client struct {
	HTTP *http.Client
}

// NewClient returns a Client with a timeout suitable for webhook requests.
func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: requestTimeout}}
}

// StatusError reports a webhook response that was not a success.
type StatusError struct {
	// Status is the HTTP status line, for example "404 Not Found".
	Status string
	// Code is the HTTP status code.
	Code int
}

func (e *StatusError) Error() string {
	return "discord returned " + e.Status
}

// RequestError wraps a transport error. Its message deliberately omits the
// underlying error because that text contains the webhook URL, which is a
// secret.
type RequestError struct {
	Err error
}

func (e *RequestError) Error() string { return "webhook request failed" }

func (e *RequestError) Unwrap() error { return e.Err }

// ThreadMessage is the data in a new-thread notification.
type ThreadMessage struct {
	Title       string
	Link        string
	Author      string
	ExternalURL string
	Body        string
}

// NewThreadContent builds the plain-text message for a new thread. It stays
// within Discord's content limit and mentions nobody by default.
func NewThreadContent(message ThreadMessage) string {
	var content strings.Builder
	content.WriteString("New thread: ")
	content.WriteString(message.Title)
	content.WriteString("\n")
	content.WriteString(message.Link)
	content.WriteString("\nby ")
	content.WriteString(message.Author)
	if message.ExternalURL != "" {
		content.WriteString("\n")
		content.WriteString(message.ExternalURL)
	}
	if body := bodyExcerpt(message.Body); body != "" {
		content.WriteString("\n\n")
		content.WriteString(body)
	}
	return clamp(content.String())
}

// bodyExcerpt trims the body and cuts it to MaxBodyExcerpt runes, ending with
// an ellipsis when something was left out.
func bodyExcerpt(body string) string {
	body = strings.TrimSpace(body)
	if utf8.RuneCountInString(body) <= MaxBodyExcerpt {
		return body
	}
	runes := []rune(body)
	return string(runes[:MaxBodyExcerpt]) + "…"
}

// clamp cuts content to Discord's limit. It is defensive: the message builder
// already keeps it well below the limit.
func clamp(content string) string {
	if utf8.RuneCountInString(content) <= MaxContentLength {
		return content
	}
	runes := []rune(content)
	return string(runes[:MaxContentLength-1]) + "…"
}

type payload struct {
	Content         string          `json:"content"`
	AllowedMentions allowedMentions `json:"allowed_mentions"`
}

type allowedMentions struct {
	Parse []string `json:"parse"`
}

// Send posts content to a webhook URL. Long content is cut to Discord's limit.
// The returned error never contains the webhook URL.
func (c *Client) Send(ctx context.Context, webhookURL, content string) error {
	body, err := json.Marshal(payload{
		Content:         clamp(content),
		AllowedMentions: allowedMentions{Parse: []string{}},
	})
	if err != nil {
		return fmt.Errorf("encode webhook payload: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return &RequestError{Err: err}
	}
	request.Header.Set("Content-Type", "application/json")

	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return &RequestError{Err: err}
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))

	if response.StatusCode < 200 || response.StatusCode > 299 {
		return &StatusError{Status: response.Status, Code: response.StatusCode}
	}
	return nil
}
