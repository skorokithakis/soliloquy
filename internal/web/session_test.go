package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/skorokithakis/soliloquy/internal/auth"
	"github.com/skorokithakis/soliloquy/internal/config"
)

func TestSessionExpiryExtendsAtMostOncePerDay(t *testing.T) {
	server := newTestServer(t)
	base := time.Unix(1_700_000_000, 0).UTC()
	server.now = func() time.Time { return base }

	user := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, user)
	tokenHash := auth.HashToken(cookie.Value)

	expiry := func() int64 {
		t.Helper()
		var value int64
		if err := server.db.QueryRow(
			"SELECT expires_at FROM sessions WHERE token_hash = ?", tokenHash,
		).Scan(&value); err != nil {
			t.Fatalf("read session expiry: %v", err)
		}
		return value
	}

	initial := expiry()
	if want := base.Add(auth.SessionLifetime).Unix(); initial != want {
		t.Fatalf("initial expiry = %d, want %d", initial, want)
	}

	// Less than a day old: the expiry must not be rewritten.
	server.now = func() time.Time { return base.Add(12 * time.Hour) }
	if response := doRequest(t, server, http.MethodGet, "/", nil, cookie, ""); response.Code != http.StatusOK {
		t.Fatalf("home at 12h: status = %d", response.Code)
	}
	if got := expiry(); got != initial {
		t.Errorf("expiry after 12h = %d, want %d", got, initial)
	}

	// More than a day old: the sliding expiry moves forward.
	extendedAt := base.Add(25 * time.Hour)
	server.now = func() time.Time { return extendedAt }
	if response := doRequest(t, server, http.MethodGet, "/", nil, cookie, ""); response.Code != http.StatusOK {
		t.Fatalf("home at 25h: status = %d", response.Code)
	}
	extended := expiry()
	if want := extendedAt.Add(auth.SessionLifetime).Unix(); extended != want {
		t.Fatalf("expiry after 25h = %d, want %d", extended, want)
	}

	// One hour later the expiry is still fresh, so it is not written again.
	server.now = func() time.Time { return base.Add(26 * time.Hour) }
	if response := doRequest(t, server, http.MethodGet, "/", nil, cookie, ""); response.Code != http.StatusOK {
		t.Fatalf("home at 26h: status = %d", response.Code)
	}
	if got := expiry(); got != extended {
		t.Errorf("expiry after 26h = %d, want %d", got, extended)
	}
}

func TestSessionCookieFlags(t *testing.T) {
	server, err := NewServer(newTestDatabase(t), config.Config{BaseURL: "https://forum.test"})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	createTestUser(t, server, "alice", false)

	response := doRequest(t, server, http.MethodPost, "/login", url.Values{
		"username": {"alice"},
		"password": {testPassword},
	}, nil, "https://forum.test")
	if response.Code != http.StatusSeeOther {
		t.Fatalf("login status = %d, want %d", response.Code, http.StatusSeeOther)
	}

	for _, cookie := range response.Result().Cookies() {
		if cookie.Name != sessionCookieName {
			continue
		}
		if !cookie.Secure {
			t.Error("session cookie is not Secure on an https base URL")
		}
		if !cookie.HttpOnly {
			t.Error("session cookie is not HttpOnly")
		}
		if cookie.SameSite != http.SameSiteLaxMode {
			t.Errorf("SameSite = %v, want Lax", cookie.SameSite)
		}
		if cookie.Path != "/" {
			t.Errorf("Path = %q, want /", cookie.Path)
		}
		return
	}
	t.Fatal("login did not set a session cookie")
}

func TestCompleteLoginRejectsPasswordChangedDuringLogin(t *testing.T) {
	server := newTestServer(t)
	user := createTestUser(t, server, "alice", false)

	stale := *user
	stale.PasswordHash = "stale-hash"
	response := httptest.NewRecorder()
	created, err := server.completeLogin(response, httptest.NewRequest(http.MethodGet, "/", nil), &stale, server.now())
	if err != nil {
		t.Fatalf("completeLogin with stale hash: %v", err)
	}
	if created {
		t.Fatal("created a session after the password changed during login")
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == sessionCookieName {
			t.Error("stale login set a session cookie")
		}
	}

	var count int
	if err := server.db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&count); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if count != 0 {
		t.Errorf("sessions = %d, want 0", count)
	}

	// The rejected race counts as a failed login.
	if allowed, _ := server.throttle.Allowed(user.Username); allowed {
		t.Error("stale login did not leave the username throttled")
	}

	// The same helper succeeds when the verified hash is still current.
	response = httptest.NewRecorder()
	created, err = server.completeLogin(response, httptest.NewRequest(http.MethodGet, "/", nil), user, server.now())
	if err != nil || !created {
		t.Fatalf("completeLogin with current hash: created = %v, err = %v", created, err)
	}
	sessionCookie(t, response)
}

func TestDeletedUserCannotLogIn(t *testing.T) {
	server := newTestServer(t)
	user := createTestUser(t, server, "alice", false)
	if _, err := server.db.Exec(
		"UPDATE users SET deleted_at = ? WHERE id = ?", server.now().Unix(), user.ID,
	); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	response := postForm(t, server, "/login", url.Values{
		"username": {"alice"},
		"password": {testPassword},
	}, nil)
	if response.Code != http.StatusUnauthorized {
		t.Errorf("deleted login status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}
