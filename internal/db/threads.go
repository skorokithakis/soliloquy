package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/skorokithakis/soliloquy/internal/auth"
)

// Thread is a forum thread. Body is the Markdown source and BodyHTML is the
// sanitized HTML rendered when the thread was saved.
//
// ID is the internal integer key. PublicID is the random identifier clients
// see in URLs and live-update payloads.
//
// Comment code maintains LastActivityAt and vote code maintains Score on the
// thread row. CommentCount is computed from the comments table when the thread
// is read.
type Thread struct {
	ID              int64
	PublicID        string
	AuthorID        int64
	Title           string
	URL             string
	Body            string
	BodyHTML        string
	CreatedAt       time.Time
	EditedAt        *time.Time
	DeletedAt       *time.Time
	LastActivityAt  time.Time
	Score           int64
	CommentCount    int64
	AuthorUsername  string
	AuthorDeletedAt *time.Time
	// Voted is true when the viewer passed to a list query has an active vote
	// on the thread. It is false for viewer 0.
	Voted bool
}

// AuthorName is the name shown for the thread's author, including on threads
// whose author has deleted their account.
func (t *Thread) AuthorName() string {
	if t.AuthorDeletedAt != nil {
		return "[deleted user]"
	}
	return t.AuthorUsername
}

const threadColumns = `t.id, t.public_id, t.author_id, t.title, t.url, t.body, t.body_html,
	t.created_at, t.edited_at, t.deleted_at, t.last_activity_at, t.score,
	(SELECT COUNT(*) FROM comments c WHERE c.thread_id = t.id AND c.deleted_at IS NULL),
	u.username, u.deleted_at`

const threadListColumns = threadColumns + `,
	EXISTS(SELECT 1 FROM votes v
		WHERE v.item_type = 'thread' AND v.item_id = t.id AND v.user_id = ?)`

func scanThread(row rowScanner) (*Thread, error) {
	return scanThreadRow(row, false)
}

func scanThreadRow(row rowScanner, withVoted bool) (*Thread, error) {
	var (
		thread          Thread
		createdAt       int64
		editedAt        sql.NullInt64
		deletedAt       sql.NullInt64
		lastActivityAt  int64
		authorDeletedAt sql.NullInt64
	)
	dest := []any{
		&thread.ID,
		&thread.PublicID,
		&thread.AuthorID,
		&thread.Title,
		&thread.URL,
		&thread.Body,
		&thread.BodyHTML,
		&createdAt,
		&editedAt,
		&deletedAt,
		&lastActivityAt,
		&thread.Score,
		&thread.CommentCount,
		&thread.AuthorUsername,
		&authorDeletedAt,
	}
	if withVoted {
		dest = append(dest, &thread.Voted)
	}
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}

	thread.CreatedAt = timeFromUnix(createdAt)
	thread.EditedAt = nullTimePtr(editedAt)
	thread.DeletedAt = nullTimePtr(deletedAt)
	thread.LastActivityAt = timeFromUnix(lastActivityAt)
	thread.AuthorDeletedAt = nullTimePtr(authorDeletedAt)
	return &thread, nil
}

// CreateThread inserts a thread with its initial score of 1. The thread's
// created and last activity times both start at now.
func (db *DB) CreateThread(ctx context.Context, authorID int64, title, rawURL, body, bodyHTML string, now time.Time) (*Thread, error) {
	publicID, err := auth.GenerateShortID()
	if err != nil {
		return nil, err
	}

	result, err := db.ExecContext(ctx, `
		INSERT INTO threads (
			public_id, author_id, title, url, body, body_html, created_at,
			last_activity_at, score
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1)`,
		publicID, authorID, title, rawURL, body, bodyHTML, now.Unix(), now.Unix(),
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return db.ThreadByID(ctx, id)
}

// ThreadByID returns a thread, including a soft-deleted one, or nil when no
// thread has that ID. Callers decide whether a deleted thread is visible.
func (db *DB) ThreadByID(ctx context.Context, id int64) (*Thread, error) {
	return db.threadBy(ctx, "t.id = ?", id)
}

// ThreadByPublicID returns a thread, including a soft-deleted one, or nil when
// no thread has that public ID. Handlers use it to resolve the ID in a URL.
func (db *DB) ThreadByPublicID(ctx context.Context, publicID string) (*Thread, error) {
	return db.threadBy(ctx, "t.public_id = ?", publicID)
}

func (db *DB) threadBy(ctx context.Context, where string, value any) (*Thread, error) {
	thread, err := scanThread(db.QueryRowContext(ctx, `
		SELECT `+threadColumns+`
		FROM threads t JOIN users u ON u.id = t.author_id
		WHERE `+where, value))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return thread, nil
}

// UpdateThread edits a thread and marks it edited. It only updates a live
// thread owned by authorID; callers pass the thread's owner, so an admin may
// edit another user's thread after checking that they are allowed to. It
// reports whether a row changed.
func (db *DB) UpdateThread(ctx context.Context, id, authorID int64, title, rawURL, body, bodyHTML string, now time.Time) (bool, error) {
	result, err := db.ExecContext(ctx, `
		UPDATE threads
		SET title = ?, url = ?, body = ?, body_html = ?, edited_at = ?
		WHERE id = ? AND author_id = ? AND deleted_at IS NULL`,
		title, rawURL, body, bodyHTML, now.Unix(), id, authorID,
	)
	if err != nil {
		return false, err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected == 1, nil
}

// DeleteThread soft-deletes a thread. It deliberately does not check who is
// calling it: handlers must authorize the author, and the admin delete task
// can call the same method after its own admin check.
func (db *DB) DeleteThread(ctx context.Context, id int64, now time.Time) (bool, error) {
	result, err := db.ExecContext(ctx, `
		UPDATE threads SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL`,
		now.Unix(), id,
	)
	if err != nil {
		return false, err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected == 1, nil
}

// ListThreads returns live threads ordered by most recent activity, along with
// the viewer's vote state. Ties are broken by the newest thread. Callers ask
// for one row more than the page size to find out whether another page exists.
func (db *DB) ListThreads(ctx context.Context, viewerID int64, limit, offset int) ([]Thread, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+threadListColumns+`
		FROM threads t JOIN users u ON u.id = t.author_id
		WHERE t.deleted_at IS NULL
		ORDER BY t.last_activity_at DESC, t.id DESC
		LIMIT ? OFFSET ?`, viewerID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	threads := []Thread{}
	for rows.Next() {
		thread, err := scanThreadRow(rows, true)
		if err != nil {
			return nil, err
		}
		threads = append(threads, *thread)
	}
	return threads, rows.Err()
}

// CountThreads returns the number of threads visible on the front page.
func (db *DB) CountThreads(ctx context.Context) (int64, error) {
	var count int64
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM threads WHERE deleted_at IS NULL`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}
