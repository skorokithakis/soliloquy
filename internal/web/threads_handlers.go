package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/skorokithakis/soliloquy/internal/db"
	"github.com/skorokithakis/soliloquy/internal/markdown"
)

const (
	threadsPerPage       = 30
	maxThreadTitleLength = 200
	maxThreadBodyLength  = 20000
	maxThreadPage        = 1_000_000
)

type threadView struct {
	ID            int64
	Title         string
	URL           string
	Domain        string
	Author        string
	AuthorDeleted bool
	Score         int64
	CommentText   string
	LastActivity  string
	Voted         bool
	CanVote       bool
}

type indexPageData struct {
	PageData
	Threads    []threadView
	Page       int
	TotalPages int
	HasPrev    bool
	HasNext    bool
	PrevPage   int
	NextPage   int
}

type threadPageData struct {
	PageData
	Thread        threadView
	Body          template.HTML
	CreatedAt     string
	Edited        bool
	IsAuthor      bool
	CanDelete     bool
	Voted         bool
	CanVote       bool
	Comments      []commentView
	CommentBody   string
	CommentLength int
	CommentError  string
}

type threadFormPageData struct {
	PageData
	Heading     string
	Action      string
	SubmitLabel string
	CancelURL   string
	FormTitle   string
	FormURL     string
	FormBody    string
	TitleLength int
	BodyLength  int
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	page := threadPageNumber(r)

	total, err := s.db.CountThreads(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}
	totalPages := int((total + threadsPerPage - 1) / threadsPerPage)
	if totalPages < 1 {
		totalPages = 1
	}

	threads, err := s.db.ListThreads(r.Context(), user.ID, threadsPerPage+1, (page-1)*threadsPerPage)
	if err != nil {
		s.serverError(w, err)
		return
	}

	hasNext := len(threads) > threadsPerPage
	if hasNext {
		threads = threads[:threadsPerPage]
	}

	now := s.now()
	views := make([]threadView, len(threads))
	for i, thread := range threads {
		views[i] = threadViewFor(thread, now)
		views[i].Voted = thread.Voted
		views[i].CanVote = thread.AuthorID != user.ID
	}

	s.render(w, "index", indexPageData{
		PageData:   s.pageData(r, "Home"),
		Threads:    views,
		Page:       page,
		TotalPages: totalPages,
		HasPrev:    page > 1,
		HasNext:    hasNext,
		PrevPage:   page - 1,
		NextPage:   page + 1,
	})
}

type previewResponse struct {
	HTML     string   `json:"html"`
	Mentions []string `json:"mentions"`
}

// handlePreview renders Markdown with the same renderer used when saving and
// reports which mentioned users would receive a notification.
func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	body := r.FormValue("body")
	rendered, err := renderThreadBody(body)
	if err != nil {
		s.serverError(w, err)
		return
	}

	user := currentUser(r)
	mentions := []string{}
	for _, name := range markdown.Mentions(body) {
		mentioned, err := s.db.UserByUsername(r.Context(), name)
		if err != nil {
			s.serverError(w, err)
			return
		}
		if mentioned == nil || !mentioned.Active() || mentioned.ID == user.ID {
			continue
		}
		mentions = append(mentions, name)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(previewResponse{HTML: rendered, Mentions: mentions}); err != nil {
		s.serverError(w, err)
	}
}

func (s *Server) handleNewThreadPage(w http.ResponseWriter, r *http.Request) {
	s.renderThreadForm(w, r, http.StatusOK, "threads/new", "/threads", "post thread", threadFormValues{}, "")
}

func (s *Server) handleCreateThread(w http.ResponseWriter, r *http.Request) {
	values, err := readThreadForm(r)
	if err != nil {
		s.renderThreadForm(w, r, http.StatusBadRequest, "threads/new", "/threads", "post thread", values, err.Error())
		return
	}

	bodyHTML, err := renderThreadBody(values.Body)
	if err != nil {
		s.serverError(w, err)
		return
	}

	thread, err := s.db.CreateThread(r.Context(), currentUser(r).ID, values.Title, values.URL, values.Body, bodyHTML, s.now())
	if err != nil {
		s.serverError(w, err)
		return
	}

	recipients, err := s.notifyMentions(r.Context(), thread.AuthorID, thread.ID, nil, thread.Body, "", nil)
	s.logNotificationError(err)
	s.publishBadgeCounts(r.Context(), recipients)

	http.Redirect(w, r, threadPath(thread.ID), http.StatusSeeOther)
}

