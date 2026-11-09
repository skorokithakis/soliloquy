---
id: rep-ninmq
status: closed
deps: [rep-pimqe]
links: []
created: 2026-11-09T21:38:26Z
type: task
priority: 2
assignee: Stavros Korokithakis
---
# Design: thread page and comments

Objective: thread page, comment tree, reply and edit pages per misc/design/01-* and 11-*.
- Thread header: vote, title, domain, meta (author, age, comment count, edited), body. Comment box above the comments, with counter 'N / 20000'.
- '14 comments · best first' label and a 'live' dot that shows while the SSE stream is open.
- comment.html (also used for live SSE inserts): tree lines, vote arrow, author, 'op' badge when the author is the thread author, age, points, edited, 'parent' link (anchor of parent comment), [-]/[+] collapse, actions reply/link/edit/delete. Deleted comment shows '[deleted]'.
- Collapse is client-side only, no state stored.
- Reply: JS opens the reply form inline under the comment (with counter, cancel). Without JS the reply link still goes to /comments/{id}/reply.
- Comments that arrive live get the 'NEW' highlight.
- Mobile: fixed 'add a comment' bar at the bottom, per 11-*.
- Restyle comment_form.html (reply and edit pages) to match.
Caveat: app.js finds .score and vote buttons and inserts SSE HTML by data-parent-id; keep those contracts or update app.js with them.
Design reference: misc/design/ (one HTML + PNG per screen, see misc/design/README.md). Match the screens closely; desktop, mobile (max-width:640px) and dark palette. All pages must keep working with no JavaScript; JS only enhances. Keep one style.css and one app.js, no build step. Ready for implementation.

