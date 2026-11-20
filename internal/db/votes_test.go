package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

// reactionFixture creates two members, a thread by the first, and a comment by
// the first, so the second member can vote and react on it.
func reactionFixture(t *testing.T) (*DB, *User, *User, *Thread, *Comment) {
	t.Helper()

	database := migratedTestDB(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	alice, err := database.CreateUser(ctx, "alice", "hash", false, nil, now)
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, err := database.CreateUser(ctx, "bob", "hash", false, nil, now)
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}
	thread, err := database.CreateThread(ctx, alice.ID, "Thread", "", "body", "<p>body</p>", now)
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	comment, err := database.CreateComment(ctx, thread.ID, nil, alice.ID, "body", "<p>body</p>", now)
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}

	return database, alice, bob, thread, comment
}

func TestAuthorVoteIsARowAndToggles(t *testing.T) {
	database, alice, _, thread, comment := reactionFixture(t)
	ctx := context.Background()

	countVotes := func(itemType string, itemID int64) int64 {
		t.Helper()
		var count int64
		if err := database.QueryRow(
			`SELECT COUNT(*) FROM votes WHERE user_id = ? AND item_type = ? AND item_id = ?`,
			alice.ID, itemType, itemID).Scan(&count); err != nil {
			t.Fatalf("count %s votes: %v", itemType, err)
		}
		return count
	}

	// Creating an item inserts the author's vote, and the page queries see it.
	if got := countVotes("thread", thread.ID); got != 1 {
		t.Errorf("author thread vote rows = %d, want 1", got)
	}
	if got := countVotes("comment", comment.ID); got != 1 {
		t.Errorf("author comment vote rows = %d, want 1", got)
	}
	if voted, err := database.ThreadVoted(ctx, alice.ID, thread.ID); err != nil || !voted {
		t.Errorf("ThreadVoted for the author = %t, %v; want true, nil", voted, err)
	}

	// Removing and restoring the author's own thread vote moves the score
	// between 1 and 0 like any other member's.
	result, err := database.ToggleThreadVote(ctx, alice.ID, thread.ID)
	if err != nil {
		t.Fatalf("ToggleThreadVote off: %v", err)
	}
	if result.Score != 0 || result.Voted {
		t.Errorf("author thread unvote = %+v, want score 0, not voted", result)
	}
	if got := countVotes("thread", thread.ID); got != 0 {
		t.Errorf("author thread vote rows after unvote = %d, want 0", got)
	}

	result, err = database.ToggleThreadVote(ctx, alice.ID, thread.ID)
	if err != nil {
		t.Fatalf("ToggleThreadVote on: %v", err)
	}
	if result.Score != 1 || !result.Voted {
		t.Errorf("author thread revote = %+v, want score 1, voted", result)
	}

	result, err = database.ToggleCommentVote(ctx, alice.ID, comment.ID)
	if err != nil {
		t.Fatalf("ToggleCommentVote off: %v", err)
	}
	if result.Score != 0 || result.Voted {
		t.Errorf("author comment unvote = %+v, want score 0, not voted", result)
	}
	assertCommentScore(t, database, comment.ID)

	result, err = database.ToggleCommentVote(ctx, alice.ID, comment.ID)
	if err != nil {
		t.Fatalf("ToggleCommentVote on: %v", err)
	}
	if result.Score != 1 || !result.Voted {
		t.Errorf("author comment revote = %+v, want score 1, voted", result)
	}
	assertCommentScore(t, database, comment.ID)
}

