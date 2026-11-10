// Package ntfy posts plain-text push notifications to ntfy topic URLs.
package ntfy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"
)

const requestTimeout = 10 * time.Second

// Client posts messages to topic URLs. The URL is a secret: anyone who knows
// the topic name can read its messages, so errors returned by this package
// never contain it.
type Client struct {
	HTTP *http.Client
}

// NewClient returns a Client with a timeout suitable for push requests.
func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: requestTimeout}}
}

// Message is the content of a push. An empty Title or Click omits that header.
type Message struct {
	Title string
	Text  string
	Click string
}

// StatusError reports a push response that was not a success. It deliberately
// keeps only the numeric code: the status text is controlled by the server and
// could repeat the topic URL.
type StatusError struct {
	// Code is the HTTP status code.
	Code int
}

func (e *StatusError) Error() string {
	return "ntfy returned HTTP " + strconv.Itoa(e.Code)
}

// RequestError wraps a transport error. Its message is a fixed description so
// it can never repeat the topic URL, which can appear in a malformed URL, a
// redirect Location or a driver error.
type RequestError struct {
	Err error
}

func (e *RequestError) Error() string {
	return failureReason(e.Err)
}

func (e *RequestError) Unwrap() error { return e.Err }

// failureReason returns an allowlisted description of a failed request. It
// never returns the cause's text because that text may contain the topic URL.
func failureReason(err error) string {
	if err == nil {
		return "request failed"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timed out"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timed out"
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return "connection failed"
	}
	return "request failed"
}

// Send posts a message to a topic URL as plain text. The text is sent as
// written; ntfy decides how to display it.
// The returned error never contains the topic URL.
func (c *Client) Send(ctx context.Context, topicURL string, message Message) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, topicURL, bytes.NewBufferString(message.Text))
	if err != nil {
		return &RequestError{Err: err}
	}
	request.Header.Set("Content-Type", "text/plain; charset=utf-8")
	if message.Title != "" {
		request.Header.Set("Title", message.Title)
	}
	if message.Click != "" {
		request.Header.Set("Click", message.Click)
	}

	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	// Do not follow redirects: a Location is controlled by the server and can
	// contain the secret topic URL, which Go would repeat in a redirect error.
	// A 3xx therefore counts as a failed response.
	noRedirect := *client
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	response, err := noRedirect.Do(request)
	if err != nil {
		return &RequestError{Err: err}
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))

	if response.StatusCode < 200 || response.StatusCode > 299 {
		return &StatusError{Code: response.StatusCode}
	}
	return nil
}

// NewThreadText is the body of a push for a newly created thread: the title
// and its author. The forum link travels in the Click header, so it is not
// repeated here.
func NewThreadText(title, author string) string {
	return fmt.Sprintf("New thread: %s\nby %s", title, author)
}
