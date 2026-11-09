package db

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBuildCommentTreeNestsAndSorts(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	parentID := int64(1)

	comments := []Comment{
		{ID: 1, Score: 1, CreatedAt: base.Add(3 * time.Minute)},
		{ID: 2, Score: 5, CreatedAt: base.Add(4 * time.Minute)},
		{ID: 3, ParentID: &parentID, Score: 1, CreatedAt: base.Add(1 * time.Minute)},
		{ID: 4, ParentID: &parentID, Score: 3, CreatedAt: base.Add(2 * time.Minute)},
		{ID: 5, ParentID: &parentID, Score: 3, CreatedAt: base.Add(90 * time.Second)},
	}

	tree := BuildCommentTree(comments)

	if len(tree) != 2 || tree[0].Comment.ID != 2 || tree[1].Comment.ID != 1 {
		t.Fatalf("root order = %v, want [2 1]", nodeIDs(tree))
	}
	if got := nodeIDs(tree[1].Replies); len(got) != 3 || got[0] != 5 || got[1] != 4 || got[2] != 3 {
		t.Errorf("reply order = %v, want [5 4 3]", got)
	}
}

func TestBuildCommentTreeHidesDeletedWithoutVisibleDescendants(t *testing.T) {
	parentID := int64(1)
	deleted := time.Unix(1_700_000_000, 0).UTC()

	comments := []Comment{
		// Deleted with a live child: kept as [deleted].
		{ID: 1, DeletedAt: &deleted},
		{ID: 2, ParentID: &parentID},
		// Deleted leaf: hidden.
		{ID: 3, DeletedAt: &deleted},
		// Deleted with only a deleted child: both hidden.
		{ID: 4, DeletedAt: &deleted},
		{ID: 5, ParentID: int64Ptr(4), DeletedAt: &deleted},
		// Live root with a deleted child that has no replies: child hidden.
		{ID: 6},
		{ID: 7, ParentID: int64Ptr(6), DeletedAt: &deleted},
	}

	tree := BuildCommentTree(comments)

	if got := nodeIDs(tree); len(got) != 2 || got[0] != 1 || got[1] != 6 {
		t.Fatalf("root IDs = %v, want [1 6]", got)
	}
	if got := nodeIDs(tree[0].Replies); len(got) != 1 || got[0] != 2 {
		t.Errorf("kept deleted comment replies = %v, want [2]", got)
	}
	if len(tree[1].Replies) != 0 {
		t.Errorf("live root kept hidden deleted child: %v", nodeIDs(tree[1].Replies))
	}
}

func TestBuildCommentTreeKeepsDeletedChainToLiveReply(t *testing.T) {
	deleted := time.Unix(1_700_000_000, 0).UTC()

	comments := []Comment{
		{ID: 1, DeletedAt: &deleted},
		{ID: 2, ParentID: int64Ptr(1), DeletedAt: &deleted},
		{ID: 3, ParentID: int64Ptr(2)},
	}

	tree := BuildCommentTree(comments)

	if len(tree) != 1 || tree[0].Comment.ID != 1 {
		t.Fatalf("root IDs = %v, want [1]", nodeIDs(tree))
	}
	if len(tree[0].Replies) != 1 || tree[0].Replies[0].Comment.ID != 2 {
		t.Fatalf("first deleted child = %v, want [2]", nodeIDs(tree[0].Replies))
	}
	if len(tree[0].Replies[0].Replies) != 1 || tree[0].Replies[0].Replies[0].Comment.ID != 3 {
		t.Errorf("live descendant = %v, want [3]", nodeIDs(tree[0].Replies[0].Replies))
	}
}

