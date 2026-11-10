// Package web contains the HTTP server, handlers, templates and static
// assets.
package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/skorokithakis/soliloquy/internal/auth"
	"github.com/skorokithakis/soliloquy/internal/config"
	"github.com/skorokithakis/soliloquy/internal/db"
	"github.com/skorokithakis/soliloquy/internal/discord"
	"github.com/skorokithakis/soliloquy/internal/hub"
	"github.com/skorokithakis/soliloquy/internal/linktitle"
)

//go:embed templates static
var assets embed.FS

// Server serves the HTML pages and static assets.
type Server struct {
	db              *db.DB
	cfg             config.Config
	templates       map[string]*template.Template
	commentTemplate *template.Template
	static          http.Handler
	favicon         []byte
	baseOrigin      string
	baseHost        string
	throttle        *auth.Throttle
	hub             *hub.Hub
	now             func() time.Time
	// webhooks posts new threads to Discord. It is an interface so tests can
	// replace it and avoid the network.
	webhooks webhookSender
	// titleFetcher fetches page titles for POST /title. Tests set
	// AllowPrivateAddresses so it can reach httptest servers on 127.0.0.1.
	titleFetcher *linktitle.Fetcher
}

// PageData is embedded in every page's template data. The layout uses
// CurrentUser for the navigation, and handlers can set Error to show a
// message above the form.
type PageData struct {
	Title               string
	CurrentUser         *db.User
	Error               string
	UnreadNotifications int64
	// Theme is the palette class on the root element: auto, light or dark.
	// Empty means auto, so handlers that build PageData by hand still follow
	// the operating system preference.
	Theme string
	// Nav is the main navigation item to mark as current: threads, invites
	// or admin.
	Nav string
	// SearchQuery keeps the header search box filled in on the search page.
	SearchQuery string
	// EventThreadID tells the layout which thread's live comment stream the
	// page should subscribe to. Empty means no thread stream.
	EventThreadID string
	// ForumName is the site name shown in the title and logo.
	ForumName string
	// BaseURL is the configured site base URL, used to build absolute URLs
	// for link preview metadata.
	BaseURL string
	// ThreadsFeedURL and CommentsFeedURL are the absolute RSS feed URLs for
	// the layout's alternate links. Empty means the page advertises no feed.
	ThreadsFeedURL  string
	CommentsFeedURL string
	// Visitor marks a public, read-only page rendered for someone without a
	// session. Those pages use the full header but hide all controls that
	// need an account.
	Visitor bool
}

// NewServer parses the embedded templates and prepares the handlers.
func NewServer(database *db.DB, cfg config.Config) (*Server, error) {
	if database == nil {
		return nil, fmt.Errorf("database is required")
	}

	base, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base URL: %w", err)
	}
	if base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("base URL %q has no scheme or host", cfg.BaseURL)
	}

	staticFS, err := fs.Sub(assets, "static")
	if err != nil {
		return nil, fmt.Errorf("open static assets: %w", err)
	}

	staticAssets, err := loadStaticAssets(staticFS)
	if err != nil {
		return nil, err
	}

	templates, err := parseTemplates(staticAssets.URL)
	if err != nil {
		return nil, err
	}

	commentTemplate, err := template.New("comment").ParseFS(assets, "templates/comment.html")
	if err != nil {
		return nil, fmt.Errorf("parse comment template: %w", err)
	}

	favicon, err := fs.ReadFile(staticFS, "favicon.ico")
	if err != nil {
		return nil, fmt.Errorf("read favicon: %w", err)
	}

	fileHandler := http.StripPrefix("/static/", http.FileServer(http.FS(staticFS)))

	return &Server{
		db:              database,
		cfg:             cfg,
		templates:       templates,
		commentTemplate: commentTemplate,
		static:          cacheHashedStatic(staticAssets, fileHandler),
		favicon:         favicon,
		baseOrigin:      base.Scheme + "://" + base.Host,
		baseHost:        base.Host,
		throttle:        auth.NewThrottle(),
		hub:             hub.New(),
		now:             time.Now,
		webhooks:        discord.NewClient(),
		titleFetcher:    linktitle.NewFetcher(),
	}, nil
}

