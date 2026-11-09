package db

import (
	"context"
	"database/sql"
	"errors"
)

// Errors returned by vote operations.
var (
	ErrVoteOwnPost  = errors.New("cannot vote on your own post")
	ErrVoteNotFound = errors.New("voting target not found")
)

// VoteResult is the state of an item after a vote toggle.
type VoteResult struct {
	Score int64
	Voted bool
}

// ToggleThreadVote adds or removes a user's vote on a thread and returns the
// resulting score. Voting on your own thread is rejected.
func (db *DB) ToggleThreadVote(ctx context.Context, userID, threadID int64) (VoteResult, error) {
	return db.toggleVote(ctx, userID, "threads", "thread", threadID)
}

// ToggleCommentVote adds or removes a user's vote on a comment and returns the
// resulting score. Voting on your own comment is rejected.
func (db *DB) ToggleCommentVote(ctx context.Context, userID, commentID int64) (VoteResult, error) {
	return db.toggleVote(ctx, userID, "comments", "comment", commentID)
}

// ThreadVoted reports whether a user has voted on a thread.
func (db *DB) ThreadVoted(ctx context.Context, userID, threadID int64) (bool, error) {
	var voted bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM votes
			WHERE user_id = ? AND item_type = 'thread' AND item_id = ?
		)`, userID, threadID).Scan(&voted)
	return voted, err
}

// toggleVote changes a vote in one transaction. table is either "threads" or
// "comments" and itemType is its matching votes.item_type value; both are
// constants chosen by the exported methods, never user input.
func (db *DB) toggleVote(ctx context.Context, userID int64, table, itemType string, itemID int64) (VoteResult, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return VoteResult{}, err
	}
	defer tx.Rollback()

	var (
		authorID  int64
		deletedAt sql.NullInt64
	)
	err = tx.QueryRowContext(ctx,
		`SELECT author_id, deleted_at FROM `+table+` WHERE id = ?`, itemID,
	).Scan(&authorID, &deletedAt)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && deletedAt.Valid) {
		return VoteResult{}, ErrVoteNotFound
	}
	if err != nil {
		return VoteResult{}, err
	}
	if authorID == userID {
		return VoteResult{}, ErrVoteOwnPost
	}

	removed, err := tx.ExecContext(ctx, `
		DELETE FROM votes
		WHERE user_id = ? AND item_type = ? AND item_id = ?`,
		userID, itemType, itemID,
	)
	if err != nil {
		return VoteResult{}, err
	}
	affected, err := removed.RowsAffected()
	if err != nil {
		return VoteResult{}, err
	}

	delta := int64(1)
	voted := true
	if affected == 1 {
		delta = -1
		voted = false
	} else {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO votes (user_id, item_type, item_id) VALUES (?, ?, ?)`,
			userID, itemType, itemID,
		); err != nil {
			return VoteResult{}, err
		}
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE `+table+` SET score = score + ? WHERE id = ?`, delta, itemID,
	); err != nil {
		return VoteResult{}, err
	}

	var score int64
	if err := tx.QueryRowContext(ctx,
		`SELECT score FROM `+table+` WHERE id = ?`, itemID,
	).Scan(&score); err != nil {
		return VoteResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return VoteResult{}, err
	}

	return VoteResult{Score: score, Voted: voted}, nil
}
