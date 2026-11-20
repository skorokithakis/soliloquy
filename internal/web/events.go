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

// notificationEvent is a personal notification for an open page. URL is the
// same site-relative link the notifications page uses, and there is no comment
// body: desktop alerts show Text only.
type notificationEvent struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

type commentEvent struct {
	CommentID        string  `json:"comment_id"`
	ThreadID         string  `json:"thread_id"`
	ParentID         *string `json:"parent_id"`
	HTML             string  `json:"html"`
	CommentCountText string  `json:"comment_count_text"`
}

// commentEditEvent replaces a comment's body. The markup comes from the
// comment template, so the page and the stream cannot drift apart.
type commentEditEvent struct {
	CommentID string `json:"comment_id"`
	HTML      string `json:"html"`
}

// commentDeleteEvent replaces a comment's header and body. It carries the
// thread's count because deleting a comment changes it.
type commentDeleteEvent struct {
	CommentID        string `json:"comment_id"`
	HTML             string `json:"html"`
	CommentCountText string `json:"comment_count_text"`
}

// threadUpdateEvent replaces the thread title and body. An empty BodyHTML
// means the thread has no body, and the page must remove the body block.
type threadUpdateEvent struct {
	TitleHTML string `json:"title_html"`
	BodyHTML  string `json:"body_html"`
}

// voteEvent updates one score. CommentID is null for the thread, and the event
// deliberately does not say who voted, so each page keeps its own highlight.
// A comment vote also carries the comment's reaction counts, because the
// viewer's arrow can remove their reaction; the event still does not say whose
// reactions they are.
type voteEvent struct {
	CommentID *string                 `json:"comment_id"`
	Score     int64                   `json:"score"`
	ScoreText string                  `json:"score_text"`
	Reactions []reactionCountResponse `json:"reactions,omitempty"`
}