func TestSetCommentReactionTransitions(t *testing.T) {
	database, _, bob, _, comment := reactionFixture(t)
	ctx := context.Background()

	check := func(result VoteResult, wantScore int64, wantVoted bool, wantReaction Reaction, wantCounts map[Reaction]int64) {
		t.Helper()
		if result.Score != wantScore || result.Voted != wantVoted || result.Reaction != wantReaction {
			t.Fatalf("result = %+v, want score %d, voted %t, reaction %q",
				result, wantScore, wantVoted, wantReaction)
		}
		assertReactionCounts(t, result.ReactionCounts, wantCounts)
		assertCommentScore(t, database, comment.ID)
	}

	// No vote yet: the reaction adds the vote and raises the score.
	result, err := database.SetCommentReaction(ctx, bob.ID, comment.ID, ReactionHeart)
	if err != nil {
		t.Fatalf("SetCommentReaction heart: %v", err)
	}
	check(result, 2, true, ReactionHeart, map[Reaction]int64{ReactionHeart: 1})

	// The same reaction again removes the emoji and the vote.
	result, err = database.SetCommentReaction(ctx, bob.ID, comment.ID, ReactionHeart)
	if err != nil {
		t.Fatalf("SetCommentReaction heart again: %v", err)
	}
	check(result, 1, false, "", nil)

	// A plain upvote becomes a reaction without changing the score.
	if _, err := database.ToggleCommentVote(ctx, bob.ID, comment.ID); err != nil {
		t.Fatalf("ToggleCommentVote: %v", err)
	}
	result, err = database.SetCommentReaction(ctx, bob.ID, comment.ID, ReactionLaugh)
	if err != nil {
		t.Fatalf("SetCommentReaction laugh after vote: %v", err)
	}
	check(result, 2, true, ReactionLaugh, map[Reaction]int64{ReactionLaugh: 1})

	// Another reaction replaces the first without changing the score.
	result, err = database.SetCommentReaction(ctx, bob.ID, comment.ID, ReactionIdea)
	if err != nil {
		t.Fatalf("SetCommentReaction idea: %v", err)
	}
	check(result, 2, true, ReactionIdea, map[Reaction]int64{ReactionIdea: 1})

	// The vote toggle removes the reaction along with the vote.
	result, err = database.ToggleCommentVote(ctx, bob.ID, comment.ID)
	if err != nil {
		t.Fatalf("ToggleCommentVote with reaction: %v", err)
	}
	check(result, 1, false, "", nil)

	var rows int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM votes WHERE user_id = ? AND item_type = 'comment' AND item_id = ?`,
		bob.ID, comment.ID).Scan(&rows); err != nil {
		t.Fatalf("count vote rows: %v", err)
	}
	if rows != 0 {
		t.Errorf("vote rows after toggle = %d, want 0", rows)
	}
}

func TestSetCommentReactionGuards(t *testing.T) {
	database, alice, bob, _, comment := reactionFixture(t)
	ctx := context.Background()

	if _, err := database.SetCommentReaction(ctx, alice.ID, comment.ID, ReactionHeart); !errors.Is(err, ErrReactOwnComment) {
		t.Errorf("own comment reaction error = %v, want ErrReactOwnComment", err)
	}
	if _, err := database.SetCommentReaction(ctx, bob.ID, 9999, ReactionHeart); !errors.Is(err, ErrVoteNotFound) {
		t.Errorf("missing comment reaction error = %v, want ErrVoteNotFound", err)
	}
	if _, err := database.SetCommentReaction(ctx, bob.ID, comment.ID, Reaction("shrug")); !errors.Is(err, ErrInvalidReaction) {
		t.Errorf("unknown reaction error = %v, want ErrInvalidReaction", err)
	}
	assertCommentScore(t, database, comment.ID)

	if _, err := database.DeleteComment(ctx, comment.ID, time.Now()); err != nil {
		t.Fatalf("DeleteComment: %v", err)
	}
	if _, err := database.SetCommentReaction(ctx, bob.ID, comment.ID, ReactionHeart); !errors.Is(err, ErrVoteNotFound) {
		t.Errorf("deleted comment reaction error = %v, want ErrVoteNotFound", err)
	}
	assertCommentScore(t, database, comment.ID)
}

func TestThreadVotesHaveNoReaction(t *testing.T) {
	database, _, bob, thread, _ := reactionFixture(t)
	ctx := context.Background()

	result, err := database.ToggleThreadVote(ctx, bob.ID, thread.ID)
	if err != nil {
		t.Fatalf("ToggleThreadVote: %v", err)
	}
	if result.Reaction != "" || result.ReactionCounts != nil {
		t.Errorf("thread vote result reaction = %q, counts = %v; want empty and nil", result.Reaction, result.ReactionCounts)
	}

	var reaction sql.NullString
	if err := database.QueryRow(
		`SELECT reaction FROM votes WHERE user_id = ? AND item_type = 'thread' AND item_id = ?`,
		bob.ID, thread.ID).Scan(&reaction); err != nil {
		t.Fatalf("read thread vote reaction: %v", err)
	}
	if reaction.Valid {
		t.Errorf("thread vote reaction = %q, want NULL", reaction.String)
	}
}

func TestCommentQueriesIncludeReactions(t *testing.T) {
	database, alice, bob, thread, comment := reactionFixture(t)
	ctx := context.Background()

	carol, err := database.CreateUser(ctx, "carol", "hash", false, nil, time.Unix(1_700_000_000, 0).UTC())
	if err != nil {
		t.Fatalf("create carol: %v", err)
	}
	second, err := database.CreateComment(ctx, thread.ID, nil, alice.ID, "second", "<p>second</p>", time.Unix(1_700_000_100, 0).UTC())
	if err != nil {
		t.Fatalf("CreateComment second: %v", err)
	}

	for _, vote := range []struct {
		userID    int64
		commentID int64
		reaction  Reaction
	}{
		{bob.ID, comment.ID, ReactionHeart},
		{bob.ID, second.ID, ReactionLaugh},
		{carol.ID, comment.ID, ReactionHeart},
	} {
		if _, err := database.SetCommentReaction(ctx, vote.userID, vote.commentID, vote.reaction); err != nil {
			t.Fatalf("SetCommentReaction %d/%d: %v", vote.userID, vote.commentID, err)
		}
	}

	for _, test := range []struct {
		name       string
		viewer     int64
		first      Reaction
		wantFirst  map[Reaction]int64
		second     Reaction
		wantSecond map[Reaction]int64
	}{
		{
			name:       "reactor sees own reactions",
			viewer:     bob.ID,
			first:      ReactionHeart,
			wantFirst:  map[Reaction]int64{ReactionHeart: 2},
			second:     ReactionLaugh,
			wantSecond: map[Reaction]int64{ReactionLaugh: 1},
		},
		{
			name:       "other member sees counts but no reaction",
			viewer:     alice.ID,
			wantFirst:  map[Reaction]int64{ReactionHeart: 2},
			wantSecond: map[Reaction]int64{ReactionLaugh: 1},
		},
		{
			name:       "visitor sees counts but no reaction",
			viewer:     0,
			wantFirst:  map[Reaction]int64{ReactionHeart: 2},
			wantSecond: map[Reaction]int64{ReactionLaugh: 1},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			comments, err := database.ListComments(ctx, thread.ID, test.viewer)
			if err != nil || len(comments) != 2 {
				t.Fatalf("ListComments = %v, %v; want 2 comments", comments, err)
			}
			first := commentFromList(t, comments, comment.ID)
			if first.Reaction != test.first {
				t.Errorf("first comment reaction = %q, want %q", first.Reaction, test.first)
			}
			assertReactionCounts(t, first.ReactionCounts, test.wantFirst)

			other := commentFromList(t, comments, second.ID)
			if other.Reaction != test.second {
				t.Errorf("second comment reaction = %q, want %q", other.Reaction, test.second)
			}
			assertReactionCounts(t, other.ReactionCounts, test.wantSecond)
		})
	}

	got, err := database.CommentByID(ctx, second.ID, bob.ID)
	if err != nil || got == nil {
		t.Fatalf("CommentByID = %v, %v", got, err)
	}
	if got.Reaction != ReactionLaugh {
		t.Errorf("CommentByID reaction = %q, want %q", got.Reaction, ReactionLaugh)
	}
	assertReactionCounts(t, got.ReactionCounts, map[Reaction]int64{ReactionLaugh: 1})
}

func assertReactionCounts(t *testing.T, got, want map[Reaction]int64) {
	t.Helper()

	for _, reaction := range CommentReactions {
		if got[reaction] != want[reaction] {
			t.Errorf("reaction %q count = %d, want %d", reaction, got[reaction], want[reaction])
		}
	}
}

// assertCommentScore checks the invariant that a comment's score is the number
// of its vote rows, including the author's.
func assertCommentScore(t *testing.T, database *DB, commentID int64) {
	t.Helper()

	var score, votes int64
	if err := database.QueryRow(`SELECT score FROM comments WHERE id = ?`, commentID).Scan(&score); err != nil {
		t.Fatalf("read comment score: %v", err)
	}
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM votes WHERE item_type = 'comment' AND item_id = ?`,
		commentID).Scan(&votes); err != nil {
		t.Fatalf("count comment votes: %v", err)
	}
	if score != votes {
		t.Errorf("comment score = %d, want %d vote rows", score, votes)
	}
}

func commentFromList(t *testing.T, comments []Comment, id int64) Comment {
	t.Helper()

	for _, comment := range comments {
		if comment.ID == id {
			return comment
		}
	}
	t.Fatalf("comment %d not in list", id)
	return Comment{}
}
