---
id: rep-wcziv
status: closed
deps: [rep-xzpha]
links: []
created: 2026-11-09T22:37:14Z
type: task
priority: 2
assignee: Stavros Korokithakis
---
# Public read-only mode for visitors

Ready for implementation.

Objective: when the 'public' setting (from the admin settings ticket) is on, visitors without a session can read the forum. When it is off, behaviour is exactly as today.

Visitor-readable when public: GET / (thread list, with paging), GET /threads/{id}, GET /search. Everything else (account, notifications, invites, /tree, admin, edit/reply pages, /events, /threads/new) still redirects visitors to /login. Any POST from a visitor redirects to /login (do not 500, do not act).

Scope:
- internal/web/middleware.go loadUser: allow the read-only paths for visitors only when the setting is on.
- Read-only handlers currently dereference currentUser(r) (e.g. handleIndex uses user.ID for vote state). Make them work with no user (viewer ID 0 / no votes, no author/admin actions).
- layout.html: add a visitor mode. Visitors see the full header (logo, nav 'threads' only, search box) and a 'log in' link instead of new-thread, notifications and account. No data-sse-url for visitors (no live updates). The login/signup/reset pages keep the current bare layout.
- Templates: visitors see scores but no vote buttons, no comment form, no reply/edit/delete links. Where the comment form is, show 'Log in to comment' linking to /login.
- Update docs/PRD.md and README.md: forum is private by default; admins can make it public (read-only for visitors) and rename it.
- Tests: visitor access with setting off (all redirect, as today) and on (allowed pages 200 with no interaction controls; disallowed pages and POSTs redirect to /login).

Non-goals: public signup, user profile pages, public invite tree, live updates for visitors, per-thread visibility, SEO/sitemap.

