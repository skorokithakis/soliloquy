package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Notification kinds. Replies and top-level comments are created from new
// comments; mentions are also created when an edit adds a name.
const (
	NotificationReply         = "reply"
	NotificationThreadComment = "thread_comment"
	NotificationMention       = "mention"
)

// Notification is an in-app notification for a user. CommentID is nil for a
// mention in a thread body; ThreadID is always set.
type Notification struct {
	ID             int64
	RecipientID    int64
	Kind           string
	ActorID        int64
	ActorUsername  string
	ActorDeletedAt *time.Time
	ThreadID       int64
	ThreadTitle    string
	CommentID      *int64
	CreatedAt      time.Time
	ReadAt         *time.Time
}

// ActorName is the name shown for the user who caused the notification.
func (n *Notification) ActorName() string {
	if n.ActorDeletedAt != nil {
		return "[deleted user]"
	}
	return n.ActorUsername
}

// CreateNotification inserts a notification for recipientID.
func (db *DB) CreateNotification(ctx context.Context, recipientID, actorID int64, kind string, threadID int64, commentID *int64, now time.Time) (*Notification, error) {
	result, err := db.ExecContext(ctx, `
		INSERT INTO notifications (
			recipient_id, kind, actor_id, thread_id, comment_id, created_at
		)
		VALUES (?, ?, ?, ?, ?, ?)`,
		recipientID, kind, actorID, threadID, nullableInt64(commentID), now.Unix(),
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return db.NotificationByID(ctx, id)
}

// NotificationByID returns a notification or nil when no notification has that
// ID.
func (db *DB) NotificationByID(ctx context.Context, id int64) (*Notification, error) {
	notification, err := scanNotification(db.QueryRowContext(ctx, `
		SELECT n.id, n.recipient_id, n.kind, n.actor_id, u.username,
			u.deleted_at, n.thread_id, t.title, n.comment_id, n.created_at,
			n.read_at
		FROM notifications n
		JOIN users u ON u.id = n.actor_id
		JOIN threads t ON t.id = n.thread_id
		WHERE n.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return notification, nil
}

// ListNotifications returns a recipient's notifications, newest first.
func (db *DB) ListNotifications(ctx context.Context, recipientID int64) ([]Notification, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT n.id, n.recipient_id, n.kind, n.actor_id, u.username,
			u.deleted_at, n.thread_id, t.title, n.comment_id, n.created_at,
			n.read_at
		FROM notifications n
		JOIN users u ON u.id = n.actor_id
		JOIN threads t ON t.id = n.thread_id
		WHERE n.recipient_id = ?
		ORDER BY n.created_at DESC, n.id DESC`, recipientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	notifications := []Notification{}
	for rows.Next() {
		notification, err := scanNotification(rows)
		if err != nil {
			return nil, err
		}
		notifications = append(notifications, *notification)
	}
	return notifications, rows.Err()
}

// CountUnreadNotifications returns the number of unread notifications for a
// recipient. It is used for the header badge.
func (db *DB) CountUnreadNotifications(ctx context.Context, recipientID int64) (int64, error) {
	var count int64
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM notifications
		WHERE recipient_id = ? AND read_at IS NULL`, recipientID).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// MarkNotificationsRead marks every unread notification for a recipient as
// read.
func (db *DB) MarkNotificationsRead(ctx context.Context, recipientID int64, now time.Time) error {
	_, err := db.ExecContext(ctx, `
		UPDATE notifications SET read_at = ?
		WHERE recipient_id = ? AND read_at IS NULL`, now.Unix(), recipientID)
	return err
}

func scanNotification(row rowScanner) (*Notification, error) {
	var (
		notification Notification
		actorDeleted sql.NullInt64
		commentID    sql.NullInt64
		createdAt    int64
		readAt       sql.NullInt64
	)
	if err := row.Scan(
		&notification.ID,
		&notification.RecipientID,
		&notification.Kind,
		&notification.ActorID,
		&notification.ActorUsername,
		&actorDeleted,
		&notification.ThreadID,
		&notification.ThreadTitle,
		&commentID,
		&createdAt,
		&readAt,
	); err != nil {
		return nil, err
	}

	notification.ActorDeletedAt = nullTimePtr(actorDeleted)
	notification.CommentID = nullInt64Ptr(commentID)
	notification.CreatedAt = timeFromUnix(createdAt)
	notification.ReadAt = nullTimePtr(readAt)
	return &notification, nil
}
