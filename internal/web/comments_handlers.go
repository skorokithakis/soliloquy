package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/skorokithakis/soliloquy/internal/db"
	"github.com/skorokithakis/soliloquy/internal/markdown"
)

const maxCommentBodyLength = 20000

// reactionOption is one supported reaction, with the emoji and the name shown
// to assistive technology.
type reactionOption struct {
	Key   db.Reaction
	Emoji string
	Name  string
}

// reactionOptions lists the supported reactions in display order. The keys
// match db.CommentReactions; app.js keeps the same list for live updates.
var reactionOptions = []reactionOption{
	{Key: db.ReactionHeart, Emoji: "❤️", Name: "Love"},
	{Key: db.ReactionLaugh, Emoji: "😂", Name: "Funny"},
	{Key: db.ReactionIdea, Emoji: "💡", Name: "Insightful"},
	{Key: db.ReactionThanks, Emoji: "🙏", Name: "Thanks"},
	{Key: db.ReactionParty, Emoji: "🎉", Name: "Congratulations"},
}

// reactionView is one reaction with the count a comment shows. Mine marks the
// viewer's own reaction, which the template highlights.
type reactionView struct {
	Key   string
	Emoji string
	Name  string
	Count int64
	Mine  bool
}

// commentView is a comment prepared for the comment template. The thread page
// and live updates both render comments through renderCommentHTML so they look
// the same. Viewer-specific controls are only emitted when IsAuthor or CanVote
// is set.
type commentView struct {
	PublicID       string
	ParentPublicID string
	HasParent      bool
	Author         string
	AuthorUsername string
	AuthorDeleted  bool
	Score          int64
	ScoreText      string
	CreatedAt      string
	Edited         bool
	Body           template.HTML
	Deleted        bool
	IsAuthor       bool
	CanDelete      bool
	Voted          bool
	CanVote        bool
	IsOP           bool
	// Reactions holds the reactions with a count above zero, in display
	// order. Mine marks the viewer's own reaction.
	Reactions []reactionView
	// ReactionOptions lists every reaction, so the "react" control can offer
	// the keys that have no count yet.
	ReactionOptions []reactionOption
	// LoggedIn says whether the viewer has an account. Visitors get no vote,
	// reply, edit or delete controls, and no account link on the author name.
	LoggedIn bool
	// New marks a comment newer than the viewer's read cutoff, so the page
	// shows it the way app.js shows a comment that arrived live.
	New     bool
	Replies []commentView
}

type commentFormPageData struct {
	PageData
	Heading     string
	Action      string
	SubmitLabel string
	FormBody    string
	BodyLength  int
	CancelURL   string
}

// reactionCountResponse is one reaction's count in a JSON vote response or a
// live vote event. The list keeps the display order and includes reactions
// with no votes, so a page can rebuild the chips from it.
type reactionCountResponse struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

func reactionCountResponses(counts map[db.Reaction]int64) []reactionCountResponse {
	if counts == nil {
		return nil
	}

	list := make([]reactionCountResponse, 0, len(reactionOptions))
	for _, option := range reactionOptions {
		list = append(list, reactionCountResponse{Key: string(option.Key), Count: counts[option.Key]})
	}
	return list
}

type voteResponse struct {
	Score      int64  `json:"score"`
	ScoreText  string `json:"score_text"`
	Voted      bool   `json:"voted"`
	ButtonText string `json:"button_text"`
	// Reaction is the member's own reaction, empty when they have none.
	// Reactions is nil for thread votes.
	Reaction  string                  `json:"reaction"`
	Reactions []reactionCountResponse `json:"reactions,omitempty"`
}

