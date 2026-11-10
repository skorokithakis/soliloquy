---
id: rep-izgum
status: closed
deps: []
links: []
created: 2026-11-10T17:50:57Z
type: feature
priority: 2
assignee: Stavros Korokithakis
external-ref: STA-669
---
# Read-only JSON API for threads and comments (STA-669)

Ready for implementation.

Objective: add two read-only JSON endpoints.
- GET /api/threads?page=N: front page as JSON. Same order (last_activity_at DESC, id DESC) and same paging (threadsPerPage=30, same page param parsing as handleIndex). Response: {"threads": [...], "page": N, "has_next": bool}. Each thread: id (public ID), title, url (external URL, empty string if none), link (absolute thread page URL, via s.absoluteURL), author (AuthorName()), score, comment_count, created_at, last_activity_at.
- GET /api/threads/{id}: {"thread": {...list fields plus body (Markdown source), body_html, edited_at (null if never)}, "comments": [tree]}. Use ListComments + db.BuildCommentTree so order and deleted-placeholder rules match the HTML thread page. Each comment: id (public ID), author, score, created_at, edited_at, body, body_html, deleted (bool), replies (always an array, never null). Deleted comments: deleted=true, author/body/body_html empty. Unknown or deleted thread: 404 JSON.

Access: exactly the RSS feed rule (session, then valid ?token= feed token, then public setting, else 404). Extend feedPath (middleware.go) or add an equivalent check so /api/ paths use allowFeed, and never get the login redirect. Pass viewer 0 to list queries (no 'voted' field).

Responses: Content-Type application/json; charset=utf-8. Times in RFC 3339 UTC. Errors as {"error": "..."}.

Scope: new internal/web/api_handlers.go, routes in server.go, middleware.go, tests in a new api_test.go following feeds_test.go/helpers_test.go patterns.

Non-goals: writes/votes, CORS, rate limiting, a separate API token or Bearer header, recent comments list, newest-first sort, search/notification/user endpoints, versioning. No README or template changes (architect handles copy).

## Design

Reuse the feed token instead of a new API token: same read-only grant, no schema or UI change. /api/ prefix instead of Reddit-style .json suffix: ServeMux would treat 'abc.json' as the ID and '/' cannot take a suffix.

## Acceptance Criteria

Tests cover: list paging and has_next; thread with nested replies and a deleted parent placeholder; private forum without auth -> 404, with valid token -> 200, with session -> 200; public forum without auth -> 200; disabled user's token rejected on private forum.

