package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/skorokithakis/soliloquy/internal/db"
)

func TestAdminSettingsRequiresAdmin(t *testing.T) {
	server := newTestServer(t)
	user := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, user)

	response := doRequest(t, server, http.MethodGet, "/admin/settings", nil, cookie, "")
	if response.Code != http.StatusForbidden {
		t.Errorf("GET status = %d, want %d", response.Code, http.StatusForbidden)
	}

	response = postForm(t, server, "/admin/settings", url.Values{
		"forum_name": {"Hijacked"},
	}, cookie)
	if response.Code != http.StatusForbidden {
		t.Errorf("POST status = %d, want %d", response.Code, http.StatusForbidden)
	}
	stored, err := server.db.Settings(context.Background())
	if err != nil {
		t.Fatalf("Settings: %v", err)
	}
	if stored.ForumName != db.DefaultForumName {
		t.Errorf("normal user changed the forum name to %q", stored.ForumName)
	}
}

func TestAdminSettingsSave(t *testing.T) {
	server := newTestServer(t)
	admin := createTestUser(t, server, "admin", true)
	cookie := signIn(t, server, admin)

	page := doRequest(t, server, http.MethodGet, "/admin/settings", nil, cookie, "")
	if page.Code != http.StatusOK {
		t.Fatalf("settings page status = %d, want %d", page.Code, http.StatusOK)
	}
	for _, want := range []string{
		`action="/admin/settings"`,
		`name="forum_name"`,
		`value="` + db.DefaultForumName + `"`,
		`name="public"`,
		"visible to anyone without an account",
	} {
		if !strings.Contains(page.Body.String(), want) {
			t.Errorf("settings page does not contain %q", want)
		}
	}

	response := postForm(t, server, "/admin/settings", url.Values{
		"public":     {"on"},
		"forum_name": {"  The Forum  "},
	}, cookie)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/admin/settings" {
		t.Fatalf("save status = %d, Location = %q", response.Code, response.Header().Get("Location"))
	}

	stored, err := server.db.Settings(context.Background())
	if err != nil {
		t.Fatalf("Settings: %v", err)
	}
	if !stored.Public {
		t.Error("public setting was not saved")
	}
	if stored.ForumName != "The Forum" {
		t.Errorf("forum name = %q, want %q", stored.ForumName, "The Forum")
	}

	// The layout uses the saved name in the title and both logo links.
	home := doRequest(t, server, http.MethodGet, "/", nil, cookie, "").Body.String()
	for _, want := range []string{
		`<title>Home · The Forum</title>`,
		`aria-label="The Forum home"`,
		`<span class="p">›</span>The Forum<span class="cur"></span>`,
	} {
		if !strings.Contains(home, want) {
			t.Errorf("home page does not contain %q", want)
		}
	}
	if !strings.Contains(home, `powered by <a href="https://github.com/skorokithakis/soliloquy">soliloquy</a>`) {
		t.Error("footer no longer names the software")
	}

	page = doRequest(t, server, http.MethodGet, "/admin/settings", nil, cookie, "")
	if !strings.Contains(page.Body.String(), `name="public" checked`) {
		t.Error("settings page does not show the saved public flag")
	}
	if !strings.Contains(page.Body.String(), `value="The Forum"`) {
		t.Error("settings page does not show the saved forum name")
	}
}

func TestAdminSettingsRejectsInvalidForumName(t *testing.T) {
	server := newTestServer(t)
	admin := createTestUser(t, server, "admin", true)
	cookie := signIn(t, server, admin)

	for _, name := range []string{"", "   ", strings.Repeat("a", 41)} {
		response := postForm(t, server, "/admin/settings", url.Values{
			"public":     {"on"},
			"forum_name": {name},
		}, cookie)
		if response.Code != http.StatusBadRequest {
			t.Errorf("name %q: status = %d, want %d", name, response.Code, http.StatusBadRequest)
		}
		if !strings.Contains(response.Body.String(), "Forum name must be between 1 and 40 characters.") {
			t.Errorf("name %q: error message is missing", name)
		}
	}

	stored, err := server.db.Settings(context.Background())
	if err != nil {
		t.Fatalf("Settings: %v", err)
	}
	if stored.Public || stored.ForumName != db.DefaultForumName {
		t.Errorf("invalid form changed settings: %+v", stored)
	}

	// Exactly 40 characters is allowed.
	response := postForm(t, server, "/admin/settings", url.Values{
		"forum_name": {strings.Repeat("a", 40)},
	}, cookie)
	if response.Code != http.StatusSeeOther {
		t.Errorf("40-character name: status = %d, want %d", response.Code, http.StatusSeeOther)
	}
}