// renderCommentHTML renders one comment with the same template the thread page
// uses. Callers sending a comment to other viewers, such as the live-update
// stream, should leave CanVote and IsAuthor false so viewer-specific controls
// are not included.
func (s *Server) renderCommentHTML(view commentView) (template.HTML, error) {
	var buf bytes.Buffer
	if err := s.commentTemplate.ExecuteTemplate(&buf, "comment", view); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

func commentViews(nodes []*db.CommentNode, viewer *db.User, threadAuthorID int64, now, newSince time.Time) []commentView {
	views := make([]commentView, len(nodes))
	for i, node := range nodes {
		views[i] = commentViewFor(node, viewer, threadAuthorID, now, newSince)
	}
	return views
}

func commentViewFor(node *db.CommentNode, viewer *db.User, threadAuthorID int64, now, newSince time.Time) commentView {
	comment := node.Comment
	view := commentView{
		PublicID:       comment.PublicID,
		Author:         comment.AuthorName(),
		AuthorUsername: comment.AuthorUsername,
		AuthorDeleted:  comment.AuthorDeletedAt != nil,
		Score:          comment.Score,
		ScoreText:      pointsText(comment.Score),
		CreatedAt:      relativeTime(comment.CreatedAt, now),
		Edited:         comment.EditedAt != nil,
		Deleted:        comment.DeletedAt != nil,
		IsAuthor:       viewer != nil && viewer.ID == comment.AuthorID && comment.DeletedAt == nil,
		CanDelete:      viewer != nil && (viewer.ID == comment.AuthorID || viewer.IsAdmin) && comment.DeletedAt == nil,
		Voted:          comment.Voted,
		CanVote:        viewer != nil && viewer.ID != comment.AuthorID && comment.DeletedAt == nil,
		IsOP:           comment.AuthorID == threadAuthorID,
		LoggedIn:       viewer != nil,
		// A deleted comment keeps its place in the tree but has no content to
		// mark, and the reader's own comments never count as new. The zero
		// cutoff means this comment list does not mark new comments at all.
		New:     viewer != nil && !newSince.IsZero() && comment.DeletedAt == nil && comment.AuthorID != viewer.ID && comment.CreatedAt.After(newSince),
		Replies: commentViews(node.Replies, viewer, threadAuthorID, now, newSince),
		// Every reaction is offered by the control; only those with a count
		// are shown as chips.
		ReactionOptions: reactionOptions,
	}
	if comment.ParentPublicID != nil {
		view.ParentPublicID = *comment.ParentPublicID
		view.HasParent = true
	}
	if comment.DeletedAt == nil {
		view.Body = template.HTML(comment.BodyHTML)
		for _, option := range reactionOptions {
			count := comment.ReactionCounts[option.Key]
			if count == 0 {
				continue
			}
			view.Reactions = append(view.Reactions, reactionView{
				Key:   string(option.Key),
				Emoji: option.Emoji,
				Name:  option.Name,
				Count: count,
				Mine:  comment.Reaction == option.Key,
			})
		}
	}
	return view
}

func (s *Server) handleCreateComment(w http.ResponseWriter, r *http.Request) {
	thread, ok := s.visibleThread(w, r)
	if !ok {
		return
	}

	body := r.FormValue("body")
	bodyHTML, err := renderCommentBody(body)
	if err != nil {
		s.renderThreadPage(w, r, thread, time.Time{}, http.StatusBadRequest, body, err.Error())
		return
	}

	comment, err := s.db.CreateComment(r.Context(), thread.ID, nil, currentUser(r).ID, body, bodyHTML, s.now())
	if err != nil {
		if errors.Is(err, db.ErrCommentNotFound) {
			http.NotFound(w, r)
			return
		}
		s.serverError(w, err)
		return
	}

	recipients, err := s.notifyNewComment(r.Context(), thread.ID, thread.AuthorID, comment, nil)
	s.logNotificationError(err)
	s.publishBadgeCounts(r.Context(), recipients)
	s.publishNewComment(r.Context(), thread.ID, comment)

	s.setDraftPostedCookie(w, r)
	http.Redirect(w, r, commentAnchorPath(thread.PublicID, comment.PublicID), http.StatusSeeOther)
}

func (s *Server) handleReplyPage(w http.ResponseWriter, r *http.Request) {
	comment, ok := s.visibleComment(w, r)
	if !ok {
		return
	}

	s.renderCommentForm(w, r, http.StatusOK, "Reply", "comments/reply", commentAction(comment.PublicID, "reply"), "post reply", "", "", commentAnchorPath(comment.ThreadPublicID, comment.PublicID))
}

func (s *Server) handleCreateReply(w http.ResponseWriter, r *http.Request) {
	comment, ok := s.visibleComment(w, r)
	if !ok {
		return
	}

	body := r.FormValue("body")
	bodyHTML, err := renderCommentBody(body)
	if err != nil {
		s.renderCommentForm(w, r, http.StatusBadRequest, "Reply", "comments/reply", commentAction(comment.PublicID, "reply"), "post reply", body, err.Error(), commentAnchorPath(comment.ThreadPublicID, comment.PublicID))
		return
	}

	reply, err := s.db.CreateComment(r.Context(), comment.ThreadID, &comment.ID, currentUser(r).ID, body, bodyHTML, s.now())
	if err != nil {
		if errors.Is(err, db.ErrCommentNotFound) {
			http.NotFound(w, r)
			return
		}
		s.serverError(w, err)
		return
	}

	recipients, err := s.notifyNewComment(r.Context(), comment.ThreadID, 0, reply, comment)
	s.logNotificationError(err)
	s.publishBadgeCounts(r.Context(), recipients)
	s.publishNewComment(r.Context(), comment.ThreadID, reply)

	s.setDraftPostedCookie(w, r)
	http.Redirect(w, r, commentAnchorPath(reply.ThreadPublicID, reply.PublicID), http.StatusSeeOther)
}

func (s *Server) handleEditCommentPage(w http.ResponseWriter, r *http.Request) {
	comment, ok := s.visibleComment(w, r)
	if !ok {
		return
	}
	if currentUser(r).ID != comment.AuthorID {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}

	s.renderCommentForm(w, r, http.StatusOK, "Edit comment", "comments/edit", commentAction(comment.PublicID, "edit"), "save changes", comment.Body, "", commentAnchorPath(comment.ThreadPublicID, comment.PublicID))
}

func (s *Server) handleUpdateComment(w http.ResponseWriter, r *http.Request) {
	comment, ok := s.visibleComment(w, r)
	if !ok {
		return
	}
	if currentUser(r).ID != comment.AuthorID {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}

	body := r.FormValue("body")
	bodyHTML, err := renderCommentBody(body)
	if err != nil {
		s.renderCommentForm(w, r, http.StatusBadRequest, "Edit comment", "comments/edit", commentAction(comment.PublicID, "edit"), "save changes", body, err.Error(), commentAnchorPath(comment.ThreadPublicID, comment.PublicID))
		return
	}

	updated, err := s.db.UpdateComment(r.Context(), comment.ID, comment.AuthorID, body, bodyHTML, s.now())
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !updated {
		http.NotFound(w, r)
		return
	}

	recipients, err := s.notifyMentions(r.Context(), comment.AuthorID, comment.ThreadID, &comment.ID, body, comment.Body, nil)
	s.logNotificationError(err)
	s.publishBadgeCounts(r.Context(), recipients)
	// The stored HTML is already rendered and sanitised, so viewers see the
	// same markup the author will after the redirect.
	s.publishCommentEdit(comment, bodyHTML)

	s.setDraftPostedCookie(w, r)
	http.Redirect(w, r, commentAnchorPath(comment.ThreadPublicID, comment.PublicID), http.StatusSeeOther)
}

func (s *Server) handleDeleteComment(w http.ResponseWriter, r *http.Request) {
	comment, ok := s.visibleComment(w, r)
	if !ok {
		return
	}

	user := currentUser(r)
	if user.ID != comment.AuthorID && !user.IsAdmin {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}

	if deleted, err := s.db.DeleteComment(r.Context(), comment.ID, s.now()); err != nil {
		s.serverError(w, err)
		return
	} else if deleted {
		s.publishCommentDelete(r.Context(), comment)
	}

	http.Redirect(w, r, commentAnchorPath(comment.ThreadPublicID, comment.PublicID), http.StatusSeeOther)
}

func (s *Server) handleVoteThread(w http.ResponseWriter, r *http.Request) {
	thread, ok := s.visibleThread(w, r)
	if !ok {
		return
	}

	result, err := s.db.ToggleThreadVote(r.Context(), currentUser(r).ID, thread.ID)
	if !s.voteError(w, err) {
		return
	}

	s.publishVote(thread.ID, votePayload(nil, result))
	s.respondVote(w, r, result, threadPath(thread.PublicID))
}

func (s *Server) handleVoteComment(w http.ResponseWriter, r *http.Request) {
	comment, ok := s.visibleComment(w, r)
	if !ok {
		return
	}

	result, err := s.db.ToggleCommentVote(r.Context(), currentUser(r).ID, comment.ID)
	if !s.voteError(w, err) {
		return
	}

	s.publishVote(comment.ThreadID, votePayload(&comment.PublicID, result))
	s.respondVote(w, r, result, commentAnchorPath(comment.ThreadPublicID, comment.PublicID))
}

// handleReactComment sets, replaces, or removes the viewer's emoji reaction on
// a comment. The rules live in the database layer; this handler only rejects
// an unknown key and mirrors the vote handler's responses.
func (s *Server) handleReactComment(w http.ResponseWriter, r *http.Request) {
	comment, ok := s.visibleComment(w, r)
	if !ok {
		return
	}

	reaction := db.Reaction(r.FormValue("reaction"))
	if !reaction.Valid() {
		http.Error(w, "Unknown reaction.", http.StatusBadRequest)
		return
	}

	result, err := s.db.SetCommentReaction(r.Context(), currentUser(r).ID, comment.ID, reaction)
	if !s.voteError(w, err) {
		return
	}

	s.publishVote(comment.ThreadID, votePayload(&comment.PublicID, result))
	s.respondVote(w, r, result, commentAnchorPath(comment.ThreadPublicID, comment.PublicID))
}

// voteError writes the response for a vote error and reports whether the
// caller may continue.
func (s *Server) voteError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, db.ErrVoteOwnPost):
		http.Error(w, "You cannot vote on your own post.", http.StatusForbidden)
	case errors.Is(err, db.ErrVoteNotFound):
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
	default:
		s.serverError(w, err)
	}
	return false
}

