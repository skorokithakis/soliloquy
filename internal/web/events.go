package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/skorokithakis/soliloquy/internal/db"
	"github.com/skorokithakis/soliloquy/internal/hub"
)

// sseKeepAlive is how often an idle stream sends a comment to keep proxies
// from closing the connection.
const sseKeepAlive = 15 * time.Second

type badgeEvent struct {
	Count int64 `json:"count"`
}

type commentEvent struct {
	CommentID int64  `json:"comment_id"`
	ThreadID  int64  `json:"thread_id"`
	ParentID  *int64 `json:"parent_id"`
	HTML      string `json:"html"`
}

// handleEvents is the single SSE stream used by a page. It always carries the
// user's badge updates and, when thread_id is given, that thread's new
// comments.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user == nil {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	threadID, hasThread := eventThreadID(r)
	if hasThread {
		thread, err := s.db.ThreadByID(r.Context(), threadID)
		if err != nil {
			s.serverError(w, err)
			return
		}
		if thread == nil || thread.DeletedAt != nil {
			http.NotFound(w, r)
			return
		}
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	badges, unsubscribeBadges := s.hub.Subscribe(hub.UserTopic(user.ID))
	defer unsubscribeBadges()

	var comments <-chan hub.Event
	if hasThread {
		stream, unsubscribe := s.hub.Subscribe(hub.ThreadTopic(threadID))
		defer unsubscribe()
		comments = stream
	}

	// Send the current count immediately so a page that opens after some
	// events were missed, or reconnects, starts in sync.
	if count, err := s.db.CountUnreadNotifications(r.Context(), user.ID); err == nil {
		if err := writeSSE(w, flusher, "badge", mustJSON(badgeEvent{Count: count})); err != nil {
			return
		}
	} else {
		slog.Error("count unread notifications", "error", err)
	}

	ticker := time.NewTicker(sseKeepAlive)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-badges:
			if !ok {
				return
			}
			if !s.eventStreamValid(r, threadID, hasThread) {
				return
			}
			if err := writeSSE(w, flusher, event.Name, event.Data); err != nil {
				return
			}
		case event, ok := <-comments:
			if !ok {
				return
			}
			if !s.eventStreamValid(r, threadID, hasThread) {
				return
			}
			if err := writeSSE(w, flusher, event.Name, event.Data); err != nil {
				return
			}
		case <-ticker.C:
			if !s.eventStreamValid(r, threadID, hasThread) {
				return
			}
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// eventStreamValid reports whether the session behind an open stream is still
// valid and, for thread streams, the thread is still live. Both were checked
// when the stream opened, but an SSE connection outlives the request that
// created it, so logout, disabling and deletion must be rechecked periodically.
func (s *Server) eventStreamValid(r *http.Request, threadID int64, hasThread bool) bool {
	if _, user, err := s.db.SessionUser(r.Context(), currentSessionHash(r), s.now()); err != nil {
		slog.Error("revalidate event stream session", "error", err)
		return false
	} else if user == nil {
		return false
	}

	if !hasThread {
		return true
	}

	thread, err := s.db.ThreadByID(r.Context(), threadID)
	if err != nil {
		slog.Error("revalidate event stream thread", "thread_id", threadID, "error", err)
		return false
	}
	return thread != nil && thread.DeletedAt == nil
}

func eventThreadID(r *http.Request) (int64, bool) {
	raw := r.URL.Query().Get("thread_id")
	if raw == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, name string, data []byte) error {
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func mustJSON(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		// The event payloads are plain structs of strings and integers, so an
		// encoding failure is a programming error.
		panic(err)
	}
	return data
}

// publishBadgeCounts pushes each recipient's current unread count. Duplicate
// recipients are collapsed so one action produces one badge event per user.
func (s *Server) publishBadgeCounts(ctx context.Context, recipientIDs []int64) {
	seen := make(map[int64]bool, len(recipientIDs))
	for _, recipientID := range recipientIDs {
		if seen[recipientID] {
			continue
		}
		seen[recipientID] = true

		count, err := s.db.CountUnreadNotifications(ctx, recipientID)
		if err != nil {
			slog.Error("count unread notifications", "user_id", recipientID, "error", err)
			continue
		}
		s.hub.Publish(hub.UserTopic(recipientID), hub.Event{
			Name: "badge",
			Data: mustJSON(badgeEvent{Count: count}),
		})
	}
}

// publishNewComment sends a new comment to everyone watching the thread. The
// HTML is rendered without viewer-specific controls because it is shown to
// other users, whose permissions and votes differ.
func (s *Server) publishNewComment(threadID int64, comment *db.Comment) {
	thread, err := s.db.ThreadByID(context.Background(), threadID)
	if err != nil {
		slog.Error("load thread for live comment", "thread_id", threadID, "error", err)
		return
	}
	threadAuthorID := int64(0)
	if thread != nil {
		threadAuthorID = thread.AuthorID
	}

	view := commentViewFor(&db.CommentNode{Comment: *comment}, nil, threadAuthorID, s.now())
	// Only logged-in users have a live stream, so they also get the reply link.
	view.LoggedIn = true
	html, err := s.renderCommentHTML(view)
	if err != nil {
		slog.Error("render live comment", "comment_id", comment.ID, "error", err)
		return
	}

	s.hub.Publish(hub.ThreadTopic(threadID), hub.Event{
		Name: "comment",
		Data: mustJSON(commentEvent{
			CommentID: comment.ID,
			ThreadID:  threadID,
			ParentID:  comment.ParentID,
			HTML:      string(html),
		}),
	})
}
