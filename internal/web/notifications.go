package web

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/skorokithakis/soliloquy/internal/db"
	"github.com/skorokithakis/soliloquy/internal/markdown"
)

type notificationView struct {
	Kind        string
	Actor       string
	ThreadTitle string
	CommentID   *int64
	ThreadID    int64
	CreatedAt   string
	Unread      bool
}

func (n notificationView) Link() string {
	if n.CommentID != nil {
		return commentAnchorPath(n.ThreadID, *n.CommentID)
	}
	return threadPath(n.ThreadID)
}

type notificationsPageData struct {
	PageData
	Notifications []notificationView
}

func (s *Server) handleNotificationsPage(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)

	notifications, err := s.db.ListNotifications(r.Context(), user.ID)
	if err != nil {
		s.serverError(w, err)
		return
	}

	now := s.now()
	views := make([]notificationView, len(notifications))
	for i, notification := range notifications {
		views[i] = notificationView{
			Kind:        notification.Kind,
			Actor:       notification.ActorName(),
			ThreadTitle: notification.ThreadTitle,
			CommentID:   notification.CommentID,
			ThreadID:    notification.ThreadID,
			CreatedAt:   relativeTime(notification.CreatedAt, now),
			Unread:      notification.ReadAt == nil,
		}
	}

	if err := s.db.MarkNotificationsRead(r.Context(), user.ID, now); err != nil {
		s.serverError(w, err)
		return
	}
	s.publishBadgeCounts(r.Context(), []int64{user.ID})

	s.render(w, "notifications", notificationsPageData{
		PageData:      s.pageData(r, "Notifications"),
		Notifications: views,
	})
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

		if _, err := s.db.CreateNotification(ctx, recipientID, comment.AuthorID, kind, threadID, &comment.ID, now); err != nil {
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

		if _, err := s.db.CreateNotification(ctx, user.ID, actorID, db.NotificationMention, threadID, commentID, now); err != nil {
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