// respondVote redirects a plain form submission or returns JSON to the
// script's fetch request.
func (s *Server) respondVote(w http.ResponseWriter, r *http.Request, result db.VoteResult, redirect string) {
	if !wantsJSON(r) {
		http.Redirect(w, r, redirect, http.StatusSeeOther)
		return
	}

	buttonText := "Upvote"
	if result.Voted {
		buttonText = "Unvote"
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(voteResponse{
		Score:      result.Score,
		ScoreText:  pointsText(result.Score),
		Voted:      result.Voted,
		ButtonText: buttonText,
		Reaction:   string(result.Reaction),
		Reactions:  reactionCountResponses(result.ReactionCounts),
	}); err != nil {
		s.serverError(w, err)
	}
}

func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}

func (s *Server) renderCommentForm(w http.ResponseWriter, r *http.Request, status int, title, heading, action, submitLabel, body, message, cancelURL string) {
	data := commentFormPageData{
		PageData:    s.pageData(r, title),
		Heading:     heading,
		Action:      action,
		SubmitLabel: submitLabel,
		FormBody:    body,
		BodyLength:  utf8.RuneCountInString(body),
		CancelURL:   cancelURL,
	}
	data.Error = message
	s.renderStatus(w, status, "comment_form", data)
}

// visibleComment loads the comment named in the path. Deleted comments and
// comments on deleted threads are treated as absent for every write, because
// replies and edits attach to live content only.
func (s *Server) visibleComment(w http.ResponseWriter, r *http.Request) (*db.Comment, bool) {
	publicID := r.PathValue("id")
	if !validPublicID(publicID) {
		http.NotFound(w, r)
		return nil, false
	}

	comment, err := s.db.CommentByPublicID(r.Context(), publicID, currentUser(r).ID)
	if err != nil {
		s.serverError(w, err)
		return nil, false
	}
	if comment == nil || comment.DeletedAt != nil {
		http.NotFound(w, r)
		return nil, false
	}

	thread, err := s.db.ThreadByID(r.Context(), comment.ThreadID)
	if err != nil {
		s.serverError(w, err)
		return nil, false
	}
	if thread == nil || thread.DeletedAt != nil {
		http.NotFound(w, r)
		return nil, false
	}
	return comment, true
}

func renderCommentBody(body string) (string, error) {
	if strings.TrimSpace(body) == "" {
		return "", errors.New("Comment is required.")
	}
	if utf8.RuneCountInString(body) > maxCommentBodyLength {
		return "", fmt.Errorf("Comment must be %d characters or fewer.", maxCommentBodyLength)
	}

	rendered, err := markdown.Render(body)
	if err != nil {
		return "", err
	}
	return string(rendered), nil
}

func commentAction(publicID string, action string) string {
	return "/comments/" + publicID + "/" + action
}

func commentAnchorPath(threadPublicID, commentPublicID string) string {
	return threadPath(threadPublicID) + "#c" + commentPublicID
}
