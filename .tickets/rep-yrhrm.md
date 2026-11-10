---
id: rep-yrhrm
status: closed
deps: []
links: []
created: 2026-11-10T14:35:53Z
type: task
priority: 2
assignee: Stavros Korokithakis
external-ref: STA-666
---
# Add RSS feeds for threads and comments (STA-666)

ready for implementation

Objective: RSS 2.0 feeds for new threads and all new comments, readable by feed readers on private forums via a per-user secret token in the URL.

Scope:
- Migration 0011: nullable users.feed_token TEXT with a unique index. Stored in plain text on purpose (read-only access to content already in the DB; lets the account page show the URL again). Generate with auth.GenerateToken.
- DB: lookup user by feed token; set/reset token; list 50 newest live threads by created_at DESC; list 50 newest live comments by created_at DESC, excluding deleted comments and comments on deleted threads, with thread public ID and title.
- Routes: GET /feeds/threads and GET /feeds/comments, optional ?token=. Output RSS 2.0 via encoding/xml, Content-Type application/rss+xml; charset=utf-8. Channel title uses the forum name from settings. All links absolute, built from cfg.BaseURL.
  - Thread item: title, link = thread page, guid = thread page URL (isPermaLink), pubDate, author name (AuthorName()), description = body HTML; if the thread has an external URL, prepend a link to it in the description.
  - Comment item: title '<author> on <thread title>', link/guid = thread page URL with the comment anchor (use the same anchor format as the thread page), description = comment body HTML.
- Access rule in middleware (loadUser), for /feeds/ paths only:
  1. token present and belongs to an active user (not disabled, not deleted) -> serve, regardless of the public setting;
  2. else if forum is public -> serve;
  3. else -> 404 (no login redirect; readers cannot follow it).
  A logged-in session cookie also works. The token must never create a session or set cookies.
- Account page: new 'feeds' section (style like the existing panels) with both tokenized feed URLs (always tokenized, also when the forum is public) and a POST 'Reset feed link' button. Create the token on first account page view if null.
- layout.html: <link rel=alternate type=application/rss+xml> for both feeds on pages for logged-in users (tokenized URLs). For visitors on a public forum, the plain URLs.
- Short README mention of feeds.

Non-goals: Atom/JSON Feed, per-thread or per-user feeds, search or notification feeds, token expiry, paging, conditional GET/ETag.

Caveats:
- Owner requirement: a tokenized URL must keep working when the forum is switched public and back to private. Test this sequence explicitly (private -> public -> private with the same token).
- Disabled or deleted user tokens must stop working. Reset must invalidate the old token.
- Follow the existing originCheck for the reset POST.

## Acceptance Criteria

Tests cover: valid token on private forum; bad/missing token on private -> 404; no token on public -> 200; same token works across private->public->private; disabled user token -> 404; reset invalidates the old token; deleted content excluded.

