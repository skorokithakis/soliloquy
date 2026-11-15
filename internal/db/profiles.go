package db

import (
	"context"
	"time"
)

// ProfileComment is one of a member's comments as shown on their profile. It
// carries the thread's public ID and title so the page can link back to it.
type ProfileComment struct {
	PublicID       string
	ThreadPublicID string
	ThreadTitle    string
	Body           string
	CreatedAt      time.Time
}

// ListThreadsByAuthor returns a member's live threads, newest first. Callers
// pass the number of rows they want; profile pages have no paging.
func (db *DB) ListThreadsByAuthor(ctx context.Context, authorID int64, limit int) ([]Thread, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+threadColumns+`
		FROM threads t JOIN users u ON u.id = t.author_id
		WHERE t.author_id = ? AND t.deleted_at IS NULL
		ORDER BY t.created_at DESC, t.id DESC
		LIMIT ?`, authorID, limit)
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

// ListCommentsByAuthor returns a member's live comments, newest first.
// Comments on deleted threads are omitted along with deleted comments.
func (db *DB) ListCommentsByAuthor(ctx context.Context, authorID int64, limit int) ([]ProfileComment, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT c.public_id, t.public_id, t.title, c.body, c.created_at
		FROM comments c JOIN threads t ON t.id = c.thread_id
		WHERE c.author_id = ? AND c.deleted_at IS NULL AND t.deleted_at IS NULL
		ORDER BY c.created_at DESC, c.id DESC
		LIMIT ?`, authorID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	comments := []ProfileComment{}
	for rows.Next() {
		var (
			comment   ProfileComment
			createdAt int64
		)
		if err := rows.Scan(
			&comment.PublicID,
			&comment.ThreadPublicID,
			&comment.ThreadTitle,
			&comment.Body,
			&createdAt,
		); err != nil {
			return nil, err
		}
		comment.CreatedAt = timeFromUnix(createdAt)
		comments = append(comments, comment)
	}
	return comments, rows.Err()
}
