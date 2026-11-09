package web

import (
	"errors"
	"net/http"

	"github.com/skorokithakis/soliloquy/internal/auth"
	"github.com/skorokithakis/soliloquy/internal/db"
)

type resetPageData struct {
	PageData
	Token string
	Valid bool
}

func (s *Server) handleResetPage(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if _, err := s.db.ActivePasswordReset(r.Context(), token, s.now()); err != nil {
		if !errors.Is(err, db.ErrResetInvalid) {
			s.serverError(w, err)
			return
		}
		s.renderInvalidReset(w, r, token)
		return
	}

	s.render(w, "reset", resetPageData{
		PageData: s.pageData(r, "Reset password"),
		Token:    token,
		Valid:    true,
	})
}

func (s *Server) handlePasswordReset(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")

	// Check the link before hashing the password, so invalid tokens cannot
	// make unauthenticated requests run bcrypt.
	if _, err := s.db.ActivePasswordReset(r.Context(), token, s.now()); err != nil {
		if !errors.Is(err, db.ErrResetInvalid) {
			s.serverError(w, err)
			return
		}
		s.renderInvalidReset(w, r, token)
		return
	}

	password := r.FormValue("password")
	confirm := r.FormValue("confirm_password")
	if password != confirm {
		s.renderReset(w, r, http.StatusBadRequest, token, "Passwords do not match.")
		return
	}

	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		s.renderReset(w, r, http.StatusBadRequest, token, err.Error())
		return
	}

	if _, err := s.db.RedeemPasswordReset(r.Context(), token, passwordHash, s.now()); err != nil {
		switch {
		case errors.Is(err, db.ErrResetInvalid):
			s.renderInvalidReset(w, r, token)
		default:
			s.serverError(w, err)
		}
		return
	}

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) renderReset(w http.ResponseWriter, r *http.Request, status int, token, message string) {
	s.renderStatus(w, status, "reset", resetPageData{
		PageData: s.pageDataWithError(r, "Reset password", message),
		Token:    token,
		Valid:    true,
	})
}

func (s *Server) renderInvalidReset(w http.ResponseWriter, r *http.Request, token string) {
	s.renderStatus(w, http.StatusNotFound, "reset", resetPageData{
		PageData: s.pageDataWithError(r, "Reset password", "This password reset link is invalid or has expired."),
		Token:    token,
	})
}
