package web

import (
	"net/http"
	"strings"

	"github.com/skorokithakis/soliloquy/internal/auth"
	"github.com/skorokithakis/soliloquy/internal/db"
)

const (
	// profileListLimit is how many threads and comments a profile shows. The
	// product has no paging for profiles.
	profileListLimit = 20
	// profileExcerptLength is the longest comment excerpt shown on a profile.
	profileExcerptLength = 160
)

type profileThreadView struct {
	PublicID    string
	Title       string
	Age         string
	CommentText string
}

type profileCommentView struct {
	PublicID       string
	ThreadPublicID string
	ThreadTitle    string
	Excerpt        string
	Age            string
}

type profilePageData struct {
	PageData
	Username        string
	Joined          string
	HasInviter      bool
	InviterUsername string
	InviterDeleted  bool
	IsAdmin         bool
	Disabled        bool
	Threads         []profileThreadView
	Comments        []profileCommentView
}

func (s *Server) handleProfilePage(w http.ResponseWriter, r *http.Request) {
	username := auth.NormalizeUsername(r.PathValue("username"))
	member, err := s.db.UserByUsername(r.Context(), username)
	if err != nil {
		s.serverError(w, err)
		return
	}
	if member == nil || member.DeletedAt != nil {
		http.NotFound(w, r)
		return
	}

	var inviter *db.User
	if member.InvitedBy != nil {
		inviter, err = s.db.UserByID(r.Context(), *member.InvitedBy)
		if err != nil {
			s.serverError(w, err)
			return
		}
	}

	threads, err := s.db.ListThreadsByAuthor(r.Context(), member.ID, profileListLimit)
	if err != nil {
		s.serverError(w, err)
		return
	}
	comments, err := s.db.ListCommentsByAuthor(r.Context(), member.ID, profileListLimit)
	if err != nil {
		s.serverError(w, err)
		return
	}

	now := s.now()
	threadViews := make([]profileThreadView, len(threads))
	for i, thread := range threads {
		threadViews[i] = profileThreadView{
			PublicID:    thread.PublicID,
			Title:       thread.Title,
			Age:         relativeTime(thread.CreatedAt, now),
			CommentText: commentsText(thread.CommentCount),
		}
	}
	commentViews := make([]profileCommentView, len(comments))
	for i, comment := range comments {
		commentViews[i] = profileCommentView{
			PublicID:       comment.PublicID,
			ThreadPublicID: comment.ThreadPublicID,
			ThreadTitle:    comment.ThreadTitle,
			Excerpt:        profileExcerpt(comment.Body, profileExcerptLength),
			Age:            relativeTime(comment.CreatedAt, now),
		}
	}

	data := profilePageData{
		PageData:   s.pageData(r, member.Username),
		Username:   member.Username,
		Joined:     member.CreatedAt.Format("2006-01-02"),
		HasInviter: inviter != nil,
		IsAdmin:    member.IsAdmin,
		Disabled:   member.DisabledAt != nil,
		Threads:    threadViews,
		Comments:   commentViews,
	}
	if inviter != nil {
		data.InviterUsername = inviter.Username
		data.InviterDeleted = inviter.DeletedAt != nil
	}
	s.render(w, "profile", data)
}

// profileExcerpt turns a comment's Markdown source into a one-line plain-text
// preview. It collapses whitespace and cuts at a word boundary, so the excerpt
// does not render any of the comment's markup.
func profileExcerpt(body string, limit int) string {
	text := strings.Join(strings.Fields(body), " ")
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}

	cut := limit
	for i := limit - 1; i >= limit/2; i-- {
		if runes[i] == ' ' {
			cut = i
			break
		}
	}
	return string(runes[:cut]) + "…"
}
