package web

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/skorokithakis/soliloquy/internal/db"
)

const (
	// metricsQueryTimeout bounds the database queries the collector runs at
	// scrape time, so a slow or locked database cannot stall a scrape.
	metricsQueryTimeout = 2 * time.Second
	// metricsUnmatchedRoute is the route label for a request no mux pattern
	// matched. The raw path is never used: its cardinality is unbounded.
	metricsUnmatchedRoute = "unmatched"
	// metricsEventsPattern is the route whose duration is not observed, as it
	// stays open for as long as the page is.
	metricsEventsPattern = "GET /events"

	// Label values for the login and delivery counters. Every combination is
	// created in newMetrics so the series exist at zero before the first event.
	metricsResultSuccess   = "success"
	metricsResultFailure   = "failure"
	metricsResultThrottled = "throttled"
	metricsTargetNtfy      = "ntfy"
	metricsTargetDiscord   = "discord"
)

// metrics is the private Prometheus registry of one Server. Every server owns
// its registry, rather than using the global default one, so tests can build
// many servers without registering duplicate collectors.
type metrics struct {
	registry       *prometheus.Registry
	requests       *prometheus.CounterVec
	duration       *prometheus.HistogramVec
	sseConnections prometheus.Gauge
	logins         *prometheus.CounterVec
	deliveries     *prometheus.CounterVec
	handler        http.Handler
}

// newMetrics registers the HTTP, runtime and database metrics and returns the
// collector set. dbPath is the SQLite file whose size is reported; an empty
// value or ":memory:" skips the size gauge.
func newMetrics(database *db.DB, dbPath string) *metrics {
	m := &metrics{
		registry: prometheus.NewRegistry(),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "soliloquy_http_requests_total",
			Help: "HTTP requests served, by matched route, method and response code.",
		}, []string{"route", "method", "code"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "soliloquy_http_request_duration_seconds",
			Help: "HTTP request duration in seconds, by matched route.",
		}, []string{"route"}),
		sseConnections: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "soliloquy_sse_connections",
			Help: "Number of open server-sent event streams.",
		}),
		logins: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "soliloquy_logins_total",
			Help: "Login attempts, by result.",
		}, []string{"result"}),
		deliveries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "soliloquy_outbound_deliveries_total",
			Help: "Outbound notification deliveries, by target and result.",
		}, []string{"target", "result"}),
	}

	// Create every series up front so a scrape shows zeros, not a metric that
	// only appears after the first event.
	for _, result := range []string{metricsResultSuccess, metricsResultFailure, metricsResultThrottled} {
		m.logins.WithLabelValues(result).Add(0)
	}
	for _, target := range []string{metricsTargetNtfy, metricsTargetDiscord} {
		for _, result := range []string{metricsResultSuccess, metricsResultFailure} {
			m.deliveries.WithLabelValues(target, result).Add(0)
		}
	}

	m.registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.requests,
		m.duration,
		m.sseConnections,
		m.logins,
		m.deliveries,
		newDBCollector(database, dbPath),
	)
	m.handler = promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})

	return m
}

// handleMetrics serves the Prometheus text format, protected by the bearer
// token from the environment. An unset token turns the endpoint into a 404 so
// its existence is not advertised.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if s.cfg.MetricsToken == "" {
		http.NotFound(w, r)
		return
	}

	token, ok := bearerToken(r.Header.Get("Authorization"))
	if !ok || subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.MetricsToken)) != 1 {
		w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	s.metrics.handler.ServeHTTP(w, r)
}

// bearerToken extracts the token from an Authorization header.
func bearerToken(header string) (string, bool) {
	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	return strings.TrimSpace(token), true
}

// countDelivery records one outbound delivery attempt. A non-nil error is a
// failure; the caller still decides how to report it.
func (s *Server) countDelivery(target string, err error) {
	result := metricsResultSuccess
	if err != nil {
		result = metricsResultFailure
	}
	s.metrics.deliveries.WithLabelValues(target, result).Inc()
}

