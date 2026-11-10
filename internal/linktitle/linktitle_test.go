package linktitle

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

func TestFetchTitle(t *testing.T) {
	tests := []struct {
		name string
		page string
		want string
	}{
		{
			name: "og title wins",
			page: `<html><head><meta property="og:title" content="Open &amp; Graph"><title>Document</title></head><body></body></html>`,
			want: "Open & Graph",
		},
		{
			name: "og title before title wins",
			page: `<html><head><title>Document</title><meta property="og:title" content="Open Graph"></head><body></body></html>`,
			want: "Open Graph",
		},
		{
			name: "og title via name attribute",
			page: `<html><head><meta name="og:title" content="Named"></head><body></body></html>`,
			want: "Named",
		},
		{
			name: "title fallback collapses whitespace and decodes entities",
			page: "<html><head><title>\n  Caf&eacute;   &amp;\n\t bar </title></head><body></body></html>",
			want: "Café & bar",
		},
		{
			name: "og title without content falls back to title",
			page: `<html><head><meta property="og:title"><title>Document</title></head><body></body></html>`,
			want: "Document",
		},
		{
			name: "no title",
			page: `<html><head><meta name="description" content="x"></head><body></body></html>`,
			want: "",
		},
		{
			name: "stops at body",
			page: `<html><head><title>Head</title><body><title>Body</title></body></html>`,
			want: "Head",
		},
		{
			name: "long title is cut to 200 characters",
			page: "<html><head><title>" + strings.Repeat("é", 250) + "</title></head><body></body></html>",
			want: strings.Repeat("é", 200),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				io.WriteString(w, test.page)
			}))
			defer server.Close()

			fetcher := NewFetcher()
			fetcher.AllowPrivateAddresses = true

			title, err := fetcher.Fetch(context.Background(), server.URL)
			if err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			if title != test.want {
				t.Errorf("title = %q, want %q", title, test.want)
			}
			if utf8.RuneCountInString(title) > maxTitleLength {
				t.Errorf("title has %d characters, want at most %d", utf8.RuneCountInString(title), maxTitleLength)
			}
		})
	}
}

func TestFetchRejectsFailures(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		contentType string
	}{
		{name: "non-200", status: http.StatusNotFound, contentType: "text/html"},
		{name: "non-HTML", status: http.StatusOK, contentType: "application/json"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", test.contentType)
				w.WriteHeader(test.status)
				io.WriteString(w, `<html><head><title>Whatever</title></head></html>`)
			}))
			defer server.Close()

			fetcher := NewFetcher()
			fetcher.AllowPrivateAddresses = true

			title, err := fetcher.Fetch(context.Background(), server.URL)
			if err == nil {
				t.Fatal("Fetch succeeded, want error")
			}
			if title != "" {
				t.Errorf("title = %q, want empty", title)
			}
		})
	}
}

func TestFetchParsesPageWithoutContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header()["Content-Type"] = nil
		io.WriteString(w, `<html><head><title>Whatever</title></head></html>`)
	}))
	defer server.Close()

	fetcher := NewFetcher()
	fetcher.AllowPrivateAddresses = true

	title, err := fetcher.Fetch(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if title != "Whatever" {
		t.Errorf("title = %q, want %q", title, "Whatever")
	}
}

func TestFetchRejectsBadURLs(t *testing.T) {
	for _, rawURL := range []string{
		"",
		"/relative",
		"ftp://example.com/",
		"javascript:alert(1)",
		"http://",
	} {
		if _, err := NewFetcher().Fetch(context.Background(), rawURL); err == nil {
			t.Errorf("Fetch(%q) succeeded, want error", rawURL)
		}
	}
}

func TestFetchRefusesPrivateAddresses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `<html><head><title>Secret</title></head></html>`)
	}))
	defer server.Close()

	// The guard stays on, so the httptest server on 127.0.0.1 is refused.
	title, err := NewFetcher().Fetch(context.Background(), server.URL)
	if err == nil {
		t.Fatal("Fetch succeeded, want the private-address guard to refuse the connection")
	}
	if title != "" {
		t.Errorf("title = %q, want empty", title)
	}
}

