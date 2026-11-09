package web

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestTreeRedirectsToInvites(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)

	response := doRequest(t, server, http.MethodGet, "/tree", nil, cookie, "")
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/invites" {
		t.Errorf("status = %d, Location = %q; want redirect to /invites", response.Code, response.Header().Get("Location"))
	}
}

func TestInvitesPageShowsTreeAndLinkStates(t *testing.T) {
	server := newTestServer(t)
	ctx := context.Background()
	now := server.now()

	admin := createTestUser(t, server, "admin", true)
	bob := createInvitedTestUser(t, server, "bob", false, admin)
	createInvitedTestUser(t, server, "anna", false, admin)

	for _, invite := range []struct {
		token string
		at    time.Time
	}{
		{token: "active-token-12345678", at: now},
		{token: "expired-token-1234567", at: now.Add(-8 * 24 * time.Hour)},
		{token: "used-carol-token-1234", at: now},
		{token: "used-dave-token-12345", at: now},
	} {
		if err := server.db.CreateInvite(ctx, invite.token, &bob.ID, false, invite.at); err != nil {
			t.Fatalf("CreateInvite %q: %v", invite.token, err)
		}
	}

	if _, err := server.db.RedeemInvite(ctx, "used-carol-token-1234", "carol", testPasswordHash, now); err != nil {
		t.Fatalf("RedeemInvite carol: %v", err)
	}
	dave, err := server.db.RedeemInvite(ctx, "used-dave-token-12345", "dave", testPasswordHash, now)
	if err != nil {
		t.Fatalf("RedeemInvite dave: %v", err)
	}
	eve := createInvitedTestUser(t, server, "eve", false, bob)
	if _, err := server.db.DisableUser(ctx, eve.ID, now); err != nil {
		t.Fatalf("disable eve: %v", err)
	}
	if err := server.db.DeleteAccount(ctx, dave.ID, now); err != nil {
		t.Fatalf("delete dave: %v", err)
	}

	cookie := signIn(t, server, bob)
	body := doRequest(t, server, http.MethodGet, "/invites", nil, cookie, "").Body.String()

	for _, want := range []string{
		">├── </span>",
		">│   ├── </span>",
		`class="nm">bob</span>`,
		`class="nm">carol</span>`,
		"[deleted user]",
		"5 accounts, 1 deleted · 1 disabled",
		`class="flag a">admin</span>`,
		`class="flag you">you</span>`,
		`class="flag">disabled</span>`,
		"unused · 7d left",
		"expired " + now.Add(-24*time.Hour).UTC().Format("2006-01-02"),
		"used</span> by carol",
		"used</span> by [deleted user]",
		`data-copy="http://forum.test/invite/active-token-12345678"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("invites page does not contain %q", want)
		}
	}

	if strings.Contains(body, "/admin/users/") {
		t.Error("invites page contains admin controls")
	}
}

func TestInvitesOlderLinksRemainReachable(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)
	ctx := context.Background()
	now := server.now()

	for _, invite := range []struct {
		token string
		at    time.Time
	}{
		{token: "older-invite-token", at: now},
		{token: "newer-invite-token", at: now.Add(time.Minute)},
	} {
		if err := server.db.CreateInvite(ctx, invite.token, &alice.ID, false, invite.at); err != nil {
			t.Fatalf("CreateInvite %q: %v", invite.token, err)
		}
	}

	body := doRequest(t, server, http.MethodGet, "/invites", nil, cookie, "").Body.String()
	for _, token := range []string{"older-invite-token", "newer-invite-token"} {
		url := testBaseURL + "/invite/" + token
		if !strings.Contains(body, `href="`+url+`"`) {
			t.Errorf("invites page does not link unused invite %q", token)
		}
	}
}

func TestInvitesPageEmptyLinks(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)

	body := doRequest(t, server, http.MethodGet, "/invites", nil, cookie, "").Body.String()
	for _, want := range []string{
		"Create a link to invite someone.",
		"You have no invite links yet.",
		"1 account, 0 deleted · 0 disabled",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("empty invites page does not contain %q", want)
		}
	}
}

func TestInvitesPageLinkSummary(t *testing.T) {
	server := newTestServer(t)
	alice := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, alice)

	response := postForm(t, server, "/invites", nil, cookie)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/invites" {
		t.Fatalf("create invite status = %d, Location = %q", response.Code, response.Header().Get("Location"))
	}

	invites, err := server.db.ListInvites(context.Background(), alice.ID)
	if err != nil || len(invites) != 1 {
		t.Fatalf("ListInvites = %v, %v; want 1", invites, err)
	}
	if invites[0].UsedAt != nil || invites[0].CreatedBy == nil || *invites[0].CreatedBy != alice.ID {
		t.Errorf("created invite = %+v", invites[0])
	}

	body := doRequest(t, server, http.MethodGet, "/invites", nil, cookie, "").Body.String()
	if !strings.Contains(body, fmt.Sprintf(`data-copy="%s/invite/%s"`, testBaseURL, invites[0].Token)) {
		t.Error("newest invite URL is not shown with a copy button")
	}
}
