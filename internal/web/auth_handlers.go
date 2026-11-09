package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/skorokithakis/soliloquy/internal/auth"
	"github.com/skorokithakis/soliloquy/internal/db"
)

type loginPageData struct {
	PageData
	Username    string
	Host        string
	WaitSeconds int
}

type signupChainLine struct {
	Prefix string
	Name   string
	You    bool
}

type signupPageData struct {
	PageData
	Token    string
	Valid    bool
	Username string
	Inviter  string
	DaysLeft int
	Chain    []signupChainLine
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if currentUser(r) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.render(w, "login", loginPageData{PageData: s.pageData(r, "Log in"), Host: s.baseHost})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	username := auth.NormalizeUsername(r.FormValue("username"))
	password := r.FormValue("password")
	now := s.now()

	if allowed, wait := s.throttle.Allowed(username); !allowed {
		seconds := int((wait + time.Second - 1) / time.Second)
		w.Header().Set("Retry-After", fmt.Sprintf("%d", seconds))
		s.renderStatus(w, http.StatusTooManyRequests, "login", loginPageData{
			PageData: PageData{
				Title:       "Log in",
				CurrentUser: currentUser(r),
				Error:       fmt.Sprintf("Too many attempts. Wait %d s before the next try. The wait doubles after each miss, up to one minute.", seconds),
			},
			Username:    username,
			Host:        s.baseHost,
			WaitSeconds: seconds,
		})
		return
	}

	user, err := s.db.UserByUsername(r.Context(), username)
	if err != nil {
		s.throttle.Release(username)
		s.serverError(w, err)
		return
	}

	passwordOK := user != nil && auth.CheckPassword(user.PasswordHash, password)
	if !passwordOK || !user.Active() {
		s.throttle.Fail(username)
		s.renderStatus(w, http.StatusUnauthorized, "login", loginPageData{
			PageData: PageData{
				Title:       "Log in",
				CurrentUser: currentUser(r),
				Error:       "Login incorrect.",
			},
			Username: username,
			Host:     s.baseHost,
		})
		return
	}

	created, err := s.completeLogin(w, r, user, now)
	if err != nil {
		s.throttle.Release(username)
		s.serverError(w, err)
		return
	}
	if !created {
		s.renderStatus(w, http.StatusUnauthorized, "login", loginPageData{
			PageData: PageData{
				Title:       "Log in",
				CurrentUser: currentUser(r),
				Error:       "Login incorrect.",
			},
			Username: username,
			Host:     s.baseHost,
		})
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// completeLogin creates a session only if the password hash that was just
// verified is still current. A concurrent password change makes this fail,
// which counts as a failed login rather than creating a session for the old
// password.
func (s *Server) completeLogin(w http.ResponseWriter, r *http.Request, user *db.User, now time.Time) (bool, error) {
	created, err := s.startSessionIfPasswordUnchanged(w, r, user.ID, user.PasswordHash, now)
	if err != nil {
		return false, err
	}
	if !created {
		s.throttle.Fail(user.Username)
		return false, nil
	}

	s.throttle.Success(user.Username)
	return true, nil
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if tokenHash := currentSessionHash(r); tokenHash != "" {
		if err := s.db.DeleteSession(r.Context(), tokenHash); err != nil {
			slog.Error("delete session", "error", err)
		}
	}
	s.clearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) handleSignupPage(w http.ResponseWriter, r *http.Request) {
	if currentUser(r) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	token := r.PathValue("token")
	invite, err := s.db.ActiveInvite(r.Context(), token, s.now())
	if err != nil {
		if !errors.Is(err, db.ErrInviteInvalid) {
			s.serverError(w, err)
			return
		}
		s.renderInvalidInvite(w, r, token)
		return
	}

	inviter, daysLeft, chain, err := s.signupInviteDetails(r.Context(), invite)
	if err != nil {
		s.serverError(w, err)
		return
	}

	s.render(w, "signup", signupPageData{
		PageData: s.pageData(r, "Sign up"),
		Token:    token,
		Valid:    true,
		Inviter:  inviter,
		DaysLeft: daysLeft,
		Chain:    chain,
	})
}

func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")

	// Check the invite before hashing the password, so invalid tokens cannot
	// make unauthenticated requests run bcrypt.
	if _, err := s.db.ActiveInvite(r.Context(), token, s.now()); err != nil {
		if !errors.Is(err, db.ErrInviteInvalid) {
			s.serverError(w, err)
			return
		}
		s.renderInvalidInvite(w, r, token)
		return
	}

	rawUsername := r.FormValue("username")

	username, err := auth.ValidateUsername(rawUsername)
	if err != nil {
		s.renderSignup(w, r, http.StatusBadRequest, token, rawUsername, err.Error())
		return
	}

	password := r.FormValue("password")
	if password != r.FormValue("confirm_password") {
		s.renderSignup(w, r, http.StatusBadRequest, token, rawUsername, "Passwords do not match.")
		return
	}

	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		s.renderSignup(w, r, http.StatusBadRequest, token, rawUsername, err.Error())
		return
	}

	user, err := s.db.RedeemInvite(r.Context(), token, username, passwordHash, s.now())
	switch {
	case errors.Is(err, db.ErrInviteInvalid):
		s.renderInvalidInvite(w, r, token)
		return
	case errors.Is(err, db.ErrUsernameTaken):
		s.renderSignup(w, r, http.StatusBadRequest, token, username, "That username is already taken.")
		return
	case err != nil:
		s.serverError(w, err)
		return
	}

	if err := s.startSession(w, r, user.ID, s.now()); err != nil {
		s.serverError(w, err)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) renderInvalidInvite(w http.ResponseWriter, r *http.Request, token string) {
	s.renderStatus(w, http.StatusNotFound, "signup", signupPageData{
		PageData: PageData{
			Title:       "Sign up",
			CurrentUser: currentUser(r),
			Error:       "This invite link is invalid or has expired.",
		},
		Token: token,
	})
}

func (s *Server) renderSignup(w http.ResponseWriter, r *http.Request, status int, token, username, message string) {
	data := signupPageData{
		PageData: PageData{
			Title:       "Sign up",
			CurrentUser: currentUser(r),
			Error:       message,
		},
		Token:    token,
		Valid:    true,
		Username: username,
	}

	invite, err := s.db.ActiveInvite(r.Context(), token, s.now())
	if err != nil {
		if !errors.Is(err, db.ErrInviteInvalid) {
			s.serverError(w, err)
			return
		}
		data.Valid = false
	} else {
		inviter, daysLeft, chain, err := s.signupInviteDetails(r.Context(), invite)
		if err != nil {
			s.serverError(w, err)
			return
		}
		data.Inviter = inviter
		data.DaysLeft = daysLeft
		data.Chain = chain
	}

	s.renderStatus(w, status, "signup", data)
}

func (s *Server) signupInviteDetails(ctx context.Context, invite *db.Invite) (string, int, []signupChainLine, error) {
	daysLeft := int(math.Ceil(invite.ExpiresAt.Sub(s.now()).Hours() / 24))

	var users []db.User
	if invite.CreatedBy != nil {
		current, err := s.db.UserByID(ctx, *invite.CreatedBy)
		if err != nil {
			return "", 0, nil, err
		}
		for current != nil {
			users = append(users, *current)
			if current.InvitedBy == nil {
				break
			}
			parent, err := s.db.UserByID(ctx, *current.InvitedBy)
			if err != nil {
				return "", 0, nil, err
			}
			current = parent
		}
	}

	for i, j := 0, len(users)-1; i < j; i, j = i+1, j-1 {
		users[i], users[j] = users[j], users[i]
	}

	chain := make([]signupChainLine, 0, len(users)+1)
	for i, user := range users {
		prefix := ""
		if i > 0 {
			prefix = strings.Repeat("    ", i-1) + "└── "
		}
		chain = append(chain, signupChainLine{Prefix: prefix, Name: user.DisplayName()})
	}

	inviter := "(root)"
	youPrefix := ""
	if len(users) > 0 {
		inviter = users[len(users)-1].DisplayName()
		youPrefix = strings.Repeat("    ", len(users)-1) + "└── "
	}
	chain = append(chain, signupChainLine{Prefix: youPrefix, Name: "you", You: true})

	return inviter, daysLeft, chain, nil
}

type usernameCheckResponse struct {
	Valid     bool `json:"valid"`
	Available bool `json:"available"`
}

// handleInviteUsernameCheck reports whether a username passes the normal rules
// and is free. It only works while the invite token is valid and unused.
func (s *Server) handleInviteUsernameCheck(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if _, err := s.db.ActiveInvite(r.Context(), token, s.now()); err != nil {
		if errors.Is(err, db.ErrInviteInvalid) {
			http.NotFound(w, r)
			return
		}
		s.serverError(w, err)
		return
	}

	response := usernameCheckResponse{}
	username, err := auth.ValidateUsername(r.URL.Query().Get("username"))
	if err == nil {
		response.Valid = true
		user, err := s.db.UserByUsername(r.Context(), username)
		if err != nil {
			s.serverError(w, err)
			return
		}
		response.Available = user == nil
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.serverError(w, err)
	}
}

func (s *Server) handlePasswordPage(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	current := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirm := r.FormValue("confirm_password")

	fail := func(message string) {
		s.renderAccountError(w, r, http.StatusBadRequest, message)
	}

	if !auth.CheckPassword(user.PasswordHash, current) {
		fail("Current password is incorrect.")
		return
	}
	if newPassword != confirm {
		fail("New passwords do not match.")
		return
	}

	passwordHash, err := auth.HashPassword(newPassword)
	if err != nil {
		fail(err.Error())
		return
	}

	changed, err := s.db.ChangePassword(r.Context(), user.ID, user.PasswordHash, passwordHash, currentSessionHash(r))
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !changed {
		fail("Your password changed during this request. Please try again.")
		return
	}

	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	if !strings.EqualFold(strings.TrimSpace(r.FormValue("username")), user.Username) {
		s.renderAccountError(w, r, http.StatusBadRequest, "Type your username to confirm.")
		return
	}

	if err := s.db.DeleteAccount(r.Context(), user.ID, s.now()); err != nil {
		if errors.Is(err, db.ErrLastAdmin) {
			s.renderAccountError(w, r, http.StatusBadRequest, "The last active admin cannot delete their account.")
			return
		}
		s.serverError(w, err)
		return
	}

	s.clearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