func TestFetchYouTubeOEmbed(t *testing.T) {
	const pageURL = "https://www.youtube.com/watch?v=rpV4phLQsE4"

	var gotURL, gotFormat, gotUserAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.Query().Get("url")
		gotFormat = r.URL.Query().Get("format")
		gotUserAgent = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"title":"  Real   Video Title "}`)
	}))
	defer server.Close()

	fetcher := NewFetcher()
	fetcher.AllowPrivateAddresses = true
	fetcher.oEmbedEndpoints = map[string]string{youTubeOEmbedEndpoint: server.URL}

	title, err := fetcher.Fetch(context.Background(), pageURL)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if title != "Real Video Title" {
		t.Errorf("title = %q, want %q", title, "Real Video Title")
	}
	if gotURL != pageURL {
		t.Errorf("oEmbed url = %q, want %q", gotURL, pageURL)
	}
	if gotFormat != "json" {
		t.Errorf("oEmbed format = %q, want %q", gotFormat, "json")
	}
	if gotUserAgent != UserAgent {
		t.Errorf("oEmbed User-Agent = %q, want %q", gotUserAgent, UserAgent)
	}
}

func TestFetchYouTubeOEmbedFallsBackToPage(t *testing.T) {
	tests := []struct {
		name           string
		oEmbedResponse string
		oEmbedStatus   int
		pageTitle      string
		want           string
	}{
		{
			name:           "oEmbed error",
			oEmbedStatus:   http.StatusInternalServerError,
			oEmbedResponse: `{"title":"Ignored"}`,
			pageTitle:      "Page Title",
			want:           "Page Title",
		},
		{
			name:           "oEmbed without title",
			oEmbedStatus:   http.StatusOK,
			oEmbedResponse: `{"title":""}`,
			pageTitle:      "Page Title",
			want:           "Page Title",
		},
		{
			name:           "page title is a YouTube bot check",
			oEmbedStatus:   http.StatusInternalServerError,
			oEmbedResponse: `{}`,
			pageTitle:      " - YouTube ",
			want:           "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			oEmbed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.oEmbedStatus)
				io.WriteString(w, test.oEmbedResponse)
			}))
			defer oEmbed.Close()

			page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				io.WriteString(w, `<html><head><title>`+test.pageTitle+`</title></head><body></body></html>`)
			}))
			defer page.Close()

			fetcher := NewFetcher()
			fetcher.AllowPrivateAddresses = true
			fetcher.oEmbedEndpoints = map[string]string{youTubeOEmbedEndpoint: oEmbed.URL}
			// Point the real YouTube rule at the test page server.
			fetcher.sites = []site{{
				hosts:      []string{"127.0.0.1"},
				title:      youTubeTitle,
				junkTitles: []string{"- YouTube"},
			}}

			title, err := fetcher.Fetch(context.Background(), page.URL)
			if err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			if title != test.want {
				t.Errorf("title = %q, want %q", title, test.want)
			}
		})
	}
}

func TestFetchJunkTitleReplacedWithEmpty(t *testing.T) {
	tests := []struct {
		name      string
		junk      []string
		pageTitle string
		want      string
	}{
		{name: "junk title", junk: []string{"Reddit"}, pageTitle: "Reddit", want: ""},
		{name: "real title", junk: []string{"Reddit"}, pageTitle: "A Reddit thread", want: "A Reddit thread"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				io.WriteString(w, `<html><head><title>`+test.pageTitle+`</title></head><body></body></html>`)
			}))
			defer page.Close()

			fetcher := NewFetcher()
			fetcher.AllowPrivateAddresses = true
			fetcher.sites = []site{{hosts: []string{"127.0.0.1"}, junkTitles: test.junk}}

			title, err := fetcher.Fetch(context.Background(), page.URL)
			if err != nil {
				t.Fatalf("Fetch: %v", err)
			}
			if title != test.want {
				t.Errorf("title = %q, want %q", title, test.want)
			}
		})
	}
}

func TestSiteMatchesHost(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{host: "youtube.com", want: true},
		{host: "www.youtube.com", want: true},
		{host: "m.youtube.com", want: true},
		{host: "music.youtube.com", want: true},
		{host: "youtu.be", want: true},
		{host: "www.youtu.be", want: true},
		{host: "notyoutube.com", want: false},
		{host: "youtube.com.example.com", want: false},
		{host: "example.com", want: false},
	}

	for _, test := range tests {
		t.Run(test.host, func(t *testing.T) {
			if got := sites[0].matchesHost(test.host); got != test.want {
				t.Errorf("matchesHost(%q) = %t, want %t", test.host, got, test.want)
			}
		})
	}
}

func TestBlockedIP(t *testing.T) {
	tests := []struct {
		ip      string
		blocked bool
	}{
		{ip: "127.0.0.1", blocked: true},
		{ip: "127.1.2.3", blocked: true},
		{ip: "10.0.0.1", blocked: true},
		{ip: "172.16.0.1", blocked: true},
		{ip: "192.168.1.1", blocked: true},
		{ip: "169.254.169.254", blocked: true},
		{ip: "0.0.0.0", blocked: true},
		{ip: "255.255.255.255", blocked: true},
		{ip: "::1", blocked: true},
		{ip: "::", blocked: true},
		{ip: "fe80::1", blocked: true},
		{ip: "ff02::1", blocked: true},
		{ip: "::ffff:127.0.0.1", blocked: true},
		{ip: "::ffff:169.254.169.254", blocked: true},
		{ip: "93.184.216.34", blocked: false},
		{ip: "8.8.8.8", blocked: false},
		{ip: "2606:2800:220:1:248:1893:25c8:1946", blocked: false},
	}

	for _, test := range tests {
		t.Run(test.ip, func(t *testing.T) {
			ip := net.ParseIP(test.ip)
			if ip == nil {
				t.Fatalf("net.ParseIP(%q) = nil", test.ip)
			}
			if got := blockedIP(ip); got != test.blocked {
				t.Errorf("blockedIP(%s) = %t, want %t", test.ip, got, test.blocked)
			}
		})
	}
}

func TestFetchRefusesRedirectToPrivateAddress(t *testing.T) {
	var blockedHits atomic.Int32
	blocked := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		blockedHits.Add(1)
		io.WriteString(w, `<html><head><title>Secret</title></head></html>`)
	}))
	defer blocked.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, blocked.URL, http.StatusFound)
	}))
	defer redirector.Close()

	redirectorURL, err := url.Parse(redirector.URL)
	if err != nil {
		t.Fatalf("parse redirector URL: %v", err)
	}

	fetcher := NewFetcher()
	// Let the first connection through, as a real public site would be, and
	// leave the guard on for the redirect target.
	fetcher.allowAddress = func(address string) bool {
		return address == redirectorURL.Host
	}

	title, err := fetcher.Fetch(context.Background(), redirector.URL)
	if err == nil {
		t.Fatal("Fetch succeeded, want the redirect to a private address to be refused")
	}
	if title != "" {
		t.Errorf("title = %q, want empty", title)
	}
	if hits := blockedHits.Load(); hits != 0 {
		t.Errorf("redirect target was contacted %d times, want 0", hits)
	}
}

func TestFetchLimitsRedirects(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, "/again", http.StatusFound)
	}))
	defer server.Close()

	fetcher := NewFetcher()
	fetcher.AllowPrivateAddresses = true

	title, err := fetcher.Fetch(context.Background(), server.URL)
	if err == nil {
		t.Fatal("Fetch succeeded, want a redirect-limit error")
	}
	if title != "" {
		t.Errorf("title = %q, want empty", title)
	}
	if got := hits.Load(); got > int32(maxRedirects)+1 {
		t.Errorf("server was contacted %d times, want at most %d", got, maxRedirects+1)
	}
}

func TestFetchTimesOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	fetcher := NewFetcher()
	fetcher.AllowPrivateAddresses = true

	// A parent deadline shorter than the fetcher's own timeout must win, so
	// the test does not have to wait five seconds.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	started := time.Now()
	title, err := fetcher.Fetch(ctx, server.URL)
	if err == nil {
		t.Fatal("Fetch succeeded, want a timeout error")
	}
	if title != "" {
		t.Errorf("title = %q, want empty", title)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Errorf("Fetch took %v, want it to stop when the context expires", elapsed)
	}
}

func TestFetchLargePage(t *testing.T) {
	t.Run("title before a large body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			io.WriteString(w, `<html><head><title>Early</title></head><body>`)
			io.WriteString(w, strings.Repeat("x", 3*maxResponseBytes))
		}))
		defer server.Close()

		fetcher := NewFetcher()
		fetcher.AllowPrivateAddresses = true

		title, err := fetcher.Fetch(context.Background(), server.URL)
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		if title != "Early" {
			t.Errorf("title = %q, want %q", title, "Early")
		}
	})

	t.Run("title past the read limit", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			io.WriteString(w, `<html><head>`+strings.Repeat(" ", maxResponseBytes+1)+`<title>Late</title></head></html>`)
		}))
		defer server.Close()

		fetcher := NewFetcher()
		fetcher.AllowPrivateAddresses = true

		title, err := fetcher.Fetch(context.Background(), server.URL)
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		if title != "" {
			t.Errorf("title = %q, want empty because the page is larger than the read limit", title)
		}
	})
}

func TestFetchOEmbedRefusesPrivateEndpoint(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		io.WriteString(w, `{"title":"Secret"}`)
	}))
	defer server.Close()

	// The guard stays on, so the httptest endpoint on 127.0.0.1 is refused.
	_, err := NewFetcher().fetchOEmbed(context.Background(), server.URL, "https://www.youtube.com/watch?v=x")
	if err == nil {
		t.Fatal("fetchOEmbed succeeded, want the private-address guard to refuse the connection")
	}
	if got := hits.Load(); got != 0 {
		t.Errorf("endpoint was contacted %d times, want 0", got)
	}
}

func TestFetchOEmbedRefusesRedirectToPrivateAddress(t *testing.T) {
	var blockedHits atomic.Int32
	blocked := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		blockedHits.Add(1)
		io.WriteString(w, `{"title":"Secret"}`)
	}))
	defer blocked.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, blocked.URL, http.StatusFound)
	}))
	defer redirector.Close()

	redirectorURL, err := url.Parse(redirector.URL)
	if err != nil {
		t.Fatalf("parse redirector URL: %v", err)
	}

	fetcher := NewFetcher()
	// Let the first connection through, as a real public site would be, and
	// leave the guard on for the redirect target.
	fetcher.allowAddress = func(address string) bool {
		return address == redirectorURL.Host
	}

	_, err = fetcher.fetchOEmbed(context.Background(), redirector.URL, "https://www.youtube.com/watch?v=x")
	if err == nil {
		t.Fatal("fetchOEmbed succeeded, want the redirect to a private address to be refused")
	}
	if got := blockedHits.Load(); got != 0 {
		t.Errorf("redirect target was contacted %d times, want 0", got)
	}
}