func TestCommentLifecycleUpdatesThread(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()

	alice, err := database.CreateUser(ctx, "alice", "hash", false, nil, base)
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}
	bob, err := database.CreateUser(ctx, "bob", "hash", false, nil, base)
	if err != nil {
		t.Fatalf("create bob: %v", err)
	}
	thread, err := database.CreateThread(ctx, alice.ID, "Thread", "", "body", "<p>body</p>", base)
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}

	root, err := database.CreateComment(ctx, thread.ID, nil, bob.ID, "root", "<p>root</p>", base.Add(time.Minute))
	if err != nil {
		t.Fatalf("CreateComment root: %v", err)
	}
	reply, err := database.CreateComment(ctx, thread.ID, &root.ID, alice.ID, "reply", "<p>reply</p>", base.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("CreateComment reply: %v", err)
	}
	if root.Score != 1 || reply.Score != 1 {
		t.Errorf("new comment scores = %d/%d, want 1/1", root.Score, reply.Score)
	}

	got, err := database.ThreadByID(ctx, thread.ID)
	if err != nil || got == nil {
		t.Fatalf("ThreadByID: thread = %v, err = %v", got, err)
	}
	if !got.LastActivityAt.Equal(base.Add(2 * time.Minute)) {
		t.Errorf("LastActivityAt = %v, want reply time", got.LastActivityAt)
	}
	if got.CommentCount != 2 {
		t.Errorf("CommentCount = %d, want 2", got.CommentCount)
	}

	comments, err := database.ListComments(ctx, thread.ID, 0)
	if err != nil || len(comments) != 2 {
		t.Fatalf("ListComments = %v, %v; want 2 comments", comments, err)
	}

	if deleted, err := database.DeleteComment(ctx, root.ID, base.Add(3*time.Minute)); err != nil || !deleted {
		t.Fatalf("DeleteComment = %t, %v; want true, nil", deleted, err)
	}

	got, err = database.ThreadByID(ctx, thread.ID)
	if err != nil || got == nil {
		t.Fatalf("ThreadByID after delete: thread = %v, err = %v", got, err)
	}
	if got.CommentCount != 1 {
		t.Errorf("CommentCount after delete = %d, want 1", got.CommentCount)
	}

	comments, err = database.ListComments(ctx, thread.ID, 0)
	if err != nil || len(comments) != 2 {
		t.Fatalf("ListComments after delete: %d comments, err = %v; want 2 rows", len(comments), err)
	}
}

