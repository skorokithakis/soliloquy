package web

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skorokithakis/soliloquy/internal/auth"
	"github.com/skorokithakis/soliloquy/internal/config"
	"github.com/skorokithakis/soliloquy/internal/db"
)

const (
	testBaseURL  = "http://forum.test"
	testPassword = "password123"
)

var testPasswordHash = func() string {
	hash, err := auth.HashPassword(testPassword)
	if err != nil {
		panic(err)
	}
	return hash
}()

func newTestServer(t *testing.T) *Server {
	t.Helper()

	server, err := NewServer(newTestDatabase(t), config.Config{BaseURL: testBaseURL})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	// The database cleanup was registered first, so it runs after this one:
	// let fire-and-forget webhook and push work finish before closing the DB.
	t.Cleanup(server.waitBackground)
	return server
}

func newTestDatabase(t *testing.T) *db.DB {
	t.Helper()

	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return database
}

func createTestUser(t *testing.T, server *Server, username string, isAdmin bool) *db.User {
	t.Helper()

	user, err := server.db.CreateUser(context.Background(), username, testPasswordHash, isAdmin, nil, server.now())
	if err != nil {
		t.Fatalf("create user %q: %v", username, err)
	}
	return user
}

func signIn(t *testing.T, server *Server, user *db.User) *http.Cookie {
	t.Helper()

	token, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("generate session token: %v", err)
	}

	now := server.now()
	if err := server.db.CreateSession(context.Background(), user.ID, auth.HashToken(token), now, now.Add(auth.SessionLifetime)); err != nil {
		t.Fatalf("create session: %v", err)
	}

	return &http.Cookie{Name: sessionCookieName, Value: token}
}

func get(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	return doRequest(t, newTestServer(t), http.MethodGet, path, nil, nil, "")
}

func doRequest(t *testing.T, server *Server, method, path string, form url.Values, cookie *http.Cookie, origin string) *httptest.ResponseRecorder {
	t.Helper()

	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}

	request := httptest.NewRequest(method, path, body)
	if form != nil {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}

	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}

func postForm(t *testing.T, server *Server, path string, form url.Values, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	return doRequest(t, server, http.MethodPost, path, form, cookie, testBaseURL)
}

func sessionCookie(t *testing.T, response *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()

	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == sessionCookieName && cookie.Value != "" {
			return &http.Cookie{Name: cookie.Name, Value: cookie.Value}
		}
	}
	t.Fatal("response did not set a session cookie")
	return nil
}
