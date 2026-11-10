package linktitle

import (
	"net/url"
	"testing"
)

// TestSitesOEmbedEndpoints checks the shape of every oEmbed rule. The
// endpoints are contacted in production, so a typo or a missing scheme would
// only show up as a failed title lookup.
func TestSitesOEmbedEndpoints(t *testing.T) {
	for _, site := range sites {
		if site.oEmbed == "" {
			continue
		}

		endpoint, err := url.Parse(site.oEmbed)
		if err != nil {
			t.Errorf("parse oEmbed endpoint %q: %v", site.oEmbed, err)
			continue
		}
		if endpoint.Scheme != "https" {
			t.Errorf("oEmbed endpoint %q has scheme %q, want https", site.oEmbed, endpoint.Scheme)
		}
		if !endpoint.IsAbs() || endpoint.Host == "" {
			t.Errorf("oEmbed endpoint %q is not absolute", site.oEmbed)
		}
		if len(site.hosts) == 0 {
			t.Errorf("oEmbed endpoint %q has no hosts", site.oEmbed)
		}
	}
}