// Handler returns the HTTP handler for the application.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /threads/new", s.handleNewThreadPage)
	mux.HandleFunc("POST /threads", s.handleCreateThread)
	mux.HandleFunc("POST /preview", s.handlePreview)
	mux.HandleFunc("POST /title", s.handleTitle)
	mux.HandleFunc("GET /threads/{id}", s.handleThreadPage)
	mux.HandleFunc("GET /threads/{id}/edit", s.handleEditThreadPage)
	mux.HandleFunc("POST /threads/{id}/edit", s.handleUpdateThread)
	mux.HandleFunc("POST /threads/{id}/delete", s.handleDeleteThread)
	mux.HandleFunc("POST /threads/{id}/vote", s.handleVoteThread)
	mux.HandleFunc("POST /threads/{id}/comments", s.handleCreateComment)
	mux.HandleFunc("GET /comments/{id}/reply", s.handleReplyPage)
	mux.HandleFunc("POST /comments/{id}/reply", s.handleCreateReply)
	mux.HandleFunc("GET /comments/{id}/edit", s.handleEditCommentPage)
	mux.HandleFunc("POST /comments/{id}/edit", s.handleUpdateComment)
	mux.HandleFunc("POST /comments/{id}/delete", s.handleDeleteComment)
	mux.HandleFunc("POST /comments/{id}/vote", s.handleVoteComment)
	mux.HandleFunc("GET /login", s.handleLoginPage)
	mux.HandleFunc("POST /login", s.handleLogin)
	mux.HandleFunc("POST /logout", s.handleLogout)
	mux.HandleFunc("GET /invite/{token}", s.handleSignupPage)
	mux.HandleFunc("GET /invite/{token}/check", s.handleInviteUsernameCheck)
	mux.HandleFunc("POST /invite/{token}", s.handleSignup)
	mux.HandleFunc("GET /invites", s.handleInvitesPage)
	mux.HandleFunc("POST /invites", s.handleCreateInvite)
	mux.HandleFunc("GET /admin/users", s.handleAdminUsersPage)
	mux.HandleFunc("GET /admin/settings", s.handleAdminSettingsPage)
	mux.HandleFunc("POST /admin/settings", s.handleAdminSettingsSave)
	mux.HandleFunc("POST /admin/settings/discord-test", s.handleAdminDiscordTest)
	mux.HandleFunc("GET /tree", s.handleInviteTree)
	mux.HandleFunc("POST /admin/users/{id}/disable", s.handleAdminDisableUser)
	mux.HandleFunc("POST /admin/users/{id}/reset", s.handleAdminResetPassword)
	mux.HandleFunc("POST /admin/users/{id}/grant-admin", s.handleAdminGrantAdmin)
	mux.HandleFunc("POST /admin/users/{id}/revoke-admin", s.handleAdminRevokeAdmin)
	mux.HandleFunc("POST /account/delete", s.handleDeleteAccount)
	mux.HandleFunc("GET /account", s.handleAccountPage)
	mux.HandleFunc("GET /account/api", s.handleAPIDocsPage)
	mux.HandleFunc("POST /account/theme", s.handleSetTheme)
	mux.HandleFunc("GET /reset/{token}", s.handleResetPage)
	mux.HandleFunc("POST /reset/{token}", s.handlePasswordReset)
	mux.HandleFunc("GET /password", s.handlePasswordPage)
	mux.HandleFunc("POST /password", s.handleChangePassword)
	mux.HandleFunc("GET /notifications", s.handleNotificationsPage)
	mux.HandleFunc("GET /search", s.handleSearch)
	mux.HandleFunc("GET /feeds/threads", s.handleThreadsFeed)
	mux.HandleFunc("GET /feeds/comments", s.handleCommentsFeed)
	mux.HandleFunc("GET /api/threads", s.handleAPIThreads)
	mux.HandleFunc("GET /api/threads/{id}", s.handleAPIThread)
	mux.HandleFunc("POST /account/feeds/reset", s.handleResetFeedToken)
	mux.HandleFunc("GET /events", s.handleEvents)
	mux.HandleFunc("GET /favicon.ico", s.handleFavicon)
	mux.Handle("GET /static/", s.static)

	return s.originCheck(s.loadUser(mux))
}

