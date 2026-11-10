// Package linktitle fetches the title of a web page, so the thread form can
// fill in a title when someone pastes a URL.
package linktitle

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	"golang.org/x/net/html"
)

const (
	// UserAgent identifies Soliloquy to the sites it fetches.
	UserAgent = "Soliloquy/1.0 (+https://github.com/skorokithakis/soliloquy)"

	// maxTitleLength is the longest title Fetch returns, in characters. It
	// matches the thread title limit.
	maxTitleLength = 200
	// maxResponseBytes is how much of a response the tokenizer reads. Some
	// sites, YouTube for example, put their <title> hundreds of kilobytes into
	// the page, so the limit cannot be much smaller.
	maxResponseBytes = 2 << 20
	requestTimeout   = 5 * time.Second
	maxRedirects     = 5
)

// Fetcher fetches page titles over HTTP.
type Fetcher struct {
	// AllowPrivateAddresses disables the guard that refuses connections to
	// loopback, private, link-local, unspecified and multicast addresses. It
	// exists for tests, which talk to httptest servers on 127.0.0.1. Set it
	// before the first request; it is not safe to change while Fetch runs.
	AllowPrivateAddresses bool

	// allowAddress lets a test allow one dial address (host:port) while the
	// guard stays on for everything else, so tests can check that a redirect
	// to a blocked address is refused. It is nil in production.
	allowAddress func(address string) bool

	client *http.Client
}

// NewFetcher returns a Fetcher with a timeout, a redirect limit and the
// private-address guard.
func NewFetcher() *Fetcher {
	fetcher := &Fetcher{}
	client := &http.Client{
		Transport: &http.Transport{
			DialContext:           fetcher.dialContext,
			TLSHandshakeTimeout:   requestTimeout,
			ResponseHeaderTimeout: requestTimeout,
			ExpectContinueTimeout: time.Second,
			// Each fetch is a one-off to an arbitrary host, so pooled idle
			// connections would only pile up.
			DisableKeepAlives: true,
		},
		Timeout: requestTimeout,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			return nil
		},
	}
	fetcher.client = client
	return fetcher
}

// Fetch returns the title of the page at rawURL. It reports an error when the
// request fails or the response is not an HTML page a title can be read from.
// A page without a title returns an empty title and no error.
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse URL: %w", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if (scheme != "http" && scheme != "https") || parsed.Hostname() == "" {
		return "", errors.New("URL must be absolute and use http or https")
	}

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	request.Header.Set("User-Agent", UserAgent)
	request.Header.Set("Accept", "text/html,application/xhtml+xml")

	response, err := f.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %s", response.Status)
	}
	if contentType := response.Header.Get("Content-Type"); contentType != "" && !isHTML(contentType) {
		return "", fmt.Errorf("unexpected content type %q", contentType)
	}

	return parseTitle(io.LimitReader(response.Body, maxResponseBytes))
}

// isHTML reports whether contentType names an HTML document. A missing header
// is not rejected, because some servers omit it and the tokenizer simply finds
// no title in a page that is not HTML.
func isHTML(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return strings.Contains(strings.ToLower(contentType), "html")
	}
	return mediaType == "text/html" || mediaType == "application/xhtml+xml"
}

// parseTitle streams HTML from r and returns its og:title, or its <title> when
// there is no og:title. It stops reading at <body> or </head>, so a large page
// body is never read.
func parseTitle(r io.Reader) (string, error) {
	tokenizer := html.NewTokenizer(r)

	var title, ogTitle string
	// firstTitle returns the best title found so far.
	firstTitle := func() string {
		if ogTitle != "" {
			return ogTitle
		}
		return title
	}

	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			if err := tokenizer.Err(); err != nil && !errors.Is(err, io.EOF) {
				return "", err
			}
			return firstTitle(), nil
		case html.EndTagToken:
			if name, _ := tokenizer.TagName(); string(name) == "head" {
				return firstTitle(), nil
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := tokenizer.TagName()
			switch string(name) {
			case "body":
				return firstTitle(), nil
			case "meta":
				if ogTitle == "" {
					ogTitle = metaTitle(tokenizer, hasAttr)
				}
			case "title":
				if title == "" {
					if tokenizer.Next() == html.TextToken {
						title = cleanTitle(string(tokenizer.Text()))
					}
				}
			}
		}
	}
}

// metaTitle returns the content of a meta tag when the tag is an og:title, and
// an empty string otherwise.
func metaTitle(tokenizer *html.Tokenizer, hasAttr bool) string {
	var isOpenGraph bool
	var content string
	for {
		key, value, more := tokenizer.TagAttr()
		switch strings.ToLower(string(key)) {
		case "property", "name":
			if strings.EqualFold(string(value), "og:title") {
				isOpenGraph = true
			}
		case "content":
			content = string(value)
		}
		if !more {
			break
		}
	}
	if !isOpenGraph {
		return ""
	}
	return cleanTitle(content)
}

// cleanTitle collapses whitespace, trims and cuts the title to maxTitleLength
// characters. The tokenizer has already decoded entities, so nothing is
// unescaped here.
func cleanTitle(title string) string {
	title = strings.Join(strings.Fields(title), " ")
	runes := []rune(title)
	if len(runes) > maxTitleLength {
		title = string(runes[:maxTitleLength])
	}
	return title
}

// dialContext dials address, refusing connections the private-address guard
// blocks. The check runs at connect time, so it covers redirects and DNS
// rebinding.
func (f *Fetcher) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout: requestTimeout,
		Control: f.control,
	}
	return dialer.DialContext(ctx, network, address)
}

// control is the net.Dialer.Control hook. It gets the resolved address, so it
// sees the IP a hostname actually points at.
func (f *Fetcher) control(_ string, address string, _ syscall.RawConn) error {
	if f.AllowPrivateAddresses || (f.allowAddress != nil && f.allowAddress(address)) {
		return nil
	}

	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("parse dial address %q: %w", address, err)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("unparseable dial address %q", address)
	}
	if blockedIP(ip) {
		return fmt.Errorf("refusing to connect to private address %s", host)
	}
	return nil
}

// blockedIP reports whether ip must not be connected to. IPv4-mapped IPv6
// addresses are reduced to IPv4 first, so the loopback, private and link-local
// checks cover them too. 169.254.169.254, the cloud metadata service, is
// link-local and therefore blocked.
func blockedIP(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified() ||
		ip.Equal(net.IPv4bcast)
}
