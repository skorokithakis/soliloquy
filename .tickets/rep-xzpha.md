---
id: rep-xzpha
status: closed
deps: []
links: []
created: 2026-11-09T22:37:14Z
type: task
priority: 2
assignee: Stavros Korokithakis
---
# Admin settings page: public flag and forum name

Ready for implementation.

Objective: add DB-backed site settings that admins edit at a new page, GET/POST /admin/settings.

Scope:
- Migration internal/db/migrations/0009_settings.sql: key/value settings table. Defaults when no row exists: public = off, forum name = "soliloquy".
- internal/db: small getter/setter for the two settings (a Settings struct is fine).
- internal/web: /admin/settings page (requireAdmin on GET and POST), link to it from the admin area (e.g. admin nav or admin_users page). Form: 'Public forum' checkbox with a short warning that turning it on makes all existing threads and comments visible to anyone without an account; 'Forum name' text field.
- Forum name: validate trimmed, non-empty, max 40 chars; show error on the form otherwise.
- Expose the forum name to the layout (e.g. PageData.ForumName filled in pageData and anywhere PageData is built by hand). Use it in layout.html for the <title> suffix, the logo text and the logo aria-label. Footer keeps 'soliloquy v1.0' and the source link unchanged (it names the software).
- Read settings from SQLite per request; no in-memory cache.
- Tests for DB defaults/round trip and for the admin page (non-admin gets 403, valid save, invalid name).

Non-goals: public access behaviour itself (separate ticket), any other settings, caching.

