package web

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/skorokithakis/soliloquy/internal/db"
)

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

	for _, want := range []string{
		`<link rel="icon" href="/static/favicon.svg" type="image/svg+xml">`,
		`<link rel="icon" href="/favicon.ico" sizes="16x16 32x32" type="image/x-icon">`,
		`<link rel="apple-touch-icon" href="/static/apple-touch-icon.png" sizes="180x180">`,
		`<meta property="og:title" content="Home">`,
		`<meta property="og:site_name" content="` + db.DefaultForumName + `">`,
		`<meta property="og:type" content="website">`,
		`<meta property="og:image" content="` + testBaseURL + `/static/og.png">`,
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
