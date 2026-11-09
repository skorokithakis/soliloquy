package web

import (
	"html"
	"html/template"
	"net/http"
	"strings"

	"github.com/skorokithakis/soliloquy/internal/db"
)

type searchResultView struct {
	Title         string
	Link          string
	ThreadLink    string
	KindLabel     string
	IsComment     bool
	Author        string
	AuthorDeleted bool
	Age           string
	CommentText   string
	Snippet       template.HTML
}

type searchPageData struct {
	PageData
	Query    string
	Searched bool
	Results  []searchResultView
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))

	results, err := s.db.Search(r.Context(), query)
	if err != nil {
		s.serverError(w, err)
		return
	}

	now := s.now()
	views := make([]searchResultView, len(results))
	for i, result := range results {
		threadLink := threadPath(result.ThreadID)
		link := threadLink
		isComment := result.Kind == db.SearchKindComment
		if isComment {
			link = commentAnchorPath(result.ThreadID, result.ItemID)
		}

		author := result.Author
		if result.AuthorDeleted {
			author = "[deleted user]"
		}
		views[i] = searchResultView{
			Title:         result.Title,
			Link:          link,
			ThreadLink:    threadLink,
			KindLabel:     result.Kind,
			IsComment:     isComment,
			Author:        author,
			AuthorDeleted: result.AuthorDeleted,
			Age:           relativeTime(result.CreatedAt, now),
			CommentText:   commentsText(result.CommentCount),
			Snippet:       searchSnippetHTML(result.Snippet),
		}
	}

	data := searchPageData{
		PageData: s.pageData(r, "Search"),
		Query:    query,
		Searched: query != "",
		Results:  views,
	}
	data.SearchQuery = query
	s.render(w, "search", data)
}

// searchSnippetHTML escapes a snippet from the database and then turns the
// marker characters into <mark> tags. Escaping happens first, so text from a
// post cannot inject markup.
func searchSnippetHTML(snippet string) template.HTML {
	escaped := html.EscapeString(snippet)
	escaped = strings.ReplaceAll(escaped, db.SearchSnippetStart, "<mark>")
	escaped = strings.ReplaceAll(escaped, db.SearchSnippetEnd, "</mark>")
	return template.HTML(escaped)
}
