package db

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"

	"github.com/skorokithakis/soliloquy/internal/auth"
)

// Comment is a reply in a thread. Body is the Markdown source and BodyHTML is
// the sanitized HTML rendered when the comment was saved. ParentID is nil for
// top-level comments.
//
// ID is the internal integer key. PublicID is the random identifier clients see
// in URLs, anchors and live-update payloads; ThreadPublicID and ParentPublicID
// are the public IDs of the thread and parent comment.
//
// Voted is true when the viewer passed to the query has an active vote on the
// comment. It is false for viewer 0, which callers use when no user is
// involved.
type Comment struct {
	ID              int64
	PublicID        string
	ThreadID        int64
	ThreadPublicID  string
	ParentID        *int64
	ParentPublicID  *string
	AuthorID        int64
	Body            string
	BodyHTML        string
	CreatedAt       time.Time
	EditedAt        *time.Time
	DeletedAt       *time.Time
	Score           int64
	AuthorUsername  string
	AuthorDeletedAt *time.Time
	Voted           bool
}

// AuthorName is the name shown for the comment's author, including on
// comments whose author has deleted their account.
func (c *Comment) AuthorName() string {
	if c.AuthorDeletedAt != nil {
		return "[deleted user]"
	}
	return c.AuthorUsername
}

// CommentNode is a comment and its visible replies. Deleted comments that have
// no visible descendants are omitted entirely, which keeps a deleted subtree
// from leaving empty placeholders behind.
type CommentNode struct {
	Comment Comment
	Replies []*CommentNode
}

const commentColumns = `c.id, c.public_id, c.thread_id, t.public_id, c.parent_id,
	(SELECT p.public_id FROM comments p WHERE p.id = c.parent_id),
	c.author_id, c.body,
	c.body_html, c.created_at, c.edited_at, c.deleted_at, c.score, u.username,
	u.deleted_at,
	EXISTS(SELECT 1 FROM votes v
		WHERE v.item_type = 'comment' AND v.item_id = c.id AND v.user_id = ?)`

func scanComment(row rowScanner) (*Comment, error) {
	var (
		comment         Comment
		parentID        sql.NullInt64
		parentPublicID  sql.NullString
		createdAt       int64
		editedAt        sql.NullInt64
		deletedAt       sql.NullInt64
		authorDeletedAt sql.NullInt64
	)
	if err := row.Scan(
		&comment.ID,
		&comment.PublicID,
		&comment.ThreadID,
		&comment.ThreadPublicID,
		&parentID,
		&parentPublicID,
		&comment.AuthorID,
		&comment.Body,
		&comment.BodyHTML,
		&createdAt,
		&editedAt,
		&deletedAt,
		&comment.Score,
		&comment.AuthorUsername,
		&authorDeletedAt,
		&comment.Voted,
	); err != nil {
		return nil, err
	}

	comment.ParentID = nullInt64Ptr(parentID)
	if parentPublicID.Valid {
		comment.ParentPublicID = &parentPublicID.String
	}
	comment.CreatedAt = timeFromUnix(createdAt)
	comment.EditedAt = nullTimePtr(editedAt)
	comment.DeletedAt = nullTimePtr(deletedAt)
	comment.AuthorDeletedAt = nullTimePtr(authorDeletedAt)
	return &comment, nil
}

// ErrCommentNotFound is returned when a comment cannot be created because its
// thread or parent comment is missing or deleted.
var ErrCommentNotFound = errors.New("comment target not found")

