---
id: rep-ttwph
status: closed
deps: [rep-pimqe, rep-kyvca]
links: []
created: 2026-11-09T21:38:26Z
type: task
priority: 2
assignee: Stavros Korokithakis
---
# Design: invites page with invite tree

Objective: one /invites page per misc/design/05-* and 13-*, merging the invite tree and 'my invites'.
- Read-only tree drawn with ├── └── │ prefixes in monospace, badges admin/you/disabled, [deleted user] in italics, join date right-aligned with a dotted leader. Summary line: accounts, deleted, disabled.
- 'Invite someone' card: '+ new invite link' (existing POST), the newest link shown highlighted with a copy button.
- 'Your links': all of the user's invites with state: 'unused · Nd left', 'expired YYYY-MM-DD', 'used by <name>'. Needs a DB query for all invites of a user, not only active ones.
- GET /tree redirects to /invites. Remove the admin action buttons from the tree (they live on /admin/users now).
Design reference: misc/design/ (one HTML + PNG per screen, see misc/design/README.md). Match the screens closely; desktop, mobile (max-width:640px) and dark palette. All pages must keep working with no JavaScript; JS only enhances. Keep one style.css and one app.js, no build step. Ready for implementation.

