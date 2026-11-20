package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/skorokithakis/soliloquy/internal/auth"
	"github.com/skorokithakis/soliloquy/internal/db"
)

func TestSeedIsIdempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "qa.db")

	database, err := db.Open(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	now := time.Date(2026, time.November, 20, 12, 0, 0, 0, time.UTC)
	seeded, err := seed(ctx, database, now)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if !seeded {
		t.Fatal("seed did not write anything to an empty database")
	}

	users, err := database.ListUsers(ctx)
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(users) != len(qaUsers) {
		t.Errorf("seeded %d users, want %d", len(users), len(qaUsers))
	}
	admin, err := database.UserByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("look up admin: %v", err)
	}
	if admin == nil || !admin.IsAdmin {
		t.Fatal("admin account was not seeded as an admin")
	}
	if !auth.CheckPassword(admin.PasswordHash, "admin") {
		t.Error("admin password does not match")
	}

	before := snapshot(t, database)
	if before.threads != len(qaThreads()) {
		t.Errorf("seeded %d threads, want %d", before.threads, len(qaThreads()))
	}

	seeded, err = seed(ctx, database, now.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if seeded {
		t.Error("second seed wrote data into a database that already had threads")
	}

	after := snapshot(t, database)
	if after != before {
		t.Errorf("second seed changed the database: before %+v, after %+v", before, after)
	}
}

