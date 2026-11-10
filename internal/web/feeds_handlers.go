package web

import (
	"encoding/xml"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/skorokithakis/soliloquy/internal/db"
)

// feedItemLimit is how many of the newest threads or comments a feed carries.
// The feeds deliberately have no paging.
const feedItemLimit = 50

// rssDCNamespace is the Dublin Core namespace declared by the feeds for
// dc:creator, the element that holds the author's display name.
const rssDCNamespace = "http://purl.org/dc/elements/1.1/"

// RSS 2.0 feed types. encoding/xml escapes the item descriptions, so readers
// receive the stored, sanitized HTML as text and render it themselves.
type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	XmlnsDC string     `xml:"xmlns:dc,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Items       []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string  `xml:"title"`
	Link        string  `xml:"link"`
	GUID        rssGUID `xml:"guid"`
	PubDate     string  `xml:"pubDate"`
	Creator     string  `xml:"dc:creator"`
	Description string  `xml:"description"`
}

type rssGUID struct {
	Value       string `xml:",chardata"`
	IsPermaLink string `xml:"isPermaLink,attr"`
}

func (s *Server) handleThreadsFeed(w http.ResponseWriter, r *http.Request) {
	settings, err := s.db.Settings(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}

	threads, err := s.db.ListFeedThreads(r.Context(), feedItemLimit)
	if err != nil {
		s.serverError(w, err)
		return
	}

	items := make([]rssItem, 0, len(threads))
	for _, thread := range threads {
		link := s.absoluteURL(threadPath(thread.PublicID))
		items = append(items, rssItem{
			Title:       thread.Title,
			Link:        link,
			GUID:        rssGUID{Value: link, IsPermaLink: "true"},
			PubDate:     rssTime(thread.CreatedAt),
			Creator:     thread.AuthorName(),
			Description: threadFeedDescription(thread),
		})
	}

	s.writeFeed(w, rssFeed{
		Version: "2.0",
		XmlnsDC: rssDCNamespace,
		Channel: rssChannel{
			Title:       settings.ForumName + " threads",
			Link:        s.cfg.BaseURL + "/",
			Description: "New threads on " + settings.ForumName,
			Items:       items,
		},
	})
}

func (s *Server) handleCommentsFeed(w http.ResponseWriter, r *http.Request) {
	settings, err := s.db.Settings(r.Context())
	if err != nil {
		s.serverError(w, err)
		return
	}

	comments, err := s.db.ListFeedComments(r.Context(), feedItemLimit)
	if err != nil {
		s.serverError(w, err)
		return
	}

	items := make([]rssItem, 0, len(comments))
	for _, comment := range comments {
		link := s.absoluteURL(commentAnchorPath(comment.ThreadPublicID, comment.PublicID))
		items = append(items, rssItem{
			Title:       comment.AuthorName() + " on " + comment.ThreadTitle,
			Link:        link,
			GUID:        rssGUID{Value: link, IsPermaLink: "true"},
			PubDate:     rssTime(comment.CreatedAt),
			Creator:     comment.AuthorName(),
			Description: comment.BodyHTML,
		})
	}

	s.writeFeed(w, rssFeed{
		Version: "2.0",
		XmlnsDC: rssDCNamespace,
		Channel: rssChannel{
			Title:       settings.ForumName + " comments",
			Link:        s.cfg.BaseURL + "/",
			Description: "New comments on " + settings.ForumName,
			Items:       items,
		},
	})
}

// threadFeedDescription is the thread body for the feed, with a link to the
// thread's external URL first when there is one. The body HTML was sanitized
// when the thread was saved.
func threadFeedDescription(thread db.Thread) string {
	if thread.URL == "" {
		return thread.BodyHTML
	}
	// The URL was validated when the thread was saved; escape it anyway
	// because it is about to go into an href.
	escaped := html.EscapeString(thread.URL)
	return fmt.Sprintf(`<p><a href="%s">%s</a></p>`, escaped, escaped) + thread.BodyHTML
}

func rssTime(t time.Time) string {
	return t.UTC().Format(time.RFC1123Z)
}

// absoluteURL turns a site-relative path into the absolute URL feed readers
// need, using the configured base URL.
func (s *Server) absoluteURL(path string) string {
	return s.cfg.BaseURL + path
}

// feedURL is the absolute feed URL, tokenized when a token is given.
func (s *Server) feedURL(path, token string) string {
	feed := s.absoluteURL(path)
	if token == "" {
		return feed
	}
	return feed + "?token=" + url.QueryEscape(token)
}

// feedURLs returns the feed URLs the layout advertises. A user with a token
// always gets tokenized URLs, even on a public forum. On a public forum a
// visitor, or a logged-in user who has no token yet, gets the plain URLs.
// Otherwise there are no URLs to advertise.
func (s *Server) feedURLs(user *db.User, public bool) (threads, comments string) {
	if user != nil && user.FeedToken != "" {
		return s.feedURL("/feeds/threads", user.FeedToken), s.feedURL("/feeds/comments", user.FeedToken)
	}
	if public {
		return s.feedURL("/feeds/threads", ""), s.feedURL("/feeds/comments", "")
	}
	return "", ""
}

func (s *Server) writeFeed(w http.ResponseWriter, feed rssFeed) {
	body, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		s.serverError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	if _, err := w.Write([]byte(xml.Header + string(body) + "\n")); err != nil {
		slog.Error("write feed", "error", err)
	}
}

// feedPath reports whether a request is for an RSS feed. The feeds have their
// own access rule: a secret token or a session instead of the login redirect.
func feedPath(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/feeds/")
}
