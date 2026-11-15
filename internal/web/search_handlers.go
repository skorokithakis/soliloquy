package web

import (
	"html"
	"html/template"
	"net/http"
	"strings"

	"github.com/skorokithakis/soliloquy/internal/db"
)

const searchPerPage = 30

type searchResultView struct {
	Title          string
	Link           string
	ThreadLink     string
	KindLabel      string
	IsComment      bool
	Author         string
	AuthorUsername string
	AuthorDeleted  bool
	Age            string
	CommentText    string
	Snippet        template.HTML
}

type searchPageData struct {
	PageData
	Query      string
	Searched   bool
	Results    []searchResultView
	Total      int64
	Page       int
	TotalPages int
	HasPrev    bool
	HasNext    bool
	PrevPage   int
	NextPage   int
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	page := threadPageNumber(r)

	total, err := s.db.CountSearch(r.Context(), query)
	if err != nil {
		s.serverError(w, err)
		return
	}
	totalPages := int((total + searchPerPage - 1) / searchPerPage)
	if totalPages < 1 {
		totalPages = 1
	}
	// A page past the end would show the total with no results and no way
	// back, so show the last page instead of redirecting.
	if page > totalPages {
		page = totalPages
	}

	results, err := s.db.Search(r.Context(), query, searchPerPage+1, (page-1)*searchPerPage)
	if err != nil {
		s.serverError(w, err)
		return
	}

	hasNext := len(results) > searchPerPage
	if hasNext {
		results = results[:searchPerPage]
	}

	now := s.now()
	views := make([]searchResultView, len(results))
	for i, result := range results {
		threadLink := threadPath(result.ThreadPublicID)
		link := threadLink
		isComment := result.Kind == db.SearchKindComment
		if isComment {
			link = commentAnchorPath(result.ThreadPublicID, result.ItemPublicID)
		}

		author := result.Author
		if result.AuthorDeleted {
			author = "[deleted user]"
		}
		views[i] = searchResultView{
			Title:          result.Title,
			Link:           link,
			ThreadLink:     threadLink,
			KindLabel:      result.Kind,
			IsComment:      isComment,
			Author:         author,
			AuthorUsername: result.Author,
			AuthorDeleted:  result.AuthorDeleted,
			Age:            relativeTime(result.CreatedAt, now),
			CommentText:    commentsText(result.CommentCount),
			Snippet:        searchSnippetHTML(result.Snippet),
		}
	}

	data := searchPageData{
		PageData:   s.pageData(r, "Search"),
		Query:      query,
		Searched:   query != "",
		Results:    views,
		Total:      total,
		Page:       page,
		TotalPages: totalPages,
		HasPrev:    page > 1 && totalPages > 1,
		HasNext:    hasNext,
		PrevPage:   page - 1,
		NextPage:   page + 1,
	}
	data.SearchQuery = query
	data.Visitor = currentUser(r) == nil
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
