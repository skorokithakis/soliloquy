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
)

// currentUser returns the logged-in user stored by loadUser, or nil.
func currentUser(r *http.Request) *db.User {
	user, _ := r.Context().Value(userContextKey).(*db.User)
	return user
}

// currentSessionHash returns the hash of the session token making the request.
func currentSessionHash(r *http.Request) string {
	hash, _ := r.Context().Value(sessionContextKey).(string)
	return hash
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
// login and sign-up.
func (s *Server) loadUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Static assets must be available on the login page.
		if strings.HasPrefix(r.URL.Path, "/static/") {
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
				ctx = context.WithValue(ctx, sessionContextKey, session.TokenHash)
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

		if publicPath(r) {
			next.ServeHTTP(w, r)
			return
		}

		if currentUser(r) == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func publicPath(r *http.Request) bool {
	return r.URL.Path == "/login" ||
		strings.HasPrefix(r.URL.Path, "/invite/") ||
		strings.HasPrefix(r.URL.Path, "/reset/")
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
