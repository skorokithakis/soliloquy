package web

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/skorokithakis/soliloquy/internal/config"
)

func TestPostRejectsMissingOrWrongOrigin(t *testing.T) {
	server := newTestServer(t)
	form := url.Values{"username": {"alice"}, "password": {"wrong"}}

	tests := []struct {
		name   string
		origin string
		want   int
	}{
		{name: "missing", origin: "", want: http.StatusForbidden},
		{name: "wrong", origin: "http://evil.example", want: http.StatusForbidden},
		{name: "matching", origin: testBaseURL, want: http.StatusUnauthorized},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := doRequest(t, server, http.MethodPost, "/login", form, nil, test.origin)
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d", response.Code, test.want)
			}
		})
	}
}

func TestGetDoesNotRequireOrigin(t *testing.T) {
	response := get(t, "/login")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
}

func TestAllowAnyOriginSkipsOriginCheck(t *testing.T) {
	server, err := NewServer(newTestDatabase(t), config.Config{BaseURL: testBaseURL, AllowAnyOrigin: true})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	// Each try uses its own username so the login throttle does not interfere.
	for i, origin := range []string{"", "http://evil.example"} {
		form := url.Values{"username": {[]string{"alice", "bobby"}[i]}, "password": {"wrong"}}
		response := doRequest(t, server, http.MethodPost, "/login", form, nil, origin)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("origin %q: status = %d, want %d", origin, response.Code, http.StatusUnauthorized)
		}
	}
}