func TestSeedSkipsDatabaseWithOnlyDeletedThread(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "qa.db")

	database, err := db.Open(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	now := time.Date(2026, time.November, 20, 12, 0, 0, 0, time.UTC)
	hash, err := auth.HashPassword("password")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user, err := database.CreateUser(ctx, "existing", hash, false, nil, now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	thread, err := database.CreateThread(ctx, user.ID, "Deleted thread", "", "body", "", now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	deleted, err := database.DeleteThread(ctx, thread.ID, now)
	if err != nil {
		t.Fatalf("delete thread: %v", err)
	}
	if !deleted {
		t.Fatal("thread was not deleted")
	}

	before := snapshot(t, database)
	seeded, err := seed(ctx, database, now.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if seeded {
		t.Error("seed wrote data into a database whose only thread was deleted")
	}

	after := snapshot(t, database)
	if after != before {
		t.Errorf("seed changed a database with a deleted thread: before %+v, after %+v", before, after)
	}
}

// TestSeedDeepThread checks the shapes the single-comment page needs: a deep
// reply chain, long bodies at shallow, middle and deep levels, a deleted comment
// with live replies, a deleted author, and both a leaf and a reply tree at
// depth 12.
func TestSeedDeepThread(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "qa.db")

	database, err := db.Open(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	now := time.Date(2026, time.November, 20, 12, 0, 0, 0, time.UTC)
	if _, err := seed(ctx, database, now); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var threadID int64
	if err := database.QueryRow(
		`SELECT id FROM threads WHERE title = ?`, deepThreadTitle,
	).Scan(&threadID); err != nil {
		t.Fatalf("find deep thread %q: %v", deepThreadTitle, err)
	}

	comments, err := database.ListComments(ctx, threadID, 0)
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}

	const (
		longBodyWords     = 400
		veryLongBodyWords = 1000
	)
	depth := make(map[int64]int, len(comments))
	children := make(map[int64][]int64, len(comments))
	longDepths := make(map[int]int)
	veryLongDepths := make(map[int]int)
	maxDepth := 0
	longComments := 0
	veryLongComments := 0
	longTopLevel := false
	longOneParent := false
	deletedWithLiveReply := false
	deletedUserComment := false

	// ListComments returns comments oldest first, so a parent's depth is
	// already known when its replies are visited.
	for _, comment := range comments {
		if comment.ParentID == nil {
			depth[comment.ID] = 1
		} else {
			depth[comment.ID] = depth[*comment.ParentID] + 1
			children[*comment.ParentID] = append(children[*comment.ParentID], comment.ID)
		}

		if depth[comment.ID] > maxDepth {
			maxDepth = depth[comment.ID]
		}

		if comment.DeletedAt != nil {
			for _, reply := range comments {
				if reply.ParentID != nil && *reply.ParentID == comment.ID && reply.DeletedAt == nil {
					deletedWithLiveReply = true
				}
			}
		}
		if comment.AuthorDeletedAt != nil {
			deletedUserComment = true
			if comment.AuthorName() != "[deleted user]" {
				t.Errorf("comment by a deleted account shows name %q", comment.AuthorName())
			}
		}

		words := len(strings.Fields(comment.Body))
		if words >= veryLongBodyWords {
			veryLongComments++
			veryLongDepths[depth[comment.ID]]++
		}
		if words < longBodyWords {
			continue
		}
		longComments++
		longDepths[depth[comment.ID]]++
		if comment.ParentID == nil {
			longTopLevel = true
		}
		if depth[comment.ID] == 2 {
			longOneParent = true
		}
	}

	t.Logf("deep thread: %d comments, max depth %d, long comments %d at depths %v, very long comments %d at depths %v",
		len(comments), maxDepth, longComments, longDepths, veryLongComments, veryLongDepths)

	if maxDepth < 14 {
		t.Errorf("deepest comment is at depth %d, want at least 14", maxDepth)
	}
	if longComments < 6 {
		t.Errorf("found %d long comments, want at least 6", longComments)
	}
	if veryLongComments < 2 {
		t.Errorf("found %d very long comments, want at least 2", veryLongComments)
	}
	if veryLongDepths[8] == 0 {
		t.Error("no very long comment at depth 8, so the middle of the chain has no long parent")
	}
	if veryLongDepths[15] == 0 {
		t.Error("no very long comment at depth 15, so the deepest long comment is not very long")
	}
	for _, band := range []struct {
		name     string
		min, max int
	}{
		{"shallow", 1, 3},
		{"middle", 4, 10},
		{"deep", 13, maxDepth},
	} {
		found := false
		for d := band.min; d <= band.max; d++ {
			if longDepths[d] > 0 {
				found = true
			}
		}
		if !found {
			t.Errorf("no long comment at a %s depth (%d-%d), depths are %v", band.name, band.min, band.max, longDepths)
		}
	}
	if !longTopLevel {
		t.Error("no long top-level comment, which the single-comment page needs for the no-parents state")
	}
	if !longOneParent {
		t.Error("no long comment at depth 2, which the single-comment page needs for the one-parent state")
	}
	if !deletedWithLiveReply {
		t.Error("no deleted comment has a live reply, which the single-comment page needs for a deleted parent")
	}
	if !deletedUserComment {
		t.Error("no comment was written by a deleted account")
	}

	// At depth 12 the chain has one long leaf and one long comment with a
	// small reply tree, so both branches exercise the context area.
	var longLeafAtDepthTwelve, longTreeAtDepthTwelve bool
	for _, comment := range comments {
		if depth[comment.ID] != 12 || len(strings.Fields(comment.Body)) < longBodyWords {
			continue
		}
		grandchildren := 0
		for _, child := range children[comment.ID] {
			grandchildren += len(children[child])
		}
		if len(children[comment.ID]) == 0 {
			longLeafAtDepthTwelve = true
		}
		if len(children[comment.ID]) >= 2 && grandchildren > 0 {
			longTreeAtDepthTwelve = true
		}
	}
	if !longLeafAtDepthTwelve {
		t.Error("no long comment at depth 12 is a leaf")
	}
	if !longTreeAtDepthTwelve {
		t.Error("no long comment at depth 12 has a reply tree")
	}

	ghost, err := database.UserByUsername(ctx, "ghost")
	if err != nil {
		t.Fatalf("look up ghost: %v", err)
	}
	if ghost == nil || ghost.DeletedAt == nil {
		t.Error("ghost account was not deleted after seeding")
	}
}

type counts struct {
	users    int
	threads  int
	comments int
	votes    int
}

func snapshot(t *testing.T, database *db.DB) counts {
	t.Helper()

	var c counts
	for _, query := range []struct {
		name string
		sql  string
		dest *int
	}{
		{"users", `SELECT COUNT(*) FROM users`, &c.users},
		{"threads", `SELECT COUNT(*) FROM threads`, &c.threads},
		{"comments", `SELECT COUNT(*) FROM comments`, &c.comments},
		{"votes", `SELECT COUNT(*) FROM votes`, &c.votes},
	} {
		if err := database.QueryRow(query.sql).Scan(query.dest); err != nil {
			t.Fatalf("count %s: %v", query.name, err)
		}
	}
	return c
}
