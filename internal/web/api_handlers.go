package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/skorokithakis/soliloquy/internal/db"
)

// The JSON API is a read-only view of the pages. It shares the feed access
// rule (see feedPath and allowFeed): a session, a feed token or a public forum
// grants access, and anything else is a 404 rather than a login redirect.

// apiThread is the thread summary the thread list returns.
type apiThread struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	URL            string `json:"url"`
	Link           string `json:"link"`
	Author         string `json:"author"`
	Score          int64  `json:"score"`
	CommentCount   int64  `json:"comment_count"`
	CreatedAt      string `json:"created_at"`
	LastActivityAt string `json:"last_activity_at"`
}

// apiThreadDetail is a thread with the fields only its own page has.
type apiThreadDetail struct {
	apiThread
	Body     string  `json:"body"`
	BodyHTML string  `json:"body_html"`
	EditedAt *string `json:"edited_at"`
}

// apiComment is one comment in the tree BuildCommentTree returns.
type apiComment struct {
	ID        string        `json:"id"`
	Author    string        `json:"author"`
	Score     int64         `json:"score"`
	CreatedAt string        `json:"created_at"`
	EditedAt  *string       `json:"edited_at"`
	Body      string        `json:"body"`
	BodyHTML  string        `json:"body_html"`
	Deleted   bool          `json:"deleted"`
	Replies   []*apiComment `json:"replies"`
}

type apiThreadsResponse struct {
	Threads []apiThread `json:"threads"`
	Page    int         `json:"page"`
	HasNext bool        `json:"has_next"`
}

type apiThreadResponse struct {
	Thread   apiThreadDetail `json:"thread"`
	Comments []*apiComment   `json:"comments"`
}

type apiErrorResponse struct {
	Error string `json:"error"`
}

func (s *Server) handleAPIThreads(w http.ResponseWriter, r *http.Request) {
	page := threadPageNumber(r)

	// viewer 0 keeps the query from computing a "voted" value, which the API
	// does not expose.
	threads, err := s.db.ListThreads(r.Context(), 0, threadsPerPage+1, (page-1)*threadsPerPage)
	if err != nil {
		s.apiServerError(w, err)
		return
	}

	hasNext := len(threads) > threadsPerPage
	if hasNext {
		threads = threads[:threadsPerPage]
	}

	response := apiThreadsResponse{
		Threads: make([]apiThread, len(threads)),
		Page:    page,
		HasNext: hasNext,
	}
	for i, thread := range threads {
		response.Threads[i] = apiThreadFor(thread, s.absoluteURL(threadPath(thread.PublicID)))
	}

	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleAPIThread(w http.ResponseWriter, r *http.Request) {
	publicID := r.PathValue("id")
	if !validPublicID(publicID) {
		writeJSONError(w, http.StatusNotFound, "thread not found")
		return
	}

	thread, err := s.db.ThreadByPublicID(r.Context(), publicID)
	if err != nil {
		s.apiServerError(w, err)
		return
	}
	if thread == nil || thread.DeletedAt != nil {
		writeJSONError(w, http.StatusNotFound, "thread not found")
		return
	}

	comments, err := s.db.ListComments(r.Context(), thread.ID, 0)
	if err != nil {
		s.apiServerError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, apiThreadResponse{
		Thread:   apiThreadDetailFor(*thread, s.absoluteURL(threadPath(thread.PublicID))),
		Comments: apiCommentsFor(db.BuildCommentTree(comments)),
	})
}

func apiThreadFor(thread db.Thread, link string) apiThread {
	return apiThread{
		ID:             thread.PublicID,
		Title:          thread.Title,
		URL:            thread.URL,
		Link:           link,
		Author:         thread.AuthorName(),
		Score:          thread.Score,
		CommentCount:   thread.CommentCount,
		CreatedAt:      apiTimestamp(thread.CreatedAt),
		LastActivityAt: apiTimestamp(thread.LastActivityAt),
	}
}

func apiThreadDetailFor(thread db.Thread, link string) apiThreadDetail {
	return apiThreadDetail{
		apiThread: apiThreadFor(thread, link),
		Body:      thread.Body,
		BodyHTML:  thread.BodyHTML,
		EditedAt:  apiOptionalTimestamp(thread.EditedAt),
	}
}

func apiCommentsFor(nodes []*db.CommentNode) []*apiComment {
	comments := make([]*apiComment, len(nodes))
	for i, node := range nodes {
		comments[i] = apiCommentFor(node)
	}
	return comments
}

func apiCommentFor(node *db.CommentNode) *apiComment {
	comment := node.Comment
	view := &apiComment{
		ID:        comment.PublicID,
		Score:     comment.Score,
		CreatedAt: apiTimestamp(comment.CreatedAt),
		EditedAt:  apiOptionalTimestamp(comment.EditedAt),
		Deleted:   comment.DeletedAt != nil,
		Replies:   apiCommentsFor(node.Replies),
	}
	// A deleted comment is a placeholder that keeps its replies attached, so
	// it must not leak the author or the body.
	if comment.DeletedAt == nil {
		view.Author = comment.AuthorName()
		view.Body = comment.Body
		view.BodyHTML = comment.BodyHTML
	}
	return view
}

func apiTimestamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// apiOptionalTimestamp formats an optional time so it encodes as null when the
// thread or comment was never edited.
func apiOptionalTimestamp(t *time.Time) *string {
	if t == nil {
		return nil
	}
	formatted := apiTimestamp(*t)
	return &formatted
}

// apiPath reports whether a request is for the JSON API. The API answers a
// refused request with JSON instead of the login redirect.
func apiPath(r *http.Request) bool {
	return r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/")
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("write JSON", "error", err)
	}
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, apiErrorResponse{Error: message})
}

// apiServerError logs an unexpected failure the way serverError does, but
// answers in JSON.
func (s *Server) apiServerError(w http.ResponseWriter, err error) {
	slog.Error("request failed", "error", err)
	writeJSONError(w, http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
}