// handleEvents is the single SSE stream used by a page. It always carries the
// user's badge updates and, when thread_id is given, that thread's live
// updates, with a replay of anything missed since the page's render time.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user == nil {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	threadPublicID, hasThread := eventThreadID(r)
	threadID := int64(0)
	if hasThread {
		thread, err := s.db.ThreadByPublicID(r.Context(), threadPublicID)
		if err != nil {
			s.serverError(w, err)
			return
		}
		if thread == nil || thread.DeletedAt != nil {
			http.NotFound(w, r)
			return
		}
		threadID = thread.ID
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

	s.metrics.sseConnections.Inc()
	defer s.metrics.sseConnections.Dec()

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

	// A thread page passes its render time so it catches up on changes it
	// missed while the stream was not connected. The read happens after
	// subscribing, so nothing can fall between the replay and the live events.
	if hasThread {
		if since, ok := eventSince(r); ok {
			thread, err := s.db.ThreadByID(r.Context(), threadID)
			if err != nil {
				slog.Error("load thread for event replay", "thread_id", threadID, "error", err)
			} else if thread != nil {
				for _, event := range s.replayThreadEvents(r.Context(), thread, user, since) {
					if err := writeSSE(w, flusher, event.Name, event.Data); err != nil {
						return
					}
				}
			}
		}
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

func eventThreadID(r *http.Request) (string, bool) {
	publicID := r.URL.Query().Get("thread_id")
	if publicID == "" || !validPublicID(publicID) {
		return "", false
	}
	return publicID, true
}

// eventSince returns the page render time passed by a thread page. A missing
// or invalid value means the page asked for no replay.
func eventSince(r *http.Request) (time.Time, bool) {
	seconds, err := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	if err != nil || seconds <= 0 {
		return time.Time{}, false
	}
	return time.Unix(seconds, 0), true
}

// replayThreadEvents returns the thread events a page missed since its render
// time, one comment at a time and oldest first so a parent comes before its
// replies. A comment created since is sent in its current state, and a comment
// deleted or edited since is also sent, even when it was created since: the
// page may already have the comment from when it arrived live, and the second
// event repairs a page that was disconnected for the edit or delete. Applying
// both is safe because the edit and delete handlers are idempotent. Votes are
// not replayed; scores are right on the next load.
func (s *Server) replayThreadEvents(ctx context.Context, thread *db.Thread, viewer *db.User, since time.Time) []hub.Event {
	comments, err := s.db.ListComments(ctx, thread.ID, viewer.ID)
	if err != nil {
		slog.Error("load comments for event replay", "thread_id", thread.ID, "error", err)
		return nil
	}

	events := make([]hub.Event, 0, len(comments)+1)
	for i := range comments {
		comment := &comments[i]

		if !comment.CreatedAt.Before(since) {
			payload, err := s.commentPayload(thread, comment, viewer)
			if err != nil {
				slog.Error("build replay comment", "comment_id", comment.ID, "error", err)
			} else {
				events = append(events, hub.Event{Name: "comment", Data: mustJSON(payload)})
			}
		}

		// A deletion is the later state when a comment was also edited, so it
		// replaces the edit in the replay.
		if comment.DeletedAt != nil && !comment.DeletedAt.Before(since) {
			payload, err := s.commentDeletePayload(thread, comment)
			if err != nil {
				slog.Error("build replay comment delete", "comment_id", comment.ID, "error", err)
			} else {
				events = append(events, hub.Event{Name: "comment_delete", Data: mustJSON(payload)})
			}
		} else if comment.EditedAt != nil && !comment.EditedAt.Before(since) {
			events = append(events, hub.Event{Name: "comment_edit", Data: mustJSON(commentEditPayload(comment))})
		}
	}

	if thread.EditedAt != nil && !thread.EditedAt.Before(since) {
		payload, err := s.threadEditPayload(thread)
		if err != nil {
			slog.Error("build replay thread edit", "thread_id", thread.ID, "error", err)
		} else {
			events = append(events, hub.Event{Name: "thread_edit", Data: mustJSON(payload)})
		}
	}

	return events
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

// publishNotification sends a new personal notification to the recipient's
// open pages. Desktop alerts use it; the badge still updates separately.
func (s *Server) publishNotification(notification *db.Notification) {
	s.hub.Publish(hub.UserTopic(notification.RecipientID), hub.Event{
		Name: "notification",
		Data: mustJSON(notificationEvent{
			Text: notificationText(notification),
			URL:  notificationPath(notification.ThreadPublicID, notification.CommentPublicID),
		}),
	})
}

// publishCommentEdit sends a comment's new body to everyone watching the
// thread. The body is sent on its own because the rest of the comment header
// holds viewer-specific controls, such as the vote and edit links.
func (s *Server) publishCommentEdit(comment *db.Comment, bodyHTML string) {
	updated := *comment
	updated.BodyHTML = bodyHTML
	s.hub.Publish(hub.ThreadTopic(comment.ThreadID), hub.Event{
		Name: "comment_edit",
		Data: mustJSON(commentEditPayload(&updated)),
	})
}

// commentEditPayload builds the body replacement from a comment's stored HTML,
// so live and replayed events use the current body.
func commentEditPayload(comment *db.Comment) commentEditEvent {
	return commentEditEvent{CommentID: comment.PublicID, HTML: comment.BodyHTML}
}

// publishCommentDelete sends a comment rendered in its deleted state to
// everyone watching the thread.
func (s *Server) publishCommentDelete(ctx context.Context, comment *db.Comment) {
	// The write is committed, so the event must still go out if the client
	// aborts the request. Replay runs in the request that opened the stream.
	thread, err := s.db.ThreadByID(context.WithoutCancel(ctx), comment.ThreadID)
	if err != nil {
		slog.Error("load thread for live comment delete", "thread_id", comment.ThreadID, "error", err)
		return
	}
	if thread == nil {
		slog.Error("load thread for live comment delete", "thread_id", comment.ThreadID, "error", "thread not found")
		return
	}

	payload, err := s.commentDeletePayload(thread, comment)
	if err != nil {
		slog.Error("build live comment delete", "comment_id", comment.ID, "error", err)
		return
	}

	s.hub.Publish(hub.ThreadTopic(comment.ThreadID), hub.Event{
		Name: "comment_delete",
		Data: mustJSON(payload),
	})
}

// commentDeletePayload builds the deleted rendering with the thread's current
// count. Each viewer's page keeps the replies it already shows and the
// comment's collapsed state, so the markup has no replies.
func (s *Server) commentDeletePayload(thread *db.Thread, comment *db.Comment) (commentDeleteEvent, error) {
	now := s.now()
	deleted := *comment
	// A live delete has no deletion time yet; a replayed row already has one.
	if deleted.DeletedAt == nil {
		deleted.DeletedAt = &now
	}

	// Build the view the same way a page load does, so a deleted reply still
	// shows its parent link.
	view := commentViewFor(&db.CommentNode{Comment: deleted}, nil, thread.AuthorID, now, time.Time{})
	// Only signed-in users have a live stream. The deleted header shows a
	// disabled vote button for them rather than a stray span.
	view.LoggedIn = true

	html, err := s.renderCommentHTML(view)
	if err != nil {
		return commentDeleteEvent{}, err
	}

	return commentDeleteEvent{
		CommentID:        comment.PublicID,
		HTML:             string(html),
		CommentCountText: commentsText(thread.CommentCount),
	}, nil
}

// publishThreadEdit sends the new title and body to everyone watching the
// thread.
func (s *Server) publishThreadEdit(thread *db.Thread, title, rawURL, bodyHTML string) {
	updated := *thread
	updated.Title = title
	updated.URL = rawURL
	updated.BodyHTML = bodyHTML

	payload, err := s.threadEditPayload(&updated)
	if err != nil {
		slog.Error("build live thread edit", "thread_id", thread.ID, "error", err)
		return
	}

	s.hub.Publish(hub.ThreadTopic(thread.ID), hub.Event{
		Name: "thread_edit",
		Data: mustJSON(payload),
	})
}

// threadEditPayload builds the title and body replacement from a thread's
// current values. The title markup comes from the same template the page uses.
func (s *Server) threadEditPayload(thread *db.Thread) (threadUpdateEvent, error) {
	titleHTML, err := s.renderThreadTitleHTML(threadViewFor(*thread, s.now()))
	if err != nil {
		return threadUpdateEvent{}, err
	}

	return threadUpdateEvent{TitleHTML: string(titleHTML), BodyHTML: thread.BodyHTML}, nil
}

// publishVote sends a new score to everyone watching the thread.
func (s *Server) publishVote(threadID int64, payload voteEvent) {
	s.hub.Publish(hub.ThreadTopic(threadID), hub.Event{
		Name: "vote",
		Data: mustJSON(payload),
	})
}

// votePayload builds the score update after a vote or reaction change.
// score_text is the points text comments show; the thread page uses the plain
// score. Thread results have no reaction counts, so the field is omitted.
func votePayload(commentID *string, result db.VoteResult) voteEvent {
	return voteEvent{
		CommentID: commentID,
		Score:     result.Score,
		ScoreText: pointsText(result.Score),
		Reactions: reactionCountResponses(result.ReactionCounts),
	}
}

// publishNewComment sends a new comment to everyone watching the thread.
func (s *Server) publishNewComment(ctx context.Context, threadID int64, comment *db.Comment) {
	// The write is committed, so the event must still go out if the client
	// aborts the request. Replay runs in the request that opened the stream.
	thread, err := s.db.ThreadByID(context.WithoutCancel(ctx), threadID)
	if err != nil {
		slog.Error("load thread for live comment", "thread_id", threadID, "error", err)
		return
	}
	if thread == nil {
		slog.Error("load thread for live comment", "thread_id", threadID, "error", "thread not found")
		return
	}

	payload, err := s.commentPayload(thread, comment, nil)
	if err != nil {
		slog.Error("build live comment", "comment_id", comment.ID, "error", err)
		return
	}

	s.hub.Publish(hub.ThreadTopic(threadID), hub.Event{
		Name: "comment",
		Data: mustJSON(payload),
	})
}

// commentPayload renders a comment for a stream. A nil viewer is the live
// broadcast, where one rendering goes to everyone; a real viewer is a replay,
// where that viewer's vote and controls must match their page.
func (s *Server) commentPayload(thread *db.Thread, comment *db.Comment, viewer *db.User) (commentEvent, error) {
	// The stream marks no comment as new; app.js adds the marker on the
	// viewer's page instead.
	view := commentViewFor(&db.CommentNode{Comment: *comment}, viewer, thread.AuthorID, s.now(), time.Time{})
	// Only logged-in users have a live stream, so they also get the reply link.
	view.LoggedIn = true
	if viewer == nil {
		// A new comment has only its author's vote, so the vote and reaction
		// controls are the same for every viewer. The author's other open tabs
		// get them too: their vote is the one row, which they can remove, and
		// the server rejects their reaction. The arrow may show as unvoted in
		// those tabs until a reload, which is a known accepted limit.
		view.CanVote = true
		view.CanReact = true
	}

	html, err := s.renderCommentHTML(view)
	if err != nil {
		return commentEvent{}, err
	}

	return commentEvent{
		CommentID:        comment.PublicID,
		ThreadID:         comment.ThreadPublicID,
		ParentID:         comment.ParentPublicID,
		HTML:             string(html),
		CommentCountText: commentsText(thread.CommentCount),
	}, nil
}
