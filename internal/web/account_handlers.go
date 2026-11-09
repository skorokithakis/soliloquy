package web

import (
	"fmt"
	"math"
	"net/http"
	"time"
)

type accountPageData struct {
	PageData
	Joined       string
	Role         string
	InvitedBy    string
	InvitedCount int64
	SessionEnds  string
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
	data := accountPageData{
		PageData:     s.pageData(r, "Account"),
		Joined:       user.CreatedAt.Format("2006-01-02"),
		Role:         "user",
		InvitedBy:    "(root)",
		InvitedCount: 0,
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

func sessionEndsText(expiresAt, now time.Time) string {
	days := int(math.Ceil(expiresAt.Sub(now).Hours() / 24))
	if days <= 0 {
		return "expires today"
	}
	return fmt.Sprintf("ends in %d days", days)
}
