---
id: rep-urkxx
status: closed
deps: [rep-pimqe]
links: []
created: 2026-11-09T21:38:26Z
type: task
priority: 2
assignee: Stavros Korokithakis
---
# Design: search page

Objective: search page per misc/design/04-*.
- Large 'grep' search input with button. Label 'N results · best match first'.
- Rows: kind column (thread/comment), for threads the title, snippet, author · age · comment count; for comments 'author in <thread title>', snippet, age · 'open comment ›'. Snippet highlights use the existing FTS snippet markers, styled as in the design.
- Search results need author name, created time and (threads) comment count added to the query.
Design reference: misc/design/ (one HTML + PNG per screen, see misc/design/README.md). Match the screens closely; desktop, mobile (max-width:640px) and dark palette. All pages must keep working with no JavaScript; JS only enhances. Keep one style.css and one app.js, no build step. Ready for implementation.