func (s *Server) handleThreadPage(w http.ResponseWriter, r *http.Request) {
	thread, ok := s.visibleThread(w, r)
	if !ok {
		return
	}

	s.renderThreadPage(w, r, thread, http.StatusOK, "", "")
}

// renderThreadPage renders a thread and its comment tree. It is shared by the
// GET handler and the comment handlers, which re-render the page with an error
// and the submitted text when a comment is invalid.
func (s *Server) renderThreadPage(w http.ResponseWriter, r *http.Request, thread *db.Thread, status int, commentBody, commentError string) {
	user := currentUser(r)
	now := s.now()

	comments, err := s.db.ListComments(r.Context(), thread.ID, user.ID)
	if err != nil {
		s.serverError(w, err)
		return
	}

	voted, err := s.db.ThreadVoted(r.Context(), user.ID, thread.ID)
	if err != nil {
		s.serverError(w, err)
		return
	}

	data := threadPageData{
		PageData:      s.pageData(r, thread.Title),
		Thread:        threadViewFor(*thread, now),
		Body:          template.HTML(thread.BodyHTML),
		CreatedAt:     relativeTime(thread.CreatedAt, now),
		Edited:        thread.EditedAt != nil,
		IsAuthor:      user.ID == thread.AuthorID,
		CanDelete:     user.ID == thread.AuthorID || user.IsAdmin,
		Voted:         voted,
		CanVote:       user.ID != thread.AuthorID,
		Comments:      commentViews(db.BuildCommentTree(comments), user, thread.AuthorID, now),
		CommentBody:   commentBody,
		CommentLength: utf8.RuneCountInString(commentBody),
		CommentError:  commentError,
	}
	data.EventThreadID = thread.ID
	s.renderStatus(w, status, "thread", data)
}

func (s *Server) handleEditThreadPage(w http.ResponseWriter, r *http.Request) {
	thread, ok := s.visibleThread(w, r)
	if !ok {
		return
	}
	if currentUser(r).ID != thread.AuthorID {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}

	s.renderThreadForm(w, r, http.StatusOK, "threads/"+strconv.FormatInt(thread.ID, 10)+"/edit", threadPath(thread.ID)+"/edit", "save changes", threadFormValues{
		Title: thread.Title,
		URL:   thread.URL,
		Body:  thread.Body,
	}, "")
}

func (s *Server) handleUpdateThread(w http.ResponseWriter, r *http.Request) {
	thread, ok := s.visibleThread(w, r)
	if !ok {
		return
	}

	action := threadPath(thread.ID) + "/edit"
	if currentUser(r).ID != thread.AuthorID {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}

	values, err := readThreadForm(r)
	if err != nil {
		s.renderThreadForm(w, r, http.StatusBadRequest, "threads/"+strconv.FormatInt(thread.ID, 10)+"/edit", action, "save changes", values, err.Error())
		return
	}

	bodyHTML, err := renderThreadBody(values.Body)
	if err != nil {
		s.serverError(w, err)
		return
	}

	updated, err := s.db.UpdateThread(r.Context(), thread.ID, thread.AuthorID, values.Title, values.URL, values.Body, bodyHTML, s.now())
	if err != nil {
		s.serverError(w, err)
		return
	}
	if !updated {
		http.NotFound(w, r)
		return
	}

	recipients, err := s.notifyMentions(r.Context(), thread.AuthorID, thread.ID, nil, values.Body, thread.Body, nil)
	s.logNotificationError(err)
	s.publishBadgeCounts(r.Context(), recipients)

	http.Redirect(w, r, threadPath(thread.ID), http.StatusSeeOther)
}