// CreateComment inserts a comment and moves the thread's last activity time
// forward. The insert only happens while the thread is live and, for a reply,
// the parent is a live comment in the same thread. It returns ErrCommentNotFound
// otherwise.
func (db *DB) CreateComment(ctx context.Context, threadID int64, parentID *int64, authorID int64, body, bodyHTML string, now time.Time) (*Comment, error) {
	publicID, err := auth.GenerateShortID()
	if err != nil {
		return nil, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	parent := nullableInt64(parentID)
	result, err := tx.ExecContext(ctx, `
		INSERT INTO comments (
			public_id, thread_id, parent_id, author_id, body, body_html,
			created_at, score
		)
		SELECT ?, ?, ?, ?, ?, ?, ?, 1
		WHERE EXISTS (
			SELECT 1 FROM threads t
			WHERE t.id = ? AND t.deleted_at IS NULL
		)
		AND (
			? IS NULL OR EXISTS (
				SELECT 1 FROM comments p
				WHERE p.id = ? AND p.thread_id = ? AND p.deleted_at IS NULL
			)
		)`,
		publicID, threadID, parent, authorID, body, bodyHTML, now.Unix(),
		threadID, parent, parent, threadID,
	)
	if err != nil {
		return nil, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected != 1 {
		return nil, ErrCommentNotFound
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE threads
		SET last_activity_at = CASE WHEN last_activity_at < ? THEN ? ELSE last_activity_at END
		WHERE id = ?`,
		now.Unix(), now.Unix(), threadID,
	); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return db.CommentByID(ctx, id, 0)
}

// CommentByID returns a comment, including a soft-deleted one, or nil when no
// comment has that ID. Callers decide whether a deleted comment is visible.
func (db *DB) CommentByID(ctx context.Context, id, viewerID int64) (*Comment, error) {
	return db.commentBy(ctx, viewerID, "c.id = ?", id)
}

// CommentByPublicID returns a comment, including a soft-deleted one, or nil
// when no comment has that public ID. Handlers use it to resolve the ID in a
// URL.
func (db *DB) CommentByPublicID(ctx context.Context, publicID string, viewerID int64) (*Comment, error) {
	return db.commentBy(ctx, viewerID, "c.public_id = ?", publicID)
}

func (db *DB) commentBy(ctx context.Context, viewerID int64, where string, value any) (*Comment, error) {
	comment, err := scanComment(db.QueryRowContext(ctx, `
		SELECT `+commentColumns+`
		FROM comments c
		JOIN users u ON u.id = c.author_id
		JOIN threads t ON t.id = c.thread_id
		WHERE `+where, viewerID, value))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return comment, nil
}

// ListComments returns every comment in a thread, including deleted ones, in
// one query. BuildCommentTree assembles and filters them for display.
func (db *DB) ListComments(ctx context.Context, threadID, viewerID int64) ([]Comment, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+commentColumns+`
		FROM comments c
		JOIN users u ON u.id = c.author_id
		JOIN threads t ON t.id = c.thread_id
		WHERE c.thread_id = ?
		ORDER BY c.created_at, c.id`, viewerID, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	comments := []Comment{}
	for rows.Next() {
		comment, err := scanComment(rows)
		if err != nil {
			return nil, err
		}
		comments = append(comments, *comment)
	}
	return comments, rows.Err()
}

// UpdateComment edits a comment and marks it edited. It only updates a live
// comment owned by authorID, so a request cannot edit another user's comment
// even if an authorization check is missed. It reports whether a row changed.
func (db *DB) UpdateComment(ctx context.Context, id, authorID int64, body, bodyHTML string, now time.Time) (bool, error) {
	result, err := db.ExecContext(ctx, `
		UPDATE comments
		SET body = ?, body_html = ?, edited_at = ?
		WHERE id = ? AND author_id = ? AND deleted_at IS NULL`,
		body, bodyHTML, now.Unix(), id, authorID,
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

// DeleteComment soft-deletes a comment. It deliberately does not check who is
// calling it: handlers must authorize the author, and the admin delete task can
// call the same method after its own admin check.
func (db *DB) DeleteComment(ctx context.Context, id int64, now time.Time) (bool, error) {
	result, err := db.ExecContext(ctx, `
		UPDATE comments SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL`,
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

// BuildCommentTree nests a thread's flat comment list and removes deleted
// comments that have no visible descendants. Siblings are ordered by score
// descending, then oldest first.
func BuildCommentTree(comments []Comment) []*CommentNode {
	present := make(map[int64]bool, len(comments))
	for _, comment := range comments {
		present[comment.ID] = true
	}

	children := make(map[int64][]Comment, len(comments))
	var roots []Comment
	for _, comment := range comments {
		// A parent that is missing from the list (which should not happen,
		// because comments are soft-deleted) is treated as a root so the
		// comment is not lost.
		if comment.ParentID == nil || !present[*comment.ParentID] {
			roots = append(roots, comment)
			continue
		}
		children[*comment.ParentID] = append(children[*comment.ParentID], comment)
	}

	var build func([]Comment) []*CommentNode
	build = func(siblings []Comment) []*CommentNode {
		sortComments(siblings)

		nodes := make([]*CommentNode, 0, len(siblings))
		for _, comment := range siblings {
			replies := build(children[comment.ID])
			if comment.DeletedAt != nil && len(replies) == 0 {
				continue
			}
			nodes = append(nodes, &CommentNode{Comment: comment, Replies: replies})
		}
		return nodes
	}

	return build(roots)
}

func sortComments(comments []Comment) {
	sort.SliceStable(comments, func(i, j int) bool {
		if comments[i].Score != comments[j].Score {
			return comments[i].Score > comments[j].Score
		}
		if !comments[i].CreatedAt.Equal(comments[j].CreatedAt) {
			return comments[i].CreatedAt.Before(comments[j].CreatedAt)
		}
		return comments[i].ID < comments[j].ID
	})
}
