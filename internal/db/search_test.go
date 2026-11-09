package db

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSearchFindsThreadsAndComments(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()

	alice, err := database.CreateUser(ctx, "alice", "hash", false, nil, base)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	thread, err := database.CreateThread(ctx, alice.ID, "Go concurrency", "", "goroutines and channels", "<p>goroutines and channels</p>", base)
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	comment, err := database.CreateComment(ctx, thread.ID, nil, alice.ID, "a channel pipeline", "<p>a channel pipeline</p>", base.Add(time.Minute))
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}

	results, err := database.Search(ctx, "goroutines")
	if err != nil {
		t.Fatalf("Search goroutines: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("goroutines results = %d, want 1", len(results))
	}
	if results[0].Kind != SearchKindThread || results[0].ItemID != thread.ID || results[0].ThreadID != thread.ID {
		t.Errorf("thread result = %+v, want thread %d", results[0], thread.ID)
	}
	if results[0].Title != thread.Title {
		t.Errorf("thread result title = %q, want %q", results[0].Title, thread.Title)
	}
	if !strings.Contains(results[0].Snippet, SearchSnippetStart) {
		t.Errorf("thread snippet %q has no highlight marker", results[0].Snippet)
	}

	results, err = database.Search(ctx, "pipeline")
	if err != nil {
		t.Fatalf("Search pipeline: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("pipeline results = %d, want 1", len(results))
	}
	if results[0].Kind != SearchKindComment || results[0].ItemID != comment.ID || results[0].ThreadID != thread.ID {
		t.Errorf("comment result = %+v, want comment %d in thread %d", results[0], comment.ID, thread.ID)
	}
	if !strings.Contains(results[0].Snippet, SearchSnippetStart+"pipeline"+SearchSnippetEnd) {
		t.Errorf("comment snippet %q does not highlight the match", results[0].Snippet)
	}
}

func TestSearchIndexFollowsEditsAndDeletes(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()

	alice, err := database.CreateUser(ctx, "alice", "hash", false, nil, base)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	thread, err := database.CreateThread(ctx, alice.ID, "Original title", "", "original body", "<p>original body</p>", base)
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	comment, err := database.CreateComment(ctx, thread.ID, nil, alice.ID, "original comment", "<p>original comment</p>", base.Add(time.Minute))
	if err != nil {
		t.Fatalf("CreateComment: %v", err)
	}

	if got := searchIDs(t, database, "original"); len(got) != 2 {
		t.Fatalf("original results = %v, want the thread and comment", got)
	}

	if _, err := database.UpdateThread(ctx, thread.ID, alice.ID, "Revised title", "", "revised body", "<p>revised body</p>", base.Add(2*time.Minute)); err != nil {
		t.Fatalf("UpdateThread: %v", err)
	}
	if _, err := database.UpdateComment(ctx, comment.ID, alice.ID, "revised comment", "<p>revised comment</p>", base.Add(2*time.Minute)); err != nil {
		t.Fatalf("UpdateComment: %v", err)
	}

	if got := searchIDs(t, database, "original"); len(got) != 0 {
		t.Errorf("stale original results = %v, want none", got)
	}
	if got := searchIDs(t, database, "revised"); len(got) != 2 {
		t.Errorf("revised results = %v, want the thread and comment", got)
	}

	if _, err := database.DeleteComment(ctx, comment.ID, base.Add(3*time.Minute)); err != nil {
		t.Fatalf("DeleteComment: %v", err)
	}
	if got := searchIDs(t, database, "revised"); len(got) != 1 || got[0] != thread.ID {
		t.Errorf("results after comment delete = %v, want just thread %d", got, thread.ID)
	}

	if _, err := database.CreateComment(ctx, thread.ID, nil, alice.ID, "another revised comment", "", base.Add(4*time.Minute)); err != nil {
		t.Fatalf("CreateComment for delete test: %v", err)
	}
	if got := searchIDs(t, database, "revised"); len(got) != 2 {
		t.Fatalf("results before thread delete = %v, want thread and comment", got)
	}

	if _, err := database.DeleteThread(ctx, thread.ID, base.Add(5*time.Minute)); err != nil {
		t.Fatalf("DeleteThread: %v", err)
	}
	if got := searchIDs(t, database, "revised"); len(got) != 0 {
		t.Errorf("results after thread delete = %v, want none", got)
	}
}

func TestSearchTreatsInputAsLiteralTerms(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()

	alice, err := database.CreateUser(ctx, "alice", "hash", false, nil, base)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := database.CreateThread(ctx, alice.ID, "Syntax", "", "foo bar baz", "<p>foo bar baz</p>", base); err != nil {
		t.Fatalf("CreateThread: %v", err)
	}

	for _, query := range []string{
		`"`, `""`, `*`, `-`, `()`, `NEAR(`, `foo OR bar`, `foo"bar`, `foo:bar`, `^foo`, `foo*`,
	} {
		if _, err := database.Search(ctx, query); err != nil {
			t.Errorf("Search(%q) returned an error: %v", query, err)
		}
	}

	results, err := database.Search(ctx, "foo bar")
	if err != nil {
		t.Fatalf("Search foo bar: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("multiple terms did not AND together: %d results", len(results))
	}

	results, err = database.Search(ctx, `"foo bar"`)
	if err != nil {
		t.Fatalf("Search quoted phrase: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("quoted phrase was treated as syntax instead of literal terms: %d results", len(results))
	}

	for _, query := range []string{"", "   "} {
		results, err := database.Search(ctx, query)
		if err != nil {
			t.Errorf("Search(%q) returned an error: %v", query, err)
		}
		if len(results) != 0 {
			t.Errorf("Search(%q) returned %d results, want none", query, len(results))
		}
	}
}

func TestSearchLimitsResults(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()

	alice, err := database.CreateUser(ctx, "alice", "hash", false, nil, base)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	for i := 0; i < maxSearchResults+5; i++ {
		title := "needle thread " + string(rune('a'+i%26))
		if _, err := database.CreateThread(ctx, alice.ID, title, "", "needle body", "", base.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatalf("CreateThread %d: %v", i, err)
		}
	}

	results, err := database.Search(ctx, "needle")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != maxSearchResults {
		t.Errorf("results = %d, want %d", len(results), maxSearchResults)
	}
}

func TestSearchRanksTitleMatchesFirst(t *testing.T) {
	database := migratedTestDB(t)
	ctx := context.Background()
	base := time.Unix(1_700_000_000, 0).UTC()

	alice, err := database.CreateUser(ctx, "alice", "hash", false, nil, base)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	titled, err := database.CreateThread(ctx, alice.ID, "needle", "", "unrelated body", "", base)
	if err != nil {
		t.Fatalf("CreateThread titled: %v", err)
	}
	bodied, err := database.CreateThread(ctx, alice.ID, "unrelated title", "", "needle in body", "", base.Add(time.Minute))
	if err != nil {
		t.Fatalf("CreateThread bodied: %v", err)
	}

	results, err := database.Search(ctx, "needle")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	if results[0].ItemID != titled.ID || results[1].ItemID != bodied.ID {
		t.Errorf("result order = [%d %d], want title match %d first", results[0].ItemID, results[1].ItemID, titled.ID)
	}
}

func searchIDs(t *testing.T, database *DB, query string) []int64 {
	t.Helper()

	results, err := database.Search(context.Background(), query)
	if err != nil {
		t.Fatalf("Search(%q): %v", query, err)
	}

	ids := make([]int64, len(results))
	for i, result := range results {
		ids[i] = result.ItemID
	}
	return ids
}
