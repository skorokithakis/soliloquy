package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestLoginPageDesign(t *testing.T) {
	body := get(t, "/login").Body.String()
	for _, want := range []string{
		`class="card auth-card"`,
		"a private forum · members only",
		"tty1 · forum.test",
		`action="/login"`,
		`name="username"`,
		`name="password"`,
		">log in</button>",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("login page does not contain %q", want)
		}
	}
}

func TestLoginThrottleShowsCountdownButton(t *testing.T) {
	server := newTestServer(t)
	form := url.Values{"username": {"alice"}, "password": {"wrong"}}

	response := postForm(t, server, "/login", form, nil)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("failed login status = %d, want %d", response.Code, http.StatusUnauthorized)
	}

	response = postForm(t, server, "/login", form, nil)
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("throttled login status = %d, want %d", response.Code, http.StatusTooManyRequests)
	}
	body := response.Body.String()
	for _, want := range []string{
		`data-wait-seconds="1"`,
		"wait 1s",
		"Too many attempts",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("throttled login page does not contain %q", want)
		}
	}
}

func TestInviteSignupPageShowsChainAndChecks(t *testing.T) {
	server := newTestServer(t)
	admin := createTestUser(t, server, "admin", true)
	if err := server.db.CreateInvite(context.Background(), "join-token", &admin.ID, false, server.now()); err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	body := doRequest(t, server, http.MethodGet, "/invite/join-token", nil, nil, "").Body.String()
	for _, want := range []string{
		`class="card auth-card"`,
		"<b>admin</b> invited you",
		"expires in 7 days",
		"└── <b>you</b>",
		`name="confirm_password"`,
		`data-username-check`,
		`data-check-url="/invite/join-token/check"`,
		"3 to 20 characters",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("signup page does not contain %q", want)
		}
	}
}

func TestInviteSignupChainIncludesAncestors(t *testing.T) {
	server := newTestServer(t)
	admin := createTestUser(t, server, "admin", true)
	bob := createInvitedTestUser(t, server, "bob", false, admin)
	if err := server.db.CreateInvite(context.Background(), "join-token", &bob.ID, false, server.now()); err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	body := doRequest(t, server, http.MethodGet, "/invite/join-token", nil, nil, "").Body.String()
	for _, want := range []string{
		"bob</b> invited you",
		"admin\n└── bob\n    └── <b>you</b>",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("signup chain does not contain %q", want)
		}
	}
}

func TestInviteUsernameCheckEndpoint(t *testing.T) {
	server := newTestServer(t)
	admin := createTestUser(t, server, "admin", true)
	ctx := context.Background()
	if err := server.db.CreateInvite(ctx, "join-token", &admin.ID, false, server.now()); err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	check := func(token, username string) (*http.Response, usernameCheckResponse) {
		t.Helper()
		response := doRequest(t, server, http.MethodGet, "/invite/"+token+"/check?username="+url.QueryEscape(username), nil, nil, "")
		var result usernameCheckResponse
		if response.Code == http.StatusOK {
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatalf("decode username check: %v", err)
			}
		}
		return response.Result(), result
	}

	response, result := check("join-token", "bob")
	if response.StatusCode != http.StatusOK || !result.Valid || !result.Available {
		t.Errorf("available username = %+v, status %d", result, response.StatusCode)
	}

	createTestUser(t, server, "bob", false)
	response, result = check("join-token", "bob")
	if response.StatusCode != http.StatusOK || !result.Valid || result.Available {
		t.Errorf("taken username = %+v, status %d", result, response.StatusCode)
	}

	response, result = check("join-token", "x")
	if response.StatusCode != http.StatusOK || result.Valid || result.Available {
		t.Errorf("invalid username = %+v, status %d", result, response.StatusCode)
	}

	response, _ = check("missing-token", "bob")
	if response.StatusCode != http.StatusNotFound {
		t.Errorf("invalid invite status = %d, want %d", response.StatusCode, http.StatusNotFound)
	}
}

func TestSignupRejectsPasswordMismatch(t *testing.T) {
	server := newTestServer(t)
	admin := createTestUser(t, server, "admin", true)
	if err := server.db.CreateInvite(context.Background(), "join-token", &admin.ID, false, server.now()); err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	response := postForm(t, server, "/invite/join-token", url.Values{
		"username":         {"bob"},
		"password":         {testPassword},
		"confirm_password": {"different"},
	}, nil)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("mismatch status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if !strings.Contains(response.Body.String(), "Passwords do not match.") {
		t.Error("mismatch error message is missing")
	}
	if user, err := server.db.UserByUsername(context.Background(), "bob"); err != nil || user != nil {
		t.Errorf("mismatched signup created a user: %+v, %v", user, err)
	}
}

func TestResetPageUsesAuthCard(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	if err := server.db.CreatePasswordReset(context.Background(), "reset-token", alice.ID, server.now()); err != nil {
		t.Fatalf("CreatePasswordReset: %v", err)
	}

	body := doRequest(t, server, http.MethodGet, "/reset/reset-token", nil, nil, "").Body.String()
	for _, want := range []string{
		`class="card auth-card"`,
		"reset password",
		`name="password"`,
		`name="confirm_password"`,
		"set password",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("reset page does not contain %q", want)
		}
	}
}
