---
id: rep-gxrog
status: closed
deps: [rep-pimqe]
links: []
created: 2026-11-09T21:38:26Z
type: task
priority: 2
assignee: Stavros Korokithakis
---
# Design: account page with theme setting

Objective: new GET /account page per misc/design/07-*, replacing /password.
- '$ whoami' block: username (fixed), role, joined, invited by, invited count with link to /invites, session expiry of the current session.
- Change password form (not in the design; keep it, same style). GET /password redirects to /account; form posts may keep their current URLs.
- Theme setting: system / light / dark. Migration adds users.theme (TEXT NOT NULL DEFAULT 'auto', CHECK in auto/light/dark). POST saves it. Layout uses it for the theme class added by the base ticket.
- Log out button. Delete account box: the user types their username to confirm; the server checks it (case-insensitive) and rejects a mismatch. Existing deletion rules stay.
- Update docs/PRD.md: 3.10 and the non-goal 'A dark mode toggle' (now a per-account theme setting on the account page).
Design reference: misc/design/ (one HTML + PNG per screen, see misc/design/README.md). Match the screens closely; desktop, mobile (max-width:640px) and dark palette. All pages must keep working with no JavaScript; JS only enhances. Keep one style.css and one app.js, no build step. Ready for implementation.

