package linktitle

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	// maxOEmbedBytes is how much of an oEmbed response is read. oEmbed
	// documents are small, and the limit keeps a misbehaving endpoint from
	// streaming an unbounded body.
	maxOEmbedBytes = 64 << 10

	// youTubeOEmbedEndpoint returns a video's title without loading the video
	// page. YouTube answers page requests from data-centre addresses with a
	// bot-check page whose title is not the video's.
	youTubeOEmbedEndpoint = "https://www.youtube.com/oembed"
)

// site holds the title-lookup rules for one site.
type site struct {
	// hosts matches a hostname exactly or as a parent domain, so youtube.com
	// matches www.youtube.com and m.youtube.com but not notyoutube.com.
	hosts []string

	// title tries to return the page's title without loading it. A nil func,
	// an error or an empty title falls back to the normal page fetch.
	title func(ctx context.Context, fetcher *Fetcher, pageURL *url.URL) (string, error)

	// junkTitles are titles, after cleanTitle, that mean the page had no real
	// title, usually because it was a bot check. A matching page-fetch title
	// is replaced by an empty one.
	junkTitles []string
}

// sites is the per-site rules table, in match order.
var sites = []site{
	{
		hosts:      []string{"youtube.com", "youtu.be"},
		title:      youTubeTitle,
		junkTitles: []string{"- YouTube"},
	},
	{
		// Reddit serves some requests a bot-check page titled "Reddit".
		hosts:      []string{"reddit.com"},
		junkTitles: []string{"Reddit"},
	},
}

// youTubeTitle fetches a video's title from YouTube's oEmbed API.
func youTubeTitle(ctx context.Context, fetcher *Fetcher, pageURL *url.URL) (string, error) {
	return fetcher.fetchOEmbed(ctx, youTubeOEmbedEndpoint, pageURL.String())
}

// siteFor returns the rule for host, or nil when no rule matches.
func (f *Fetcher) siteFor(host string) *site {
	host = strings.ToLower(host)
	for i := range f.sites {
		if f.sites[i].matchesHost(host) {
			return &f.sites[i]
		}
	}
	return nil
}

// matchesHost reports whether host is one of the site's hosts or a subdomain
// of one.
func (s *site) matchesHost(host string) bool {
	host = strings.ToLower(host)
	for _, candidate := range s.hosts {
		candidate = strings.ToLower(candidate)
		if host == candidate || strings.HasSuffix(host, "."+candidate) {
			return true
		}
	}
	return false
}

// isJunkTitle reports whether title, as returned by the page fetch, means the
// page had no real title.
func (s *site) isJunkTitle(title string) bool {
	for _, junk := range s.junkTitles {
		if title == cleanTitle(junk) {
			return true
		}
	}
	return false
}

// fetchOEmbed asks an oEmbed endpoint for pageURL's title. The response is read
// through the fetcher's client, so the timeout, redirect limit and
// private-address guard apply here too.
func (f *Fetcher) fetchOEmbed(ctx context.Context, endpoint, pageURL string) (string, error) {
	if override, ok := f.oEmbedEndpoints[endpoint]; ok && override != "" {
		endpoint = override
	}

	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse oEmbed endpoint: %w", err)
	}
	query := endpointURL.Query()
	query.Set("url", pageURL)
	query.Set("format", "json")
	endpointURL.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpointURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("build oEmbed request: %w", err)
	}
	request.Header.Set("User-Agent", UserAgent)
	request.Header.Set("Accept", "application/json")

	response, err := f.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected oEmbed status %s", response.Status)
	}

	var payload struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxOEmbedBytes)).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode oEmbed response: %w", err)
	}
	return cleanTitle(payload.Title), nil
}