func (s *Server) handleDeleteThread(w http.ResponseWriter, r *http.Request) {
	thread, ok := s.visibleThread(w, r)
	if !ok {
		return
	}

	user := currentUser(r)
	if user.ID != thread.AuthorID && !user.IsAdmin {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}

	if _, err := s.db.DeleteThread(r.Context(), thread.ID, s.now()); err != nil {
		s.serverError(w, err)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// visibleThread loads the thread named in the path and sends 404s for missing,
// invalid and deleted threads. It returns false when it has already written a
// response.
func (s *Server) visibleThread(w http.ResponseWriter, r *http.Request) (*db.Thread, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return nil, false
	}

	thread, err := s.db.ThreadByID(r.Context(), id)
	if err != nil {
		s.serverError(w, err)
		return nil, false
	}
	if thread == nil || thread.DeletedAt != nil {
		http.NotFound(w, r)
		return nil, false
	}
	return thread, true
}

func (s *Server) renderThreadForm(w http.ResponseWriter, r *http.Request, status int, heading, action, submitLabel string, values threadFormValues, message string) {
	cancelURL := "/"
	if strings.HasSuffix(action, "/edit") {
		cancelURL = strings.TrimSuffix(action, "/edit")
	}

	data := threadFormPageData{
		PageData:    s.pageData(r, heading),
		Heading:     heading,
		Action:      action,
		SubmitLabel: submitLabel,
		CancelURL:   cancelURL,
		FormTitle:   values.Title,
		FormURL:     values.URL,
		FormBody:    values.Body,
		TitleLength: utf8.RuneCountInString(values.Title),
		BodyLength:  utf8.RuneCountInString(values.Body),
	}
	data.Error = message
	s.renderStatus(w, status, "thread_form", data)
}

type threadFormValues struct {
	Title string
	URL   string
	Body  string
}

func readThreadForm(r *http.Request) (threadFormValues, error) {
	values := threadFormValues{
		Title: strings.TrimSpace(r.FormValue("title")),
		URL:   strings.TrimSpace(r.FormValue("url")),
		// Keep the body as written: leading whitespace can be meaningful in
		// Markdown (for example an indented code block).
		Body: r.FormValue("body"),
	}

	if values.Title == "" {
		return values, errors.New("Title is required.")
	}
	if utf8.RuneCountInString(values.Title) > maxThreadTitleLength {
		return values, fmt.Errorf("Title must be %d characters or fewer.", maxThreadTitleLength)
	}

	normalizedURL, err := validateThreadURL(values.URL)
	if err != nil {
		return values, err
	}
	values.URL = normalizedURL

	if utf8.RuneCountInString(values.Body) > maxThreadBodyLength {
		return values, fmt.Errorf("Body must be %d characters or fewer.", maxThreadBodyLength)
	}
	if values.URL == "" && strings.TrimSpace(values.Body) == "" {
		return values, errors.New("Add a URL, a body, or both.")
	}

	return values, nil
}

// validateThreadURL accepts an empty URL or an absolute http(s) URL. In
// particular it rejects javascript:, data: and other schemes that would be
// dangerous even though html/template also escapes them.
func validateThreadURL(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("Enter a valid URL.")
	}

	scheme := strings.ToLower(parsed.Scheme)
	if (scheme != "http" && scheme != "https") || parsed.Hostname() == "" {
		return "", errors.New("URL must start with http:// or https://.")
	}

	return raw, nil
}

func renderThreadBody(body string) (string, error) {
	if body == "" {
		return "", nil
	}

	rendered, err := markdown.Render(body)
	if err != nil {
		return "", err
	}
	return string(rendered), nil
}

func threadViewFor(thread db.Thread, now time.Time) threadView {
	domain := ""
	if thread.URL != "" {
		if parsed, err := url.Parse(thread.URL); err == nil {
			domain = parsed.Hostname()
		}
	}

	return threadView{
		ID:            thread.ID,
		Title:         thread.Title,
		URL:           thread.URL,
		Domain:        domain,
		Author:        thread.AuthorName(),
		AuthorDeleted: thread.AuthorDeletedAt != nil,
		Score:         thread.Score,
		CommentText:   commentsText(thread.CommentCount),
		LastActivity:  relativeTime(thread.LastActivityAt, now),
	}
}

func threadPath(id int64) string {
	return "/threads/" + strconv.FormatInt(id, 10)
}

func threadPageNumber(r *http.Request) int {
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		return 1
	}
	if page > maxThreadPage {
		return maxThreadPage
	}
	return page
}

func pointsText(points int64) string {
	if points == 1 {
		return "1 point"
	}
	return fmt.Sprintf("%d points", points)
}

func commentsText(comments int64) string {
	if comments == 1 {
		return "1 comment"
	}
	return fmt.Sprintf("%d comments", comments)
}

func relativeTime(t, now time.Time) string {
	elapsed := now.Sub(t)
	if elapsed < time.Minute {
		return "now"
	}

	switch {
	case elapsed < time.Hour:
		return fmt.Sprintf("%dm", int(elapsed/time.Minute))
	case elapsed < 24*time.Hour:
		return fmt.Sprintf("%dh", int(elapsed/time.Hour))
	case elapsed < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(elapsed/(24*time.Hour)))
	case elapsed < 365*24*time.Hour:
		return fmt.Sprintf("%dmo", int(elapsed/(30*24*time.Hour)))
	default:
		return fmt.Sprintf("%dy", int(elapsed/(365*24*time.Hour)))
	}
}
