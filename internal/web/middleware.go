package web

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/skorokithakis/soliloquy/internal/auth"
	"github.com/skorokithakis/soliloquy/internal/db"
)

const (
	sessionCookieName = "session"
	// sessionExtensionInterval is how stale a session's expiry may get before
	// the sliding expiry is refreshed. Refreshing at most once a day keeps
	// writes down while still extending active sessions.
	sessionExtensionInterval = 24 * time.Hour
)

type contextKey int

const (
	userContextKey contextKey = iota
	sessionContextKey
	// feedTokenUserContextKey holds the active user a feed's ?token= belongs
	// to. It is separate from userContextKey because the token does not log
	// the request in and must not create a session or a cookie.
	feedTokenUserContextKey
)

// currentUser returns the logged-in user stored by loadUser, or nil.
func currentUser(r *http.Request) *db.User {
	user, _ := r.Context().Value(userContextKey).(*db.User)
	return user
}

// feedUser returns the user a feed request is for: the user whose feed token
// was given, or the session user when the token is absent or invalid. It
// returns nil when the request identifies nobody, which personal feeds treat
// as a 404 even on a public forum.
func feedUser(r *http.Request) *db.User {
	if user, _ := r.Context().Value(feedTokenUserContextKey).(*db.User); user != nil {
		return user
	}
	return currentUser(r)
}

// currentSession returns the session making the request, or nil.
func currentSession(r *http.Request) *db.Session {
	session, _ := r.Context().Value(sessionContextKey).(*db.Session)
	return session
}

// currentSessionHash returns the hash of the session token making the request.
func currentSessionHash(r *http.Request) string {
	session := currentSession(r)
	if session == nil {
		return ""
	}
	return session.TokenHash
}

