package web

import (
	"bytes"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skorokithakis/soliloquy/internal/db"
)

var testStaticAssets = []struct {
	logical     string
	contentType string
}{
	{logical: "style.css", contentType: "text/css"},
	{logical: "app.js", contentType: "javascript"},
	{logical: "favicon.svg", contentType: "image/svg+xml"},
	{logical: "apple-touch-icon.png", contentType: "image/png"},
	{logical: "og.png", contentType: "image/png"},
}

func TestFaviconServedWhenLoggedOut(t *testing.T) {
	response := get(t, "/favicon.ico")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Content-Type"); got != "image/x-icon" {
		t.Errorf("Content-Type = %q, want image/x-icon", got)
	}
	body := response.Body.Bytes()
	// ICO files start with a reserved zero word followed by the icon type.
	if len(body) < 4 || !bytes.Equal(body[:4], []byte{0, 0, 1, 0}) {
		t.Errorf("body is not an ICO file: %x", body[:min(len(body), 4)])
	}
}

func TestLayoutHasIconsAndOpenGraphMetadata(t *testing.T) {
	server := newTestServer(t)
	user := createTestUser(t, server, "alice", false)
	body := doRequest(t, server, http.MethodGet, "/", nil, signIn(t, server, user), "").Body.String()

	faviconURL := staticURL(t, "favicon.svg")
	appleTouchIconURL := staticURL(t, "apple-touch-icon.png")
	ogImageURL := staticURL(t, "og.png")

	for _, want := range []string{
		`<link rel="icon" href="` + faviconURL + `" type="image/svg+xml">`,
		`<link rel="icon" href="/favicon.ico" sizes="16x16 32x32" type="image/x-icon">`,
		`<link rel="apple-touch-icon" href="` + appleTouchIconURL + `" sizes="180x180">`,
		`<meta property="og:title" content="Home">`,
		`<meta property="og:site_name" content="` + db.DefaultForumName + `">`,
		`<meta property="og:type" content="website">`,
		`<meta property="og:image" content="` + testBaseURL + ogImageURL + `">`,
		`<meta name="twitter:card" content="summary_large_image">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
	}

	if strings.Contains(body, `property="og:description"`) {
		t.Error("body contains an og:description tag")
	}
}

func TestLayoutLinksHashedStaticAssets(t *testing.T) {
	server := newTestServer(t)
	user := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, user)

	pages := []struct {
		name   string
		path   string
		cookie *http.Cookie
	}{
		{name: "logged in", path: "/", cookie: cookie},
		{name: "logged out", path: "/login"},
	}
	for _, page := range pages {
		body := doRequest(t, server, http.MethodGet, page.path, nil, page.cookie, "").Body.String()
		for _, asset := range testStaticAssets {
			url := staticURL(t, asset.logical)
			if !strings.Contains(body, url) {
				t.Errorf("%s page does not link %s", page.name, url)
			}
		}
	}
}

func TestHashedStaticAssetsServeImmutableContent(t *testing.T) {
	server := newTestServer(t)

	for _, asset := range testStaticAssets {
		t.Run(asset.logical, func(t *testing.T) {
			url := staticURL(t, asset.logical)
			response := doRequest(t, server, http.MethodGet, url, nil, nil, "")

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
			}
			if got := response.Header().Get("Cache-Control"); got != immutableCacheControl {
				t.Errorf("Cache-Control = %q, want %q", got, immutableCacheControl)
			}
			if got := response.Header().Get("Content-Type"); !strings.Contains(got, asset.contentType) {
				t.Errorf("Content-Type = %q, want %q", got, asset.contentType)
			}

			content, err := fs.ReadFile(assets, "static/"+asset.logical)
			if err != nil {
				t.Fatalf("read embedded %s: %v", asset.logical, err)
			}
			if !bytes.Equal(response.Body.Bytes(), content) {
				t.Errorf("body does not match %s", asset.logical)
			}
		})
	}
}

func TestUnhashedStaticAssetsHaveNoImmutableCacheControl(t *testing.T) {
	server := newTestServer(t)

	for _, path := range []string{
		"/static/style.css",
		"/static/fonts/ibm-plex-mono-latin-400.woff2",
	} {
		response := doRequest(t, server, http.MethodGet, path, nil, nil, "")
		if response.Code != http.StatusOK {
			t.Errorf("%s status = %d, want %d", path, response.Code, http.StatusOK)
		}
		if got := response.Header().Get("Cache-Control"); got != "" {
			t.Errorf("%s Cache-Control = %q, want empty", path, got)
		}
	}
}

func TestWrongStaticHashReturnsNotFound(t *testing.T) {
	server := newTestServer(t)

	for _, path := range []string{
		"/static/style.00000000.css",
		"/static/app.00000000.js",
	} {
		response := doRequest(t, server, http.MethodGet, path, nil, nil, "")
		if response.Code != http.StatusNotFound {
			t.Errorf("%s status = %d, want %d", path, response.Code, http.StatusNotFound)
		}
	}
}

func TestStaticHashesAreStableAcrossRestarts(t *testing.T) {
	first := loadTestStaticAssets(t)
	second := loadTestStaticAssets(t)

	for _, asset := range testStaticAssets {
		firstURL, err := first.URL(asset.logical)
		if err != nil {
			t.Fatalf("static URL from first load for %q: %v", asset.logical, err)
		}
		secondURL, err := second.URL(asset.logical)
		if err != nil {
			t.Fatalf("static URL from second load for %q: %v", asset.logical, err)
		}
		if firstURL != secondURL {
			t.Errorf("%s URL differs between loads: %q != %q", asset.logical, firstURL, secondURL)
		}
	}
}

func TestStaticURLRejectsUnknownAsset(t *testing.T) {
	assets := loadTestStaticAssets(t)

	if _, err := assets.URL("missing.css"); err == nil {
		t.Error("URL returned no error for an unknown asset")
	}
}

func TestHashedStaticErrorResponsesHaveNoImmutableCacheControl(t *testing.T) {
	server := newTestServer(t)
	url := staticURL(t, "style.css")

	tests := []struct {
		name       string
		header     string
		value      string
		wantStatus int
	}{
		{name: "failed If-Match", header: "If-Match", value: `"x"`, wantStatus: http.StatusPreconditionFailed},
		{name: "unsatisfiable range", header: "Range", value: "bytes=999999999-", wantStatus: http.StatusRequestedRangeNotSatisfiable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, url, nil)
			request.Header.Set(test.header, test.value)
			response := httptest.NewRecorder()
			server.Handler().ServeHTTP(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			if got := response.Header().Get("Cache-Control"); got != "" {
				t.Errorf("Cache-Control = %q, want empty", got)
			}
		})
	}
}

func loadTestStaticAssets(t *testing.T) *assetSet {
	t.Helper()

	staticFS, err := fs.Sub(assets, "static")
	if err != nil {
		t.Fatalf("open embedded static assets: %v", err)
	}
	set, err := loadStaticAssets(staticFS)
	if err != nil {
		t.Fatalf("load static assets: %v", err)
	}
	return set
}

func staticURL(t *testing.T, logical string) string {
	t.Helper()

	url, err := loadTestStaticAssets(t).URL(logical)
	if err != nil {
		t.Fatalf("static URL for %q: %v", logical, err)
	}
	return url
}
