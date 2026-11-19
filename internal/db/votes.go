package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Errors returned by vote operations.
var (
	ErrVoteOwnPost     = errors.New("cannot vote on your own post")
	ErrVoteNotFound    = errors.New("voting target not found")
	ErrInvalidReaction = errors.New("unknown reaction")
)

// Reaction is the key of an emoji reaction on a comment vote. The key, not the
// emoji character, is stored in the database; the web layer chooses the emoji
// to display.
type Reaction string

// The supported reaction keys.
const (
	ReactionHeart  Reaction = "heart"
	ReactionLaugh  Reaction = "laugh"
	ReactionIdea   Reaction = "idea"
	ReactionThanks Reaction = "thanks"
	ReactionParty  Reaction = "party"
)

// CommentReactions lists the supported reaction keys in display order.
var CommentReactions = []Reaction{
	ReactionHeart,
	ReactionLaugh,
	ReactionIdea,
	ReactionThanks,
	ReactionParty,
}

// Valid reports whether r is one of CommentReactions.
func (r Reaction) Valid() bool {
	for _, known := range CommentReactions {
		if r == known {
			return true
		}
	}
	return false
}

// VoteResult is the state of an item after a vote toggle or a reaction change.
// Reaction is the member's own reaction and ReactionCounts is the number of
// votes for each reaction. Both are only filled for comments; a thread result
// has an empty reaction and nil counts.
type VoteResult struct {
	Score          int64
	Voted          bool
	Reaction       Reaction
	ReactionCounts map[Reaction]int64
}

// ToggleThreadVote adds or removes a user's vote on a thread and returns the
// resulting score. Voting on your own thread is rejected.
func (db *DB) ToggleThreadVote(ctx context.Context, userID, threadID int64) (VoteResult, error) {
	return db.toggleVote(ctx, userID, "threads", "thread", threadID)
}

// ToggleCommentVote adds or removes a user's vote on a comment and returns the
// resulting score and reaction state. Voting on your own comment is rejected.
func (db *DB) ToggleCommentVote(ctx context.Context, userID, commentID int64) (VoteResult, error) {
	return db.toggleVote(ctx, userID, "comments", "comment", commentID)
}

// SetCommentReaction sets, replaces, or removes a user's reaction on a comment.
// With no vote yet it adds the vote and the reaction, so the score rises by
// one. A plain vote or another reaction is relabelled with no score change.
// Setting the reaction that is already there removes both the reaction and the
// vote, so the score falls by one. Voting on your own comment and reacting to a
// missing or deleted comment are rejected.
func (db *DB) SetCommentReaction(ctx context.Context, userID, commentID int64, reaction Reaction) (VoteResult, error) {
	if !reaction.Valid() {
		return VoteResult{}, ErrInvalidReaction
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return VoteResult{}, err
	}
	defer tx.Rollback()

	if err := checkVoteTarget(ctx, tx, "comments", commentID, userID); err != nil {
		return VoteResult{}, err
	}

	var current sql.NullString
	err = tx.QueryRowContext(ctx, `
		SELECT reaction FROM votes
		WHERE user_id = ? AND item_type = 'comment' AND item_id = ?`,
		userID, commentID,
	).Scan(&current)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		// No vote yet: the reaction adds the vote.
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO votes (user_id, item_type, item_id, reaction)
			VALUES (?, 'comment', ?, ?)`,
			userID, commentID, reaction,
		); err != nil {
			return VoteResult{}, err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE comments SET score = score + 1 WHERE id = ?`, commentID,
		); err != nil {
			return VoteResult{}, err
		}
	case err != nil:
		return VoteResult{}, err
	case current.Valid && Reaction(current.String) == reaction:
		// The same reaction again removes the emoji and the vote.
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM votes
			WHERE user_id = ? AND item_type = 'comment' AND item_id = ?`,
			userID, commentID,
		); err != nil {
			return VoteResult{}, err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE comments SET score = score - 1 WHERE id = ?`, commentID,
		); err != nil {
			return VoteResult{}, err
		}
	default:
		// A plain vote or another reaction becomes this reaction. The score
		// does not change.
		if _, err := tx.ExecContext(ctx, `
			UPDATE votes SET reaction = ?
			WHERE user_id = ? AND item_type = 'comment' AND item_id = ?`,
			reaction, userID, commentID,
		); err != nil {
			return VoteResult{}, err
		}
	}

	var score int64
	if err := tx.QueryRowContext(ctx,
		`SELECT score FROM comments WHERE id = ?`, commentID,
	).Scan(&score); err != nil {
		return VoteResult{}, err
	}

	viewerReaction, reactionCounts, err := commentReactionState(ctx, tx, userID, commentID)
	if err != nil {
		return VoteResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return VoteResult{}, err
	}

	return VoteResult{
		Score:          score,
		Voted:          viewerReaction != "",
		Reaction:       viewerReaction,
		ReactionCounts: reactionCounts,
	}, nil
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

	if err := checkVoteTarget(ctx, tx, table, itemID, userID); err != nil {
		return VoteResult{}, err
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

	result := VoteResult{Score: score, Voted: voted}
	if itemType == "comment" {
		// A comment toggle also deletes any reaction with the vote, so the
		// caller needs the new reaction state.
		viewerReaction, reactionCounts, err := commentReactionState(ctx, tx, userID, itemID)
		if err != nil {
			return VoteResult{}, err
		}
		result.Reaction = viewerReaction
		result.ReactionCounts = reactionCounts
	}

	if err := tx.Commit(); err != nil {
		return VoteResult{}, err
	}

	return result, nil
}