// originCheck rejects POST requests whose Origin header is missing or does not
// match the configured base URL. This, together with SameSite cookies, is the
// CSRF defence. AllowAnyOrigin turns it off, for QA servers only.
func (s *Server) originCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && !s.cfg.AllowAnyOrigin && r.Header.Get("Origin") != s.baseOrigin {
			http.Error(w, "invalid origin", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// loadUser loads the session's user into the request context. It refreshes
// sliding session expiry and requires a logged-in user for every route except
// login, sign-up and, when the forum is public, the read-only pages.
func (s *Server) loadUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Static assets and the favicon must be available on the login page
		// and to logged-out visitors.
		if strings.HasPrefix(r.URL.Path, "/static/") || r.URL.Path == "/favicon.ico" {
			next.ServeHTTP(w, r)
			return
		}

		now := s.now()
		if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
			tokenHash := auth.HashToken(cookie.Value)
			session, user, err := s.db.SessionUser(r.Context(), tokenHash, now)
			if err != nil {
				s.serverError(w, err)
				return
			}

			if user == nil {
				s.clearSessionCookie(w)
			} else {
				ctx := context.WithValue(r.Context(), userContextKey, user)
				ctx = context.WithValue(ctx, sessionContextKey, session)
				r = r.WithContext(ctx)

				if session.ExpiresAt.Sub(now) < auth.SessionLifetime-sessionExtensionInterval {
					expires := now.Add(auth.SessionLifetime)
					if err := s.db.ExtendSession(r.Context(), session.TokenHash, expires); err != nil {
						slog.Error("extend session", "error", err)
					} else {
						s.setSessionCookie(w, cookie.Value, expires)
					}
				}
			}
		}

		if feedPath(r) {
			feedRequest, allowed := s.allowFeed(w, r)
			if !allowed {
				return
			}
			next.ServeHTTP(w, feedRequest)
			return
		}

		if publicPath(r) {
			next.ServeHTTP(w, r)
			return
		}

		if currentUser(r) == nil {
			if readOnlyPath(r) {
				settings, err := s.db.Settings(r.Context())
				if err != nil {
					s.serverError(w, err)
					return
				}
				if settings.Public {
					next.ServeHTTP(w, r)
					return
				}
			}

			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// allowFeed applies the access rule for feed and JSON API requests. A session,
// a token belonging to an active user, or a public forum grants access.
// Anything else is a plain 404: feed readers and API clients cannot follow a
// login redirect, so the request must fail instead. The token is read-only and
// never creates a session or sets a cookie. The returned request carries the
// token's user in its context so personal feeds can tell whose feed is being
// read.
func (s *Server) allowFeed(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
	if token := r.URL.Query().Get("token"); token != "" {
		user, err := s.db.UserByFeedToken(r.Context(), token)
		if err != nil {
			s.feedAccessError(w, r, err)
			return r, false
		}
		if user.Active() {
			ctx := context.WithValue(r.Context(), feedTokenUserContextKey, user)
			return r.WithContext(ctx), true
		}
	}

	if currentUser(r) != nil {
		return r, true
	}

	settings, err := s.db.Settings(r.Context())
	if err != nil {
		s.feedAccessError(w, r, err)
		return r, false
	}
	if settings.Public {
		return r, true
	}

	if apiPath(r) {
		writeJSONError(w, http.StatusNotFound, "not found")
		return r, false
	}
	http.NotFound(w, r)
	return r, false
}

// feedAccessError reports an unexpected failure of the feed/API access check.
// The JSON API gets a JSON error; the RSS feeds keep their plain-text one.
func (s *Server) feedAccessError(w http.ResponseWriter, r *http.Request, err error) {
	if apiPath(r) {
		s.apiServerError(w, err)
		return
	}
	s.serverError(w, err)
}

func publicPath(r *http.Request) bool {
	return r.URL.Path == "/login" ||
		strings.HasPrefix(r.URL.Path, "/invite/") ||
		strings.HasPrefix(r.URL.Path, "/reset/")
}

// readOnlyPath reports whether a request is one visitors may make when the
// forum is public: the front page, a thread page and search. Everything else,
// including every write, still needs a logged-in user.
func readOnlyPath(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if r.URL.Path == "/" || r.URL.Path == "/search" {
		return true
	}

	// Any single /threads/{id} path is read-only, including IDs that no longer
	// resolve. The handler returns 404 for malformed or old numeric IDs, so a
	// visitor sees the same 404 as a logged-in user instead of a login
	// redirect. /threads/new and subpaths such as /threads/{id}/edit still need
	// an account.
	rest, found := strings.CutPrefix(r.URL.Path, "/threads/")
	return found && rest != "" && rest != "new" && !strings.Contains(rest, "/")
}

// validPublicID reports whether id has the shape of a thread or comment public
// ID. Handlers use it to reject malformed IDs without a database lookup, and
// the live-update query uses it to ignore a malformed thread ID.
func validPublicID(id string) bool {
	if len(id) != auth.ShortIDLength {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		alphanumeric := (c >= 'A' && c <= 'Z') ||
			(c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9')
		if !alphanumeric {
			return false
		}
	}
	return true
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(auth.SessionLifetime / time.Second),
		HttpOnly: true,
		Secure:   s.secureCookies(),
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(1, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secureCookies(),
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) secureCookies() bool {
	return strings.HasPrefix(s.cfg.BaseURL, "https://")
}

// startSession creates a session for userID and sends the session cookie.
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, userID int64, now time.Time) error {
	token, err := auth.GenerateToken()
	if err != nil {
		return err
	}

	expires := now.Add(auth.SessionLifetime)
	if err := s.db.CreateSession(r.Context(), userID, auth.HashToken(token), now, expires); err != nil {
		return err
	}

	s.setSessionCookie(w, token, expires)
	return nil
}

// startSessionIfPasswordUnchanged creates a session only if the user's password
// hash still matches expectedHash. It reports whether a session was created.
func (s *Server) startSessionIfPasswordUnchanged(w http.ResponseWriter, r *http.Request, userID int64, expectedHash string, now time.Time) (bool, error) {
	token, err := auth.GenerateToken()
	if err != nil {
		return false, err
	}

	expires := now.Add(auth.SessionLifetime)
	created, err := s.db.CreateSessionIfPassword(r.Context(), userID, expectedHash, auth.HashToken(token), now, expires)
	if err != nil || !created {
		return created, err
	}

	s.setSessionCookie(w, token, expires)
	return true, nil
}

// requireAdmin is the server-side permission check for admin-only handlers.
// Later admin handlers should call it before acting.
func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	user := currentUser(r)
	if user == nil || !user.IsAdmin {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return false
	}
	return true
}
