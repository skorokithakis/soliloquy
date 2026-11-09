package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/skorokithakis/soliloquy/internal/auth"
	"github.com/skorokithakis/soliloquy/internal/db"
)

type adminUserView struct {
	ID        int64
	Username  string
	IsAdmin   bool
	IsYou     bool
	Disabled  bool
	InvitedBy string
	Joined    string
	Status    string
}

type adminPageData struct {
	PageData
	Users       []adminUserView
	Accounts    int
	Admins      int
	Disabled    int
	ResetURL    string
	ResetFor    string
	ResetUserID int64
}

func (s *Server) handleAdminUsersPage(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	s.renderAdminUsers(w, r, http.StatusOK, "", "", 0)
}

func (s *Server) renderAdminUsers(w http.ResponseWriter, r *http.Request, status int, resetURL, resetFor string, resetUserID int64) {
	users, err := s.db.ListUsers(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}

	byID := make(map[int64]db.User, len(users))
	for _, user := range users {
		byID[user.ID] = user
	}

	viewer := currentUser(r)
	views := make([]adminUserView, 0, len(users))
	accounts := 0
	admins := 0
	disabled := 0
	for _, user := range users {
		if user.DeletedAt != nil {
			continue
		}

		accounts++
		if user.IsAdmin {
			admins++
		}
		if user.DisabledAt != nil {
			disabled++
		}

		invitedBy := "(root)"
		if user.InvitedBy != nil {
			invitedBy = "[deleted user]"
			if inviter, ok := byID[*user.InvitedBy]; ok && inviter.DeletedAt == nil {
				invitedBy = inviter.Username
			}
		}

		statusText := "active"
		if user.DisabledAt != nil {
			statusText = "disabled"
		}

		views = append(views, adminUserView{
			ID:        user.ID,
			Username:  user.Username,
			IsAdmin:   user.IsAdmin,
			IsYou:     viewer != nil && viewer.ID == user.ID,
			Disabled:  user.DisabledAt != nil,
			InvitedBy: invitedBy,
			Joined:    user.CreatedAt.Format("2006-01-02"),
			Status:    statusText,
		})
	}

	s.renderStatus(w, status, "admin_users", adminPageData{
		PageData:    s.pageData(r, "Admin users"),
		Users:       views,
		Accounts:    accounts,
		Admins:      admins,
		Disabled:    disabled,
		ResetURL:    resetURL,
		ResetFor:    resetFor,
		ResetUserID: resetUserID,
	})
}

func (s *Server) handleInviteTree(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/invites", http.StatusSeeOther)
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
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
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

	s.renderAdminUsers(w, r, http.StatusOK, s.cfg.BaseURL+"/reset/"+token, target.DisplayName(), target.ID)
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
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

func pathUserID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return 0, false
	}
	return id, true
}