// checkVoteTarget verifies that the item exists, is not deleted, and was not
// written by the voter. table is "threads" or "comments", chosen by the
// exported methods, never user input.
func checkVoteTarget(ctx context.Context, q execQueryer, table string, itemID, userID int64) error {
	var (
		authorID  int64
		deletedAt sql.NullInt64
	)
	err := q.QueryRowContext(ctx,
		`SELECT author_id, deleted_at FROM `+table+` WHERE id = ?`, itemID,
	).Scan(&authorID, &deletedAt)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && deletedAt.Valid) {
		return ErrVoteNotFound
	}
	if err != nil {
		return err
	}
	if authorID == userID {
		return ErrVoteOwnPost
	}
	return nil
}

// newReactionCounts returns a count for every supported reaction, including
// the ones with no votes.
func newReactionCounts() map[Reaction]int64 {
	counts := make(map[Reaction]int64, len(CommentReactions))
	for _, reaction := range CommentReactions {
		counts[reaction] = 0
	}
	return counts
}

// reactionCountColumns returns a SELECT-list fragment with a count column for
// each supported reaction, in the order of CommentReactions. The fragment
// starts with a comma, so it must follow an existing column. item is the SQL
// expression for the vote's item_id and must refer to a comment. The reaction
// keys are package constants, never user input, so they are safe to inline.
func reactionCountColumns(item string) string {
	var b strings.Builder
	for _, reaction := range CommentReactions {
		fmt.Fprintf(&b, `,
	(SELECT COUNT(*) FROM votes v
		WHERE v.item_type = 'comment' AND v.item_id = %s AND v.reaction = '%s')`,
			item, reaction)
	}
	return b.String()
}

// commentReactionState returns the member's own reaction on a comment, empty
// when they have none, and the comment's reaction counts.
func commentReactionState(ctx context.Context, q execQueryer, userID, commentID int64) (Reaction, map[Reaction]int64, error) {
	query := `SELECT
	(SELECT v.reaction FROM votes v
		WHERE v.user_id = ? AND v.item_type = 'comment' AND v.item_id = ?)` +
		reactionCountColumns("?")

	args := make([]any, 0, len(CommentReactions)+2)
	args = append(args, userID, commentID)
	for range CommentReactions {
		args = append(args, commentID)
	}

	var viewerReaction sql.NullString
	counts := make([]int64, len(CommentReactions))
	dest := make([]any, 0, len(counts)+1)
	dest = append(dest, &viewerReaction)
	for i := range counts {
		dest = append(dest, &counts[i])
	}

	if err := q.QueryRowContext(ctx, query, args...).Scan(dest...); err != nil {
		return "", nil, err
	}

	reactionCounts := newReactionCounts()
	for i, reaction := range CommentReactions {
		reactionCounts[reaction] = counts[i]
	}

	result := Reaction("")
	if viewerReaction.Valid {
		result = Reaction(viewerReaction.String)
	}
	return result, reactionCounts, nil
}
