package web

import (
	"net/http"
	"time"

	"github.com/skorokithakis/soliloquy/internal/auth"
)

type inviteView struct {
	URL       string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type invitesPageData struct {
	PageData
	Invites []inviteView
}

func (s *Server) handleInvitesPage(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	invites, err := s.db.ListActiveInvites(r.Context(), user.ID, s.now())
	if err != nil {
		s.serverError(w, err)
		return
	}

	views := make([]inviteView, 0, len(invites))
	for _, invite := range invites {
		views = append(views, inviteView{
			URL:       s.cfg.BaseURL + "/invite/" + invite.Token,
			CreatedAt: invite.CreatedAt,
			ExpiresAt: invite.ExpiresAt,
		})
	}

	s.render(w, "invites", invitesPageData{
		PageData: s.pageData(r, "My invites"),
		Invites:  views,
	})
}

func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	token, err := auth.GenerateToken()
	if err != nil {
		s.serverError(w, err)
		return
	}
	if err := s.db.CreateInvite(r.Context(), token, &user.ID, false, s.now()); err != nil {
		s.serverError(w, err)
		return
	}

	http.Redirect(w, r, "/invites", http.StatusSeeOther)
}
