package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/skorokithakis/soliloquy/internal/config"
	"github.com/skorokithakis/soliloquy/internal/db"
)

const testMetricsToken = "super-secret-token"

func newMetricsServer(t *testing.T, cfg config.Config) *Server {
	t.Helper()

	if cfg.BaseURL == "" {
		cfg.BaseURL = testBaseURL
	}
	server, err := NewServer(newTestDatabase(t), cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(server.waitBackground)
	return server
}

func scrapeMetrics(t *testing.T, server *Server) string {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	request.Header.Set("Authorization", "Bearer "+server.cfg.MetricsToken)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET /metrics = %d, want %d; body: %s", response.Code, http.StatusOK, response.Body)
	}
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/plain") {
		t.Errorf("Content-Type = %q, want Prometheus text", contentType)
	}
	return response.Body.String()
}

func TestMetricsDisabledWithoutToken(t *testing.T) {
	server := newMetricsServer(t, config.Config{})

	response := doRequest(t, server, http.MethodGet, "/metrics", nil, nil, "")
	if response.Code != http.StatusNotFound {
		t.Fatalf("GET /metrics without token configured = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestMetricsRequiresBearerToken(t *testing.T) {
	server := newMetricsServer(t, config.Config{MetricsToken: testMetricsToken})

	tests := []struct {
		name   string
		header string
	}{
		{"missing", ""},
		{"wrong token", "Bearer not-the-token"},
		{"wrong scheme", "Basic " + testMetricsToken},
		{"token only", testMetricsToken},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			if test.header != "" {
				request.Header.Set("Authorization", test.header)
			}
			response := httptest.NewRecorder()
			server.Handler().ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("GET /metrics with %s = %d, want %d", test.name, response.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestMetricsRouteLabelUsesPattern(t *testing.T) {
	server := newMetricsServer(t, config.Config{MetricsToken: testMetricsToken})
	user := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, user)

	// The ID is malformed, so the handler answers 404, but the mux still
	// matches GET /threads/{id} and the label must be the pattern, not the URL.
	response := doRequest(t, server, http.MethodGet, "/threads/abc", nil, cookie, "")
	if response.Code != http.StatusNotFound {
		t.Fatalf("GET /threads/abc = %d, want %d", response.Code, http.StatusNotFound)
	}
	// A path with no pattern is labelled "unmatched", never with the path.
	response = doRequest(t, server, http.MethodGet, "/no-such-page", nil, cookie, "")
	if response.Code != http.StatusNotFound {
		t.Fatalf("GET /no-such-page = %d, want %d", response.Code, http.StatusNotFound)
	}

	body := scrapeMetrics(t, server)
	line := `soliloquy_http_requests_total{code="404",method="GET",route="GET /threads/{id}"} 1`
	if !strings.Contains(body, line) {
		t.Errorf("metrics missing %q", line)
	}
	line = `soliloquy_http_requests_total{code="404",method="GET",route="unmatched"} 1`
	if !strings.Contains(body, line) {
		t.Errorf("metrics missing %q", line)
	}
	if strings.Contains(body, "/threads/abc") || strings.Contains(body, "/no-such-page") {
		t.Error("metrics contain the raw request path, want the matched pattern only")
	}
	if strings.Contains(body, `route="/metrics"`) {
		t.Error("metrics recorded the /metrics request itself")
	}
}

func TestMetricsMethodLabelIsBounded(t *testing.T) {
	server := newMetricsServer(t, config.Config{MetricsToken: testMetricsToken})
	cookie := signIn(t, server, createTestUser(t, server, "alice", false))

	// A client can send any method name. Only GET, HEAD and POST get their own
	// label; the rest collapse into "other".
	methods := []string{"CUSTOM123", "PROPFIND", "BREW"}
	for _, method := range methods {
		response := doRequest(t, server, method, "/", nil, cookie, "")
		if response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s / status = %d, want %d", method, response.Code, http.StatusMethodNotAllowed)
		}
	}

	body := scrapeMetrics(t, server)
	line := `soliloquy_http_requests_total{code="405",method="other",route="unmatched"} 3`
	if !strings.Contains(body, line) {
		t.Errorf("metrics missing %q", line)
	}
	for _, method := range methods {
		if strings.Contains(body, `method="`+method+`"`) {
			t.Errorf("metrics contain the raw method label %q", method)
		}
	}
}

func TestMetricsSSEConnectionLifecycle(t *testing.T) {
	server := newMetricsServer(t, config.Config{MetricsToken: testMetricsToken})
	user := createTestUser(t, server, "alice", false)
	cookie := signIn(t, server, user)
	handler := server.Handler()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	request := httptest.NewRequest(http.MethodGet, "/events", nil)
	request = request.WithContext(ctx)
	request.AddCookie(cookie)

	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(recorder, request)
		close(done)
	}()

	waitForMetric := func(want string) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for {
			if strings.Contains(scrapeMetrics(t, server), want) {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %q", want)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}

	waitForMetric("soliloquy_sse_connections 1")

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("event stream did not stop after its context was cancelled")
	}
	waitForMetric("soliloquy_sse_connections 0")

	body := scrapeMetrics(t, server)
	if !strings.Contains(body, `soliloquy_http_requests_total{code="200",method="GET",route="GET /events"} 1`) {
		t.Error("metrics missing the /events request count")
	}
	// The stream lives as long as the page, so its duration is never recorded.
	for _, metric := range []string{
		`soliloquy_http_request_duration_seconds_bucket{route="GET /events"`,
		`soliloquy_http_request_duration_seconds_sum{route="GET /events"`,
		`soliloquy_http_request_duration_seconds_count{route="GET /events"`,
	} {
		if strings.Contains(body, metric) {
			t.Errorf("metrics contain %q, want no duration for GET /events", metric)
		}
	}
}

func TestMetricsCountsLoginResults(t *testing.T) {
	server := newMetricsServer(t, config.Config{MetricsToken: testMetricsToken})
	createTestUser(t, server, "alice", false)
	createTestUser(t, server, "bob", false)

	response := postForm(t, server, "/login", url.Values{
		"username": {"alice"},
		"password": {"wrong"},
	}, nil)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("failed login status = %d, want %d", response.Code, http.StatusUnauthorized)
	}

	// Alice's next attempt is rejected by the throttle before the password is
	// checked, so it counts as throttled rather than failed.
	response = postForm(t, server, "/login", url.Values{
		"username": {"alice"},
		"password": {testPassword},
	}, nil)
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("throttled login status = %d, want %d", response.Code, http.StatusTooManyRequests)
	}

	response = postForm(t, server, "/login", url.Values{
		"username": {"bob"},
		"password": {testPassword},
	}, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("successful login status = %d, want %d", response.Code, http.StatusSeeOther)
	}

	body := scrapeMetrics(t, server)
	for _, want := range []string{
		`soliloquy_logins_total{result="failure"} 1`,
		`soliloquy_logins_total{result="throttled"} 1`,
		`soliloquy_logins_total{result="success"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics missing %q", want)
		}
	}
}

func TestMetricsCounterSeriesStartAtZero(t *testing.T) {
	server := newMetricsServer(t, config.Config{MetricsToken: testMetricsToken})

	body := scrapeMetrics(t, server)
	for _, want := range []string{
		`soliloquy_logins_total{result="failure"} 0`,
		`soliloquy_logins_total{result="success"} 0`,
		`soliloquy_logins_total{result="throttled"} 0`,
		`soliloquy_outbound_deliveries_total{result="failure",target="discord"} 0`,
		`soliloquy_outbound_deliveries_total{result="success",target="discord"} 0`,
		`soliloquy_outbound_deliveries_total{result="failure",target="ntfy"} 0`,
		`soliloquy_outbound_deliveries_total{result="success",target="ntfy"} 0`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics missing pre-initialised series %q", want)
		}
	}
}

func TestMetricsCountsDeliveryResults(t *testing.T) {
	server := newMetricsServer(t, config.Config{MetricsToken: testMetricsToken})
	alice := createTestUser(t, server, "alice", false)
	bob := createTestUser(t, server, "bob", false)

	pushes := newFakePushSender()
	pushes.errs["https://ntfy.sh/bob"] = errors.New("ntfy is down")
	server.pushes = pushes
	saveNtfySettings(t, server, bob, "https://ntfy.sh/bob", false, true)

	webhooks := []string{
		"https://discord.com/api/webhooks/1/aaa",
		"https://discord.com/api/webhooks/2/bbb",
	}
	discordSender := newFakeWebhookSender()
	discordSender.errs[webhooks[1]] = errors.New("discord is down")
	server.webhooks = discordSender
	saveDiscordWebhooks(t, server, webhooks)

	response := postForm(t, server, "/threads", url.Values{
		"title": {"Hello"},
		"body":  {"body"},
	}, signIn(t, server, alice))
	if response.Code != http.StatusSeeOther {
		t.Fatalf("create thread status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	server.waitBackground()

	body := scrapeMetrics(t, server)
	for _, want := range []string{
		`soliloquy_outbound_deliveries_total{result="failure",target="ntfy"} 1`,
		`soliloquy_outbound_deliveries_total{result="success",target="ntfy"} 0`,
		`soliloquy_outbound_deliveries_total{result="failure",target="discord"} 1`,
		`soliloquy_outbound_deliveries_total{result="success",target="discord"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics missing %q", want)
		}
	}
}

func TestMetricsDatabaseGauges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.db")
	database, err := db.Open(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := database.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	server, err := NewServer(database, config.Config{
		BaseURL:      testBaseURL,
		DBPath:       path,
		MetricsToken: testMetricsToken,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(server.waitBackground)

	user := createTestUser(t, server, "alice", false)
	disabled := createTestUser(t, server, "bob", false)
	if _, err := database.ExecContext(context.Background(),
		`UPDATE users SET disabled_at = ? WHERE id = ?`, server.now().Unix(), disabled.ID); err != nil {
		t.Fatalf("disable user: %v", err)
	}
	if _, err := database.CreateThread(context.Background(), user.ID, "Hello", "", "body", "<p>body</p>", server.now()); err != nil {
		t.Fatalf("create thread: %v", err)
	}

	body := scrapeMetrics(t, server)
	wants := []string{
		`soliloquy_users{state="active"} 1`,
		`soliloquy_users{state="disabled"} 1`,
		`soliloquy_active_users{window="1d"} 1`,
		`soliloquy_active_users{window="7d"} 1`,
		`soliloquy_active_users{window="30d"} 1`,
		`soliloquy_threads 1`,
		`soliloquy_comments 0`,
		"soliloquy_db_size_bytes ",
	}
	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Errorf("metrics missing %q", want)
		}
	}
}

func TestMetricsCountsActivityInWindow(t *testing.T) {
	server := newMetricsServer(t, config.Config{MetricsToken: testMetricsToken})
	now := server.now()

	recent := createTestUser(t, server, "recent", false)
	stale := createTestUser(t, server, "stale", false)
	if _, err := server.db.CreateThread(context.Background(), recent.ID, "New", "", "body", "<p>body</p>", now); err != nil {
		t.Fatalf("create thread: %v", err)
	}
	if _, err := server.db.CreateThread(context.Background(), stale.ID, "Old", "", "body", "<p>body</p>", now.Add(-10*24*time.Hour)); err != nil {
		t.Fatalf("create thread: %v", err)
	}

	body := scrapeMetrics(t, server)
	if !strings.Contains(body, `soliloquy_active_users{window="1d"} 1`) {
		t.Error("1d active users should count only the recent author")
	}
	if !strings.Contains(body, `soliloquy_active_users{window="30d"} 2`) {
		t.Error("30d active users should count both authors")
	}
}
