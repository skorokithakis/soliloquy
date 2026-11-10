package web

import (
	"context"
	"html"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/skorokithakis/soliloquy/internal/db"
	"github.com/skorokithakis/soliloquy/internal/markdown"
)

const notificationsPerPage = 30

type notificationView struct {
	Kind            string
	KindLabel       string
	Actor           string
	ActorDeleted    bool
	ThreadTitle     string
	Snippet         string
	CommentPublicID *string
	ThreadPublicID  string
	CreatedAt       string
	Unread          bool
}

func (n notificationView) Link() string {
	return notificationPath(n.ThreadPublicID, n.CommentPublicID)
}

// notificationPath is the site-relative link for a notification: the anchor
// of the comment it is about, or the thread for a mention in the thread body.
func notificationPath(threadPublicID string, commentPublicID *string) string {
	if commentPublicID != nil {
		return commentAnchorPath(threadPublicID, *commentPublicID)
	}
	return threadPath(threadPublicID)
}

// notificationText is the one-line message for a personal notification. It is
// shared by browser desktop alerts and push notifications, so it never
// includes the comment body.
func notificationText(notification *db.Notification) string {
	actor := notification.ActorName()
	switch notification.Kind {
	case db.NotificationReply:
		return actor + " replied to you in " + notification.ThreadTitle
	case db.NotificationThreadComment:
		return actor + " commented on your thread " + notification.ThreadTitle
	default:
		return actor + " mentioned you in " + notification.ThreadTitle
	}
}

type notificationsPageData struct {
	PageData
	Unread      []notificationView
	Earlier     []notificationView
	UnreadCount int64
	Page        int
	HasNext     bool
	NextPage    int
}

func (s *Server) handleNotificationsPage(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	page := threadPageNumber(r)

	notifications, err := s.db.ListNotifications(r.Context(), user.ID, notificationsPerPage+1, (page-1)*notificationsPerPage)
	if err != nil {
		s.serverError(w, err)
		return
	}

	hasNext := len(notifications) > notificationsPerPage
	if hasNext {
		notifications = notifications[:notificationsPerPage]
	}

	now := s.now()
	unread := []notificationView{}
	earlier := []notificationView{}
	for _, notification := range notifications {
		view := notificationView{
			Kind:            notification.Kind,
			KindLabel:       notificationKindLabel(notification.Kind),
			Actor:           notification.ActorName(),
			ActorDeleted:    notification.ActorDeletedAt != nil,
			ThreadTitle:     notification.ThreadTitle,
			Snippet:         notificationSnippet(notification.BodyHTML, 140),
			CommentPublicID: notification.CommentPublicID,
			ThreadPublicID:  notification.ThreadPublicID,
			CreatedAt:       relativeTime(notification.CreatedAt, now),
			Unread:          notification.ReadAt == nil,
		}
		if view.Unread {
			unread = append(unread, view)
		} else {
			earlier = append(earlier, view)
		}
	}

	if err := s.db.MarkNotificationsRead(r.Context(), user.ID, now); err != nil {
		s.serverError(w, err)
		return
	}
	s.publishBadgeCounts(r.Context(), []int64{user.ID})

	s.render(w, "notifications", notificationsPageData{
		PageData:    s.pageData(r, "Notifications"),
		Unread:      unread,
		Earlier:     earlier,
		UnreadCount: int64(len(unread)),
		Page:        page,
		HasNext:     hasNext,
		NextPage:    page + 1,
	})
}

func notificationKindLabel(kind string) string {
	switch kind {
	case db.NotificationMention:
		return "mention"
	case db.NotificationReply:
		return "reply"
	default:
		return "comment"
	}
}

var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

// notificationSnippet takes the text content of rendered, sanitized HTML and
// returns a short one-line preview. Tags are removed before entities are
// unescaped, so user text that looks like HTML stays visible as text.
func notificationSnippet(bodyHTML string, limit int) string {
	text := htmlTagPattern.ReplaceAllString(bodyHTML, " ")
	text = html.UnescapeString(text)
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return ""
	}

	runes := []rune(text)
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return text
}

// createNotification creates a notification and pushes it to the recipient's
// open pages. Every notification goes through here so a new one always
// reaches the recipient's live stream, not just the unread badge.
func (s *Server) createNotification(ctx context.Context, recipientID, actorID int64, kind string, threadID int64, commentID *int64, now time.Time) (*db.Notification, error) {
	notification, err := s.db.CreateNotification(ctx, recipientID, actorID, kind, threadID, commentID, now)
	if err != nil {
		return nil, err
	}
	s.publishNotification(notification)
	// Pushing must never delay or fail the request that caused the
	// notification, and the request context may already be gone by the time
	// the push completes.
	s.goBackground(func() { s.pushNotification(notification) })
	return notification, nil
}

// notifyNewComment creates the notifications caused by a new comment: a reply
// to its parent's author, a top-level comment on the thread's author, and any
// mentions in its body. It returns the recipients whose badge counts changed.
func (s *Server) notifyNewComment(ctx context.Context, threadID, threadAuthorID int64, comment *db.Comment, parent *db.Comment) ([]int64, error) {
	now := s.now()
	var recipients []int64
	notified := make(map[int64]struct{})

	notify := func(recipientID int64, kind string) error {
		if recipientID == 0 || recipientID == comment.AuthorID {
			return nil
		}

		user, err := s.db.UserByID(ctx, recipientID)
		if err != nil {
			return err
		}
		if !user.Active() {
			return nil
		}

		if _, err := s.createNotification(ctx, recipientID, comment.AuthorID, kind, threadID, &comment.ID, now); err != nil {
			return err
		}
		recipients = append(recipients, recipientID)
		notified[recipientID] = struct{}{}
		return nil
	}

	var err error
	if parent == nil {
		err = notify(threadAuthorID, db.NotificationThreadComment)
	} else {
		err = notify(parent.AuthorID, db.NotificationReply)
	}
	if err != nil {
		return recipients, err
	}

	mentioned, err := s.notifyMentions(ctx, comment.AuthorID, threadID, &comment.ID, comment.Body, "", notified)
	recipients = append(recipients, mentioned...)
	return recipients, err
}

// notifyMentions creates mention notifications for names in newSource that
// were not in oldSource. Passing an empty oldSource means every mention is new.
// Only active users other than the actor are notified, and users in skip
// already have a notification for this post.
func (s *Server) notifyMentions(ctx context.Context, actorID, threadID int64, commentID *int64, newSource, oldSource string, skip map[int64]struct{}) ([]int64, error) {
	oldNames := make(map[string]struct{})
	if oldSource != "" {
		for _, name := range markdown.Mentions(oldSource) {
			oldNames[name] = struct{}{}
		}
	}

	now := s.now()
	var recipients []int64
	for _, name := range markdown.Mentions(newSource) {
		if _, seen := oldNames[name]; seen {
			continue
		}

		user, err := s.db.UserByUsername(ctx, name)
		if err != nil {
			return recipients, err
		}
		if !user.Active() || user.ID == actorID {
			continue
		}
		if _, seen := skip[user.ID]; seen {
			continue
		}

		if _, err := s.createNotification(ctx, user.ID, actorID, db.NotificationMention, threadID, commentID, now); err != nil {
			return recipients, err
		}
		recipients = append(recipients, user.ID)
	}
	return recipients, nil
}

func (s *Server) logNotificationError(err error) {
	if err != nil {
		slog.Error("create notifications", "error", err)
	}
}
