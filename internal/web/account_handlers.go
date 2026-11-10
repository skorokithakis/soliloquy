package web

import (
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/skorokithakis/soliloquy/internal/auth"
	"github.com/skorokithakis/soliloquy/internal/db"
)

type accountPageData struct {
	PageData
	Joined       string
	Role         string
	InvitedBy    string
	InvitedCount int64
	SessionEnds  string
	// NtfyURL is empty when push notifications are off. It is a secret and is
	// only ever shown back on the user's own account page.
	NtfyURL     string
	NtfyReplies bool
	NtfyThreads bool
	// NtfyTestResult is set after the user sends a test push.
	NtfyTestResult string
}

// ntfyFormValues is what the notifications form submitted. It survives a
// validation error so the user does not lose their input.
type ntfyFormValues struct {
	URL     string
	Replies bool
	Threads bool
}

// apiDocsPageData is the API documentation page. Its links carry the member's
// feed token, created on first view like the account page's.
type apiDocsPageData struct {
	PageData
	ThreadsURL    string
	ThreadPattern string
}

func (s *Server) handleAccountPage(w http.ResponseWriter, r *http.Request) {
	data, err := s.accountPageData(r)
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.render(w, "account", data)
}

func (s *Server) accountPageData(r *http.Request) (accountPageData, error) {
	user := currentUser(r)
	// The page always shows the feed URLs, so the token is created the first
	// time the account page is opened.
	if _, err := s.ensureFeedToken(r, user); err != nil {
		return accountPageData{}, err
	}

	data := accountPageData{
		PageData:     s.pageData(r, "Account"),
		Joined:       user.CreatedAt.Format("2006-01-02"),
		Role:         "user",
		InvitedBy:    "(root)",
		InvitedCount: 0,
		NtfyURL:      user.NtfyURL,
		NtfyReplies:  user.NtfyReplies,
		NtfyThreads:  user.NtfyThreads,
	}
	if user.IsAdmin {
		data.Role = "admin"
	}

	if user.InvitedBy != nil {
		inviter, err := s.db.UserByID(r.Context(), *user.InvitedBy)
		if err != nil {
			return accountPageData{}, err
		}
		if inviter != nil {
			data.InvitedBy = inviter.DisplayName()
		}
	}

	count, err := s.db.CountInvitedUsers(r.Context(), user.ID)
	if err != nil {
		return accountPageData{}, err
	}
	data.InvitedCount = count

	if session := currentSession(r); session != nil {
		data.SessionEnds = sessionEndsText(session.ExpiresAt, s.now())
	}

	return data, nil
}

// ensureFeedToken returns the user's feed token, creating one on first use. A
// concurrent request may store a token first; the stored value is returned so
// every page shows the same link.
func (s *Server) ensureFeedToken(r *http.Request, user *db.User) (string, error) {
	if user.FeedToken != "" {
		return user.FeedToken, nil
	}
	token, err := auth.GenerateToken()
	if err != nil {
		return "", err
	}
	stored, err := s.db.SetFeedTokenIfEmpty(r.Context(), user.ID, token)
	if err != nil {
		return "", err
	}
	user.FeedToken = stored
	return stored, nil
}

// handleAPIDocsPage shows the member their tokenized API URLs and documents the
// endpoints. The page needs a session like the rest of /account, so it never
// appears for visitors.
func (s *Server) handleAPIDocsPage(w http.ResponseWriter, r *http.Request) {
	token, err := s.ensureFeedToken(r, currentUser(r))
	if err != nil {
		s.serverError(w, err)
		return
	}

	data := apiDocsPageData{
		PageData:      s.pageData(r, "API"),
		ThreadsURL:    s.feedURL("/api/threads", token),
		ThreadPattern: s.feedURL("/api/threads/{id}", token),
	}
	s.render(w, "api_docs", data)
}

// renderAccountError re-renders the account page with a message, so password
// and deletion errors stay on the page that owns those forms.
func (s *Server) renderAccountError(w http.ResponseWriter, r *http.Request, status int, message string) {
	data, err := s.accountPageData(r)
	if err != nil {
		s.serverError(w, err)
		return
	}
	data.Error = message
	s.renderStatus(w, status, "account", data)
}

// renderAccountNtfy re-renders the account page after a push settings error,
// keeping the submitted values so the user can fix the URL instead of
// retyping the whole form.
func (s *Server) renderAccountNtfy(w http.ResponseWriter, r *http.Request, status int, values ntfyFormValues, message, testResult string) {
	data, err := s.accountPageData(r)
	if err != nil {
		s.serverError(w, err)
		return
	}
	data.NtfyURL = values.URL
	data.NtfyReplies = values.Replies
	data.NtfyThreads = values.Threads
	data.Error = message
	data.NtfyTestResult = testResult
	s.renderStatus(w, status, "account", data)
}

func (s *Server) handleSetTheme(w http.ResponseWriter, r *http.Request) {
	theme := r.FormValue("theme")
	switch theme {
	case "auto", "light", "dark":
	default:
		s.renderAccountError(w, r, http.StatusBadRequest, "Choose auto, light or dark.")
		return
	}

	if err := s.db.SetTheme(r.Context(), currentUser(r).ID, theme); err != nil {
		s.serverError(w, err)
		return
	}

	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

// handleResetFeedToken replaces the user's feed token, which makes every old
// feed URL stop working.
func (s *Server) handleResetFeedToken(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GenerateToken()
	if err != nil {
		s.serverError(w, err)
		return
	}
	if err := s.db.SetFeedToken(r.Context(), currentUser(r).ID, token); err != nil {
		s.serverError(w, err)
		return
	}

	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

func sessionEndsText(expiresAt, now time.Time) string {
	days := int(math.Ceil(expiresAt.Sub(now).Hours() / 24))
	if days <= 0 {
		return "expires today"
	}
	return fmt.Sprintf("ends in %d days", days)
}
