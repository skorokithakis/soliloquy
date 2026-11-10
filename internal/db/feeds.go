package db

import (
	"context"
	"database/sql"
	"time"
)

// FeedComment is a comment in the comments feed. It carries the public ID and
// title of its thread so the feed can link to the comment and name its thread.
// Plain Comment has the thread's internal ID only.
type FeedComment struct {
	PublicID        string
	ThreadPublicID  string
	ThreadTitle     string
	BodyHTML        string
	CreatedAt       time.Time
	AuthorUsername  string
	AuthorDeletedAt *time.Time
}

// AuthorName is the name shown for the comment's author, including on comments
// whose author has deleted their account.
func (c *FeedComment) AuthorName() string {
	if c.AuthorDeletedAt != nil {
		return "[deleted user]"
	}
	return c.AuthorUsername
}

// ListFeedThreads returns the newest live threads, newest first. The feed
// shows a fixed number of items, so there is no offset.
func (db *DB) ListFeedThreads(ctx context.Context, limit int) ([]Thread, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+threadColumns+`
		FROM threads t JOIN users u ON u.id = t.author_id
		WHERE t.deleted_at IS NULL
		ORDER BY t.created_at DESC, t.id DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	threads := []Thread{}
	for rows.Next() {
		thread, err := scanThread(rows)
		if err != nil {
			return nil, err
		}
		threads = append(threads, *thread)
	}
	return threads, rows.Err()
}

// ListFeedComments returns the newest live comments, newest first, skipping
// comments on deleted threads. The feed shows a fixed number of items, so
// there is no offset.
func (db *DB) ListFeedComments(ctx context.Context, limit int) ([]FeedComment, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT c.public_id, t.public_id, t.title, c.body_html, c.created_at,
			u.username, u.deleted_at
		FROM comments c
		JOIN threads t ON t.id = c.thread_id
		JOIN users u ON u.id = c.author_id
		WHERE c.deleted_at IS NULL AND t.deleted_at IS NULL
		ORDER BY c.created_at DESC, c.id DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	comments := []FeedComment{}
	for rows.Next() {
		var (
			comment         FeedComment
			createdAt       int64
			authorDeletedAt sql.NullInt64
		)
		if err := rows.Scan(
			&comment.PublicID,
			&comment.ThreadPublicID,
			&comment.ThreadTitle,
			&comment.BodyHTML,
			&createdAt,
			&comment.AuthorUsername,
			&authorDeletedAt,
		); err != nil {
			return nil, err
		}
		comment.CreatedAt = timeFromUnix(createdAt)
		comment.AuthorDeletedAt = nullTimePtr(authorDeletedAt)
		comments = append(comments, comment)
	}
	return comments, rows.Err()
}
