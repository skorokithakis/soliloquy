package db

import (
	"context"
	"strings"
)

// Search result kinds.
const (
	SearchKindThread  = "thread"
	SearchKindComment = "comment"
)

// Search snippet highlight markers. The database returns snippets with these
// control characters around matching terms so the caller can HTML-escape the
// whole snippet and then turn only the markers into markup.
const (
	SearchSnippetStart = "\x01"
	SearchSnippetEnd   = "\x02"
)

// maxSearchResults is the fixed result limit for a search. The product does
// not paginate search results.
const maxSearchResults = 50

// SearchResult is one matching thread or comment. Title is the thread title for
// both kinds, so comment results can show which thread they belong to. Snippet
// contains Markdown source with SearchSnippetStart and SearchSnippetEnd around
// matching terms.
type SearchResult struct {
	Kind     string
	ThreadID int64
	ItemID   int64
	Title    string
	Snippet  string
}

// Search returns up to maxSearchResults matches for query, best first. Deleted
// comments and live content in deleted threads are excluded.
func (db *DB) Search(ctx context.Context, query string) ([]SearchResult, error) {
	match := searchMatchQuery(query)
	if match == "" {
		return []SearchResult{}, nil
	}

	rows, err := db.QueryContext(ctx, `
		SELECT search_index.kind, search_index.item_id, search_index.thread_id,
			t.title,
			snippet(search_index, 4, char(1), char(2), '…', 20)
		FROM search_index
		JOIN threads t ON t.id = search_index.thread_id
		LEFT JOIN comments c
			ON search_index.kind = 'comment' AND c.id = search_index.item_id
		WHERE search_index MATCH ?
			AND t.deleted_at IS NULL
			AND (search_index.kind = 'thread' OR c.deleted_at IS NULL)
		ORDER BY bm25(search_index, 0.0, 0.0, 0.0, 10.0, 1.0)
		LIMIT ?`, match, maxSearchResults)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := []SearchResult{}
	for rows.Next() {
		var result SearchResult
		if err := rows.Scan(&result.Kind, &result.ItemID, &result.ThreadID, &result.Title, &result.Snippet); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, rows.Err()
}

// searchMatchQuery turns user input into a safe FTS5 MATCH expression. Each
// whitespace-separated term is quoted and terms are joined with AND, so FTS5
// operators and syntax characters in the input are treated as literal text.
// An input with no terms produces an empty expression.
func searchMatchQuery(input string) string {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return ""
	}

	terms := make([]string, 0, len(fields))
	for _, field := range fields {
		terms = append(terms, `"`+strings.ReplaceAll(field, `"`, `""`)+`"`)
	}
	return strings.Join(terms, " AND ")
}
