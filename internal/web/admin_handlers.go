package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/skorokithakis/soliloquy/internal/auth"
	"github.com/skorokithakis/soliloquy/internal/db"
)

type inviteNodeView struct {
	ID       int64
	Name     string
	IsAdmin  bool
	Disabled bool
	CanAdmin bool
	Joined   string
	Children []*inviteNodeView
}

type inviteTreePageData struct {
	PageData
	Nodes    []*inviteNodeView
	ResetURL string
	ResetFor string
}

func (s *Server) handleInviteTree(w http.ResponseWriter, r *http.Request) {
	s.renderInviteTree(w, r, http.StatusOK, "", "")
}

// renderInviteTree renders the tree and, after an admin creates a reset link,
// the one-time URL for it.
func (s *Server) renderInviteTree(w http.ResponseWriter, r *http.Request, status int, resetURL, resetFor string) {
	users, err := s.db.ListUsers(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}

	s.renderStatus(w, status, "tree", inviteTreePageData{
		PageData: s.pageData(r, "Invite tree"),
		Nodes:    inviteNodeViews(db.BuildInviteTree(users), currentUser(r)),
		ResetURL: resetURL,
		ResetFor: resetFor,
	})
}

func inviteNodeViews(nodes []*db.InviteNode, viewer *db.User) []*inviteNodeView {
	views := make([]*inviteNodeView, len(nodes))
	for i, node := range nodes {
		user := node.User
		views[i] = &inviteNodeView{
			ID:       user.ID,
			Name:     user.DisplayName(),
			IsAdmin:  user.IsAdmin,
			Disabled: user.DisabledAt != nil,
			CanAdmin: viewer != nil && viewer.IsAdmin && user.ID != viewer.ID && user.DeletedAt == nil,
			Joined:   user.CreatedAt.Format("2006-01-02"),
			Children: inviteNodeViews(node.Children, viewer),
		}
	}
	return views
}

func (s *Server) handleAdminDisableUser(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	id, ok := pathUserID(w, r)
	if !ok {
		return
	}
	if id == currentUser(r).ID {
		http.Error(w, "You cannot disable yourself.", http.StatusForbidden)
		return
	}

	if _, err := s.db.DisableUser(r.Context(), id, s.now()); err != nil {
		if errors.Is(err, db.ErrLastAdmin) {
			http.Error(w, "The last active admin cannot be disabled.", http.StatusForbidden)
			return
		}
		s.serverError(w, err)
		return
	}
	http.Redirect(w, r, "/tree", http.StatusSeeOther)
}

func (s *Server) handleAdminResetPassword(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}

	id, ok := pathUserID(w, r)
	if !ok {
		return
	}
	target, err := s.db.UserByID(r.Context(), id)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if target == nil || target.DeletedAt != nil {
		http.NotFound(w, r)
		return
	}

	token, err := auth.GenerateToken()
	if err != nil {
		s.serverError(w, err)
		return
	}
	if err := s.db.CreatePasswordReset(r.Context(), token, target.ID, s.now()); err != nil {
		s.serverError(w, err)
		return
	}

	s.renderInviteTree(w, r, http.StatusOK, s.cfg.BaseURL+"/reset/"+token, target.DisplayName())
}

func (s *Server) handleAdminGrantAdmin(w http.ResponseWriter, r *http.Request) {
	s.setAdmin(w, r, true)
}

func (s *Server) handleAdminRevokeAdmin(w http.ResponseWriter, r *http.Request) {
	s.setAdmin(w, r, false)
}

func (s *Server) setAdmin(w http.ResponseWriter, r *http.Request, isAdmin bool) {
	if !s.requireAdmin(w, r) {
		return
	}

	id, ok := pathUserID(w, r)
	if !ok {
		return
	}
	if !isAdmin && id == currentUser(r).ID {
		http.Error(w, "You cannot remove your own admin rights.", http.StatusForbidden)
		return
	}

	target, err := s.db.UserByID(r.Context(), id)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if target == nil || target.DeletedAt != nil {
		http.NotFound(w, r)
		return
	}

	if _, err := s.db.SetAdmin(r.Context(), id, isAdmin); err != nil {
		if errors.Is(err, db.ErrLastAdmin) {
			http.Error(w, "The last active admin cannot lose their admin rights.", http.StatusForbidden)
			return
		}
		s.serverError(w, err)
		return
	}
	http.Redirect(w, r, "/tree", http.StatusSeeOther)
}

func pathUserID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return 0, false
	}
	return id, true
}
