---
id: rep-arsyj
status: closed
deps: [rep-pimqe]
links: []
created: 2026-11-09T21:38:26Z
type: task
priority: 2
assignee: Stavros Korokithakis
---
# Design: highlight @mentions

Objective: mentions show highlighted as in misc/design/01-*.
- internal/markdown Render wraps each @username in normal text in <span class="mention">. Same rules as Mentions(): not in code spans or code blocks. No links. The sanitizer must allow exactly this span class.
- Highlight every syntactic mention; no user lookup at render time.
- Update docs/PRD.md section 3.7 ('Mentions show as plain text') to say they are highlighted, not links.
Non-goals: re-rendering stored posts (no production data exists).
Design reference: misc/design/ (one HTML + PNG per screen, see misc/design/README.md). Match the screens closely; desktop, mobile (max-width:640px) and dark palette. All pages must keep working with no JavaScript; JS only enhances. Keep one style.css and one app.js, no build step. Ready for implementation.