func TestCreateCommentRequiresLiveThreadAndParent(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	alice, _ := database.CreateUser(ctx, "alice", "hash", false, nil, now)
	bob, _ := database.CreateUser(ctx, "bob", "hash", false, nil, now)
	thread, err := database.CreateThread(ctx, alice.ID, "Thread", "", "body", "<p>body</p>", now)
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	otherThread, err := database.CreateThread(ctx, alice.ID, "Other", "", "body", "<p>body</p>", now)
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}

	root, err := database.CreateComment(ctx, thread.ID, nil, alice.ID, "root", "<p>root</p>", now)
	if err != nil {
		t.Fatalf("CreateComment root: %v", err)
	}
	parentInOtherThread, err := database.CreateComment(ctx, otherThread.ID, nil, alice.ID, "other root", "<p>other root</p>", now)
	if err != nil {
		t.Fatalf("CreateComment other thread root: %v", err)
	}

	if _, err := database.CreateComment(ctx, thread.ID, &parentInOtherThread.ID, bob.ID, "cross-thread", "<p>cross-thread</p>", now); !errors.Is(err, ErrCommentNotFound) {
		t.Errorf("reply to a parent in another thread error = %v, want ErrCommentNotFound", err)
	}
	if _, err := database.CreateComment(ctx, 9999, nil, bob.ID, "missing thread", "<p>missing thread</p>", now); !errors.Is(err, ErrCommentNotFound) {
		t.Errorf("comment on a missing thread error = %v, want ErrCommentNotFound", err)
	}

	if _, err := database.DeleteComment(ctx, root.ID, now); err != nil {
		t.Fatalf("DeleteComment root: %v", err)
	}
	if _, err := database.CreateComment(ctx, thread.ID, &root.ID, bob.ID, "deleted parent", "<p>deleted parent</p>", now); !errors.Is(err, ErrCommentNotFound) {
		t.Errorf("reply to a deleted parent error = %v, want ErrCommentNotFound", err)
	}

	if _, err := database.DeleteThread(ctx, thread.ID, now); err != nil {
		t.Fatalf("DeleteThread: %v", err)
	}
	if _, err := database.CreateComment(ctx, thread.ID, nil, bob.ID, "deleted thread", "<p>deleted thread</p>", now); !errors.Is(err, ErrCommentNotFound) {
		t.Errorf("comment on a deleted thread error = %v, want ErrCommentNotFound", err)
	}

	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM comments`).Scan(&count); err != nil {
		t.Fatalf("count comments: %v", err)
	}
	if count != 2 {
		t.Errorf("comments = %d, want 2; rejected inserts should not store rows", count)
	}
}

func TestUpdateCommentRequiresAuthor(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	alice, _ := database.CreateUser(ctx, "alice", "hash", false, nil, now)
	bob, _ := database.CreateUser(ctx, "bob", "hash", false, nil, now)
	thread, _ := database.CreateThread(ctx, alice.ID, "Thread", "", "body", "<p>body</p>", now)
	comment, err := database.CreateComment(ctx, thread.ID, nil, alice.ID, "old", "<p>old</p>", now)
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}

	editedAt := now.Add(time.Hour)
	if updated, err := database.UpdateComment(ctx, comment.ID, bob.ID, "hijacked", "<p>hijacked</p>", editedAt); err != nil || updated {
		t.Fatalf("UpdateComment as bob = %t, %v; want false, nil", updated, err)
	}

	if updated, err := database.UpdateComment(ctx, comment.ID, alice.ID, "new", "<p>new</p>", editedAt); err != nil || !updated {
		t.Fatalf("UpdateComment as alice = %t, %v; want true, nil", updated, err)
	}

	got, err := database.CommentByID(ctx, comment.ID, 0)
	if err != nil || got == nil {
		t.Fatalf("CommentByID: comment = %v, err = %v", got, err)
	}
	if got.Body != "new" || got.BodyHTML != "<p>new</p>" || got.EditedAt == nil {
		t.Errorf("updated comment = %+v", got)
	}

	if deleted, err := database.DeleteComment(ctx, comment.ID, editedAt); err != nil || !deleted {
		t.Fatalf("DeleteComment = %t, %v", deleted, err)
	}
	if deleted, err := database.DeleteComment(ctx, comment.ID, editedAt); err != nil || deleted {
		t.Errorf("second DeleteComment = %t, %v; want false, nil", deleted, err)
	}
	if updated, err := database.UpdateComment(ctx, comment.ID, alice.ID, "again", "<p>again</p>", editedAt); err != nil || updated {
		t.Errorf("UpdateComment after delete = %t, %v; want false, nil", updated, err)
	}
}

func TestCommentDeletedUserAuthorName(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	alice, err := database.CreateUser(ctx, "alice", "hash", false, nil, now)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	thread, err := database.CreateThread(ctx, alice.ID, "Thread", "", "body", "<p>body</p>", now)
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	comment, err := database.CreateComment(ctx, thread.ID, nil, alice.ID, "body", "<p>body</p>", now)
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}

	if _, err := database.ExecContext(ctx, `UPDATE users SET deleted_at = ? WHERE id = ?`, now.Unix(), alice.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	got, err := database.CommentByID(ctx, comment.ID, 0)
	if err != nil || got == nil {
		t.Fatalf("CommentByID: comment = %v, err = %v", got, err)
	}
	if got.AuthorName() != "[deleted user]" {
		t.Errorf("AuthorName = %q, want [deleted user]", got.AuthorName())
	}
}

func TestVoteToggleAndRules(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	alice, _ := database.CreateUser(ctx, "alice", "hash", false, nil, now)
	bob, _ := database.CreateUser(ctx, "bob", "hash", false, nil, now)
	thread, err := database.CreateThread(ctx, alice.ID, "Thread", "", "body", "<p>body</p>", now)
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	comment, err := database.CreateComment(ctx, thread.ID, nil, alice.ID, "body", "<p>body</p>", now)
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}

	if _, err := database.ToggleThreadVote(ctx, alice.ID, thread.ID); !errors.Is(err, ErrVoteOwnPost) {
		t.Fatalf("self thread vote error = %v, want ErrVoteOwnPost", err)
	}
	if _, err := database.ToggleCommentVote(ctx, alice.ID, comment.ID); !errors.Is(err, ErrVoteOwnPost) {
		t.Fatalf("self comment vote error = %v, want ErrVoteOwnPost", err)
	}

	result, err := database.ToggleThreadVote(ctx, bob.ID, thread.ID)
	if err != nil {
		t.Fatalf("ToggleThreadVote: %v", err)
	}
	if result.Score != 2 || !result.Voted {
		t.Errorf("vote result = %+v, want score 2, voted", result)
	}
	if voted, err := database.ThreadVoted(ctx, bob.ID, thread.ID); err != nil || !voted {
		t.Errorf("ThreadVoted = %t, %v; want true, nil", voted, err)
	}

	result, err = database.ToggleThreadVote(ctx, bob.ID, thread.ID)
	if err != nil {
		t.Fatalf("ToggleThreadVote off: %v", err)
	}
	if result.Score != 1 || result.Voted {
		t.Errorf("unvote result = %+v, want score 1, not voted", result)
	}

	result, err = database.ToggleCommentVote(ctx, bob.ID, comment.ID)
	if err != nil {
		t.Fatalf("ToggleCommentVote: %v", err)
	}
	if result.Score != 2 || !result.Voted {
		t.Errorf("comment vote result = %+v, want score 2, voted", result)
	}

	if _, err := database.ToggleThreadVote(ctx, bob.ID, 9999); !errors.Is(err, ErrVoteNotFound) {
		t.Errorf("missing thread vote error = %v, want ErrVoteNotFound", err)
	}

	if _, err := database.DeleteComment(ctx, comment.ID, now); err != nil {
		t.Fatalf("DeleteComment: %v", err)
	}
	if _, err := database.ToggleCommentVote(ctx, bob.ID, comment.ID); !errors.Is(err, ErrVoteNotFound) {
		t.Errorf("deleted comment vote error = %v, want ErrVoteNotFound", err)
	}
}

func TestListCommentsIncludesViewerVotes(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	alice, _ := database.CreateUser(ctx, "alice", "hash", false, nil, now)
	bob, _ := database.CreateUser(ctx, "bob", "hash", false, nil, now)
	thread, _ := database.CreateThread(ctx, alice.ID, "Thread", "", "body", "<p>body</p>", now)
	comment, err := database.CreateComment(ctx, thread.ID, nil, alice.ID, "body", "<p>body</p>", now)
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}
	if _, err := database.ToggleCommentVote(ctx, bob.ID, comment.ID); err != nil {
		t.Fatalf("ToggleCommentVote: %v", err)
	}

	for _, test := range []struct {
		viewer int64
		want   bool
	}{
		{viewer: alice.ID, want: false},
		{viewer: bob.ID, want: true},
		{viewer: 0, want: false},
	} {
		comments, err := database.ListComments(ctx, thread.ID, test.viewer)
		if err != nil || len(comments) != 1 {
			t.Fatalf("ListComments viewer %d = %v, %v", test.viewer, comments, err)
		}
		if comments[0].Voted != test.want {
			t.Errorf("viewer %d Voted = %t, want %t", test.viewer, comments[0].Voted, test.want)
		}
	}
}

func nodeIDs(nodes []*CommentNode) []int64 {
	ids := make([]int64, len(nodes))
	for i, node := range nodes {
		ids[i] = node.Comment.ID
	}
	return ids
}

func int64Ptr(value int64) *int64 {
	return &value
}