// handleFavicon serves the icon at the site root because browsers and bots
// request /favicon.ico by default.
func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/x-icon")
	http.ServeContent(w, r, "favicon.ico", time.Time{}, bytes.NewReader(s.favicon))
}

func (s *Server) pageData(r *http.Request, title string) PageData {
	data := PageData{
		Title:       title,
		CurrentUser: currentUser(r),
		Theme:       "auto",
		Nav:         navSection(r.URL.Path),
		ForumName:   db.DefaultForumName,
		BaseURL:     s.cfg.BaseURL,
	}
	settings, err := s.db.Settings(r.Context())
	if err != nil {
		slog.Error("load settings", "error", err)
	} else {
		data.ForumName = settings.ForumName
		data.ThreadsFeedURL, data.CommentsFeedURL = s.feedURLs(data.CurrentUser, settings.Public)
	}
	if data.CurrentUser != nil {
		if data.CurrentUser.Theme != "" {
			data.Theme = data.CurrentUser.Theme
		}
		count, err := s.db.CountUnreadNotifications(r.Context(), data.CurrentUser.ID)
		if err != nil {
			slog.Error("count unread notifications", "error", err)
		} else {
			data.UnreadNotifications = count
		}
	}
	return data
}

// pageDataWithError builds PageData for a page that re-renders after a form
// error, so the site settings and navigation are still filled in.
func (s *Server) pageDataWithError(r *http.Request, title, message string) PageData {
	data := s.pageData(r, title)
	data.Error = message
	return data
}

// navSection maps a request path to the main navigation item it belongs to.
func navSection(path string) string {
	switch {
	case path == "/" || strings.HasPrefix(path, "/threads/") || strings.HasPrefix(path, "/comments/"):
		return "threads"
	case path == "/invites" || path == "/tree":
		return "invites"
	case strings.HasPrefix(path, "/admin/"):
		return "admin"
	default:
		return ""
	}
}

func (s *Server) render(w http.ResponseWriter, page string, data any) {
	s.renderStatus(w, http.StatusOK, page, data)
}

func (s *Server) renderStatus(w http.ResponseWriter, status int, page string, data any) {
	tmpl, ok := s.templates[page]
	if !ok {
		slog.Error("template not found", "page", page)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	var body bytes.Buffer
	if err := tmpl.ExecuteTemplate(&body, "layout", data); err != nil {
		slog.Error("render template", "page", page, "error", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if _, err := body.WriteTo(w); err != nil {
		slog.Error("write response", "page", page, "error", err)
	}
}

func (s *Server) serverError(w http.ResponseWriter, err error) {
	slog.Error("request failed", "error", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

// parseTemplates builds one template set per page. Each page defines a
// "content" block and the shared layout defines the surrounding document, so
// pages do not overwrite each other's content.
func parseTemplates(static func(string) (string, error)) (map[string]*template.Template, error) {
	pageFiles, err := fs.Glob(assets, "templates/pages/*.html")
	if err != nil {
		return nil, fmt.Errorf("find page templates: %w", err)
	}
	if len(pageFiles) == 0 {
		return nil, fmt.Errorf("no page templates found")
	}

	templates := make(map[string]*template.Template, len(pageFiles))
	for _, pageFile := range pageFiles {
		name := strings.TrimSuffix(path.Base(pageFile), ".html")
		tmpl, err := template.New(name).Funcs(template.FuncMap{"static": static}).ParseFS(assets, "templates/layout.html", "templates/comment.html", pageFile)
		if err != nil {
			return nil, fmt.Errorf("parse page template %q: %w", pageFile, err)
		}
		templates[name] = tmpl
	}

	return templates, nil
}
