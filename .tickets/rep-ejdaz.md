---
id: rep-ejdaz
status: closed
deps: []
links: []
created: 2026-11-10T19:22:14Z
type: feature
priority: 2
assignee: Stavros Korokithakis
external-ref: STA-669
---
# API documentation page at /account/api (STA-669)

Ready for implementation.

Objective: an HTML page at GET /account/api, logged-in members only (same access as /account; visitors on a public forum are redirected to login like other account pages). Link to it from the feeds panel on the account page ('API documentation').

Content, in this order:
1. Your links: full absolute clickable URLs with the member's token: /api/threads?token=..., and the pattern /api/threads/{id}?token=... (pattern shown as text, not a link).
2. Endpoints: the two endpoints, the page param (30 per page, front page order), has_next.
3. Fields: short tables/kv lists of thread list fields, extra thread-detail fields (body, body_html, edited_at), and comment fields, matching internal/web/api_handlers.go exactly. Explain deleted placeholders (deleted: true, empty author/body/body_html, replies kept).
4. Example: a short hand-written JSON response for one thread with one nested reply, in a pre/code block. Use placeholder data only.
5. Access: session, token, or public forum; refused requests get a JSON 404; the token is the feed token; link to /account for reset.

Token: the member may have no token yet. Reuse the account page's lazy creation (SetFeedTokenIfEmpty in accountPageData) by extracting a shared helper; do not duplicate it. Build URLs with the existing feedURL/absoluteURL helpers.

Scope: new template under internal/web/templates/pages/, handler in account_handlers.go, route in server.go, account.html link, small CSS only if existing classes (panel, kv, hint, label) are not enough. Tests: page requires login, shows the member's tokenized URL, creates a token when missing, account page links to it.

Non-goals: interactive console, OpenAPI, RSS docs on this page, nav/footer links, a test that checks the example against real output, README changes (architect handles copy). Copy should be terse, lowercase headings like the account page; the architect will polish it.

## Design

Not under /api/: that prefix is JSON-only with its own access rule. Members only: visitors need no token and the README covers them. No design-reference screen exists; reuse account page panel styles.