// httpMetrics records the route, method, code and duration of every request.
// It must wrap the mux directly, inside loadUser: ServeMux sets r.Pattern on
// the request it is handed, and loadUser may replace the request with one that
// carries the user context, so an outer middleware would never see the pattern.
func (s *Server) httpMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}

		next.ServeHTTP(recorder, r)

		route := r.Pattern
		if route == "" {
			route = metricsUnmatchedRoute
		}
		code := recorder.status
		if code == 0 {
			code = http.StatusOK
		}

		s.metrics.requests.WithLabelValues(route, methodLabel(r.Method), strconv.Itoa(code)).Inc()
		if r.Pattern != metricsEventsPattern {
			s.metrics.duration.WithLabelValues(route).Observe(time.Since(start).Seconds())
		}
	})
}

// methodLabel bounds the method label to the methods the app routes. A client
// can send an arbitrary method name, and each one would otherwise create a new
// series.
func methodLabel(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost:
		return method
	default:
		return "other"
	}
}

// statusRecorder remembers the response code a handler chose.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(body []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(body)
}

// Flush keeps the http.Flusher assertion in the /events handler working.
func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// dbCollector computes the database gauges at scrape time. Query errors are
// logged and only skip the affected metric.
type dbCollector struct {
	db   *db.DB
	path string

	users       *prometheus.Desc
	activeUsers *prometheus.Desc
	threads     *prometheus.Desc
	comments    *prometheus.Desc
	size        *prometheus.Desc
}

func newDBCollector(database *db.DB, path string) *dbCollector {
	return &dbCollector{
		db:   database,
		path: path,
		users: prometheus.NewDesc(
			"soliloquy_users",
			"Current user accounts by state.",
			[]string{"state"}, nil,
		),
		activeUsers: prometheus.NewDesc(
			"soliloquy_active_users",
			"Users who read, started or replied in the window.",
			[]string{"window"}, nil,
		),
		threads: prometheus.NewDesc(
			"soliloquy_threads",
			"Threads that are not deleted.",
			nil, nil,
		),
		comments: prometheus.NewDesc(
			"soliloquy_comments",
			"Comments that are not deleted.",
			nil, nil,
		),
		size: prometheus.NewDesc(
			"soliloquy_db_size_bytes",
			"Size of the SQLite database file, including its write-ahead log.",
			nil, nil,
		),
	}
}

func (c *dbCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.users
	ch <- c.activeUsers
	ch <- c.threads
	ch <- c.comments
	ch <- c.size
}

func (c *dbCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), metricsQueryTimeout)
	defer cancel()

	if counts, err := c.db.CountUsersByState(ctx); err != nil {
		slog.Error("collect user counts", "error", err)
	} else {
		ch <- prometheus.MustNewConstMetric(c.users, prometheus.GaugeValue, float64(counts.Active), "active")
		ch <- prometheus.MustNewConstMetric(c.users, prometheus.GaugeValue, float64(counts.Disabled), "disabled")
	}

	now := time.Now()
	for _, window := range []struct {
		label string
		span  time.Duration
	}{
		{"1d", 24 * time.Hour},
		{"7d", 7 * 24 * time.Hour},
		{"30d", 30 * 24 * time.Hour},
	} {
		count, err := c.db.CountActiveUsersSince(ctx, now.Add(-window.span))
		if err != nil {
			slog.Error("collect active users", "window", window.label, "error", err)
			continue
		}
		ch <- prometheus.MustNewConstMetric(c.activeUsers, prometheus.GaugeValue, float64(count), window.label)
	}

	if count, err := c.db.CountThreads(ctx); err != nil {
		slog.Error("collect thread count", "error", err)
	} else {
		ch <- prometheus.MustNewConstMetric(c.threads, prometheus.GaugeValue, float64(count))
	}

	if count, err := c.db.CountComments(ctx); err != nil {
		slog.Error("collect comment count", "error", err)
	} else {
		ch <- prometheus.MustNewConstMetric(c.comments, prometheus.GaugeValue, float64(count))
	}

	if size, ok := databaseSize(c.path); ok {
		ch <- prometheus.MustNewConstMetric(c.size, prometheus.GaugeValue, float64(size))
	}
}

// databaseSize returns the size of the database file plus its write-ahead log.
// A memory database or a file that does not exist yet reports nothing.
func databaseSize(path string) (int64, bool) {
	if path == "" || path == ":memory:" {
		return 0, false
	}

	info, err := os.Stat(path)
	if err != nil {
		return 0, false
	}

	size := info.Size()
	if wal, err := os.Stat(path + "-wal"); err == nil {
		size += wal.Size()
	}
	return size, true
}
