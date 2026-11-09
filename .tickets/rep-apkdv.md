---
id: rep-apkdv
status: closed
deps: [rep-pimqe]
links: []
created: 2026-11-09T21:38:26Z
type: task
priority: 2
assignee: Stavros Korokithakis
---
# Design: notifications page

Objective: notifications page per misc/design/03-* and 12-*.
- Rows: unread dot, kind label (mention/comment/reply), '<actor> mentioned you in “title”' etc., snippet of the comment as plain text truncated to about one line, age.
- Sections 'unread · N' and 'earlier'. Keep the current rule that opening the page marks all as read; the 'unread' section shows the items that were unread before this view. No 'mark all as read' button.
- Pagination: 30 per page, 'load older' link (plain GET, e.g. ?before= or ?page=).
Design reference: misc/design/ (one HTML + PNG per screen, see misc/design/README.md). Match the screens closely; desktop, mobile (max-width:640px) and dark palette. All pages must keep working with no JavaScript; JS only enhances. Keep one style.css and one app.js, no build step. Ready for implementation.

