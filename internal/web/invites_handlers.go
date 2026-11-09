package web

import (
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/skorokithakis/soliloquy/internal/auth"
	"github.com/skorokithakis/soliloquy/internal/db"
)

type inviteTreeRowView struct {
	Prefix   string
	Name     string
	Joined   string
	Deleted  bool
	IsAdmin  bool
	IsYou    bool
	Disabled bool
}

type inviteLinkView struct {
	Short      string
	URL        string
	State      string
	Used       bool
	UsedByName string
	Available  bool
}

type invitesPageData struct {
	PageData
	Tree      []inviteTreeRowView
	Accounts  int
	Deleted   int
	Disabled  int
	NewestURL string
	Links     []inviteLinkView
}

func (s *Server) handleInvitesPage(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	users, err := s.db.ListUsers(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}
	invites, err := s.db.ListInvites(r.Context(), user.ID)
	if err != nil {
		s.serverError(w, err)
		return
	}

	now := s.now()
	links, newestURL := inviteLinkViews(invites, s.cfg.BaseURL, now)
	tree, accounts, deleted, disabled := inviteTreeRows(db.BuildInviteTree(users), user.ID)

	s.render(w, "invites", invitesPageData{
		PageData:  s.pageData(r, "Invites"),
		Tree:      tree,
		Accounts:  accounts,
		Deleted:   deleted,
		Disabled:  disabled,
		NewestURL: newestURL,
		Links:     links,
	})
}

// inviteTreeRows flattens the invite tree into display rows with branch
// prefixes, and returns the summary counts.
func inviteTreeRows(nodes []*db.InviteNode, viewerID int64) ([]inviteTreeRowView, int, int, int) {
	var rows []inviteTreeRowView
	accounts := 0
	deleted := 0
	disabled := 0

	var walk func([]*db.InviteNode, string, int)
	walk = func(siblings []*db.InviteNode, prefix string, depth int) {
		for i, node := range siblings {
			last := i == len(siblings)-1
			branch := ""
			childPrefix := prefix
			if depth > 0 {
				if last {
					branch = "└── "
					childPrefix = prefix + "    "
				} else {
					branch = "├── "
					childPrefix = prefix + "│   "
				}
			}

			user := node.User
			isDeleted := user.DeletedAt != nil
			isDisabled := !isDeleted && user.DisabledAt != nil
			if isDeleted {
				deleted++
			} else {
				accounts++
			}
			if isDisabled {
				disabled++
			}

			rows = append(rows, inviteTreeRowView{
				Prefix:   prefix + branch,
				Name:     user.DisplayName(),
				Joined:   user.CreatedAt.Format("2006-01-02"),
				Deleted:  isDeleted,
				IsAdmin:  !isDeleted && user.IsAdmin,
				IsYou:    !isDeleted && user.ID == viewerID,
				Disabled: isDisabled,
			})
			walk(node.Children, childPrefix, depth+1)
		}
	}

	walk(nodes, "", 0)
	return rows, accounts, deleted, disabled
}

func inviteLinkViews(invites []db.Invite, baseURL string, now time.Time) ([]inviteLinkView, string) {
	views := make([]inviteLinkView, 0, len(invites))
	newestURL := ""

	for _, invite := range invites {
		view := inviteLinkView{
			Short: "…/" + tokenSuffix(invite.Token, 8),
			URL:   baseURL + "/invite/" + invite.Token,
		}

		switch {
		case invite.UsedAt != nil:
			name := invite.UsedByName
			if invite.UsedByDeleted {
				name = "[deleted user]"
			}
			view.Used = true
			view.UsedByName = name
		case !now.Before(invite.ExpiresAt):
			view.State = "expired " + invite.ExpiresAt.Format("2006-01-02")
		default:
			days := int(math.Ceil(invite.ExpiresAt.Sub(now).Hours() / 24))
			view.State = fmt.Sprintf("unused · %dd left", days)
			view.Available = true
			if newestURL == "" {
				newestURL = view.URL
			}
		}

		views = append(views, view)
	}

	return views, newestURL
}

func tokenSuffix(token string, n int) string {
	if len(token) <= n {
		return token
	}
	return token[len(token)-n:]
}

func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	token, err := auth.GenerateInviteToken()
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
