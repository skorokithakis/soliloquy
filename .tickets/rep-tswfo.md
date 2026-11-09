---
id: rep-tswfo
status: closed
deps: [rep-pimqe]
links: []
created: 2026-11-09T21:38:26Z
type: task
priority: 2
assignee: Stavros Korokithakis
---
# Design: front page

Objective: front page per misc/design/00-* and 10-*.
- Rows with vote arrow and score column (vote form per row, uses the existing vote endpoint and app.js no-reload voting; no arrow on own threads), title, domain, author, comment count, age on the right. Deleted authors in italics.
- Header line '~/threads' and 'sorted by latest activity · page N'.
- Footer line 'page N of M · 30 per page' with newer/older buttons. Needs a count of visible threads.
Design reference: misc/design/ (one HTML + PNG per screen, see misc/design/README.md). Match the screens closely; desktop, mobile (max-width:640px) and dark palette. All pages must keep working with no JavaScript; JS only enhances. Keep one style.css and one app.js, no build step. Ready for implementation.

