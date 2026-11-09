package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/skorokithakis/soliloquy/internal/db"
)

const maxCommentBodyLength = 20000

// commentView is a comment prepared for the comment template. The thread page
// and live updates both render comments through renderCommentHTML so they look
// the same. Viewer-specific controls are only emitted when IsAuthor or CanVote
// is set.
type commentView struct {
	ID            int64
	ThreadID      int64
	ParentID      int64
	HasParent     bool
	Author        string
	AuthorDeleted bool
	Score         int64
	ScoreText     string
	CreatedAt     string
	Edited        bool
	Body          template.HTML
	Deleted       bool
	IsAuthor      bool
	CanDelete     bool
	Voted         bool
	CanVote       bool
	IsOP          bool
	Replies       []commentView
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

type voteResponse struct {
	Score      int64  `json:"score"`
	ScoreText  string `json:"score_text"`
	Voted      bool   `json:"voted"`
	ButtonText string `json:"button_text"`
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

func commentViews(nodes []*db.CommentNode, viewer *db.User, threadAuthorID int64, now time.Time) []commentView {
	views := make([]commentView, len(nodes))
	for i, node := range nodes {
		views[i] = commentViewFor(node, viewer, threadAuthorID, now)
	}
	return views
}

func commentViewFor(node *db.CommentNode, viewer *db.User, threadAuthorID int64, now time.Time) commentView {
	comment := node.Comment
	view := commentView{
		ID:            comment.ID,
		ThreadID:      comment.ThreadID,
		Author:        comment.AuthorName(),
		AuthorDeleted: comment.AuthorDeletedAt != nil,
		Score:         comment.Score,
		ScoreText:     pointsText(comment.Score),
		CreatedAt:     relativeTime(comment.CreatedAt, now),
		Edited:        comment.EditedAt != nil,
		Deleted:       comment.DeletedAt != nil,
		IsAuthor:      viewer != nil && viewer.ID == comment.AuthorID && comment.DeletedAt == nil,
		CanDelete:     viewer != nil && (viewer.ID == comment.AuthorID || viewer.IsAdmin) && comment.DeletedAt == nil,
		Voted:         comment.Voted,
		CanVote:       viewer != nil && viewer.ID != comment.AuthorID && comment.DeletedAt == nil,
		IsOP:          comment.AuthorID == threadAuthorID,
		Replies:       commentViews(node.Replies, viewer, threadAuthorID, now),
	}
	if comment.ParentID != nil {
		view.ParentID = *comment.ParentID
		view.HasParent = true
	}
	if comment.DeletedAt == nil {
		view.Body = template.HTML(comment.BodyHTML)
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
		s.renderThreadPage(w, r, thread, http.StatusBadRequest, body, err.Error())
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
	s.publishNewComment(thread.ID, comment)

	http.Redirect(w, r, commentAnchorPath(thread.ID, comment.ID), http.StatusSeeOther)
}

func (s *Server) handleReplyPage(w http.ResponseWriter, r *http.Request) {
	comment, ok := s.visibleComment(w, r)
	if !ok {
		return
	}

	s.renderCommentForm(w, r, http.StatusOK, "Reply", commentAction(comment.ID, "reply"), "Post reply", "", "", commentAnchorPath(comment.ThreadID, comment.ID))
}

func (s *Server) handleCreateReply(w http.ResponseWriter, r *http.Request) {
	comment, ok := s.visibleComment(w, r)
	if !ok {
		return
	}

	body := r.FormValue("body")
	bodyHTML, err := renderCommentBody(body)
	if err != nil {
		s.renderCommentForm(w, r, http.StatusBadRequest, "Reply", commentAction(comment.ID, "reply"), "Post reply", body, err.Error(), commentAnchorPath(comment.ThreadID, comment.ID))
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
	s.publishNewComment(comment.ThreadID, reply)

	http.Redirect(w, r, commentAnchorPath(reply.ThreadID, reply.ID), http.StatusSeeOther)
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

	s.renderCommentForm(w, r, http.StatusOK, "Edit comment", commentAction(comment.ID, "edit"), "Save changes", comment.Body, "", commentAnchorPath(comment.ThreadID, comment.ID))
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
		s.renderCommentForm(w, r, http.StatusBadRequest, "Edit comment", commentAction(comment.ID, "edit"), "Save changes", body, err.Error(), commentAnchorPath(comment.ThreadID, comment.ID))
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

	http.Redirect(w, r, commentAnchorPath(comment.ThreadID, comment.ID), http.StatusSeeOther)
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

	if _, err := s.db.DeleteComment(r.Context(), comment.ID, s.now()); err != nil {
		s.serverError(w, err)
		return
	}

	http.Redirect(w, r, commentAnchorPath(comment.ThreadID, comment.ID), http.StatusSeeOther)
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

	s.respondVote(w, r, result, threadPath(thread.ID))
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

	s.respondVote(w, r, result, commentAnchorPath(comment.ThreadID, comment.ID))
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
	}); err != nil {
		s.serverError(w, err)
	}
}

func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}

func (s *Server) renderCommentForm(w http.ResponseWriter, r *http.Request, status int, heading, action, submitLabel, body, message, cancelURL string) {
	data := commentFormPageData{
		PageData:    s.pageData(r, heading),
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
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return nil, false
	}

	comment, err := s.db.CommentByID(r.Context(), id, currentUser(r).ID)
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

	return renderThreadBody(body)
}

func commentAction(id int64, action string) string {
	return "/comments/" + strconv.FormatInt(id, 10) + "/" + action
}

func commentAnchorPath(threadID, commentID int64) string {
	return threadPath(threadID) + "#c" + strconv.FormatInt(commentID, 10)
}
