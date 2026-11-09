---
id: rep-kyvca
status: closed
deps: [rep-pimqe]
links: []
created: 2026-11-09T21:38:26Z
type: task
priority: 2
assignee: Stavros Korokithakis
---
# Design: admin users page

Objective: new admin page GET /admin/users per misc/design/06-*. Admins only; others get the existing forbidden response.
- Table: user (you badge, disabled struck through), role, invited by ((root) / [deleted user]), joined, status, actions: reset link, make/remove admin, disable. Header counts: accounts, admins, disabled. Hide actions that the rules forbid (own admin removal, own disable, disabled users).
- The existing admin POST handlers redirect back to /admin/users. A created reset link shows in the banner at the top with a copy button, and the row is highlighted. Keep the current 'shown only once' behaviour.
- Client-side filter box for usernames (no server filtering).
- 'admin' nav item links here.
Non-goals: removing the buttons from the invite tree (the invites ticket does it).
Design reference: misc/design/ (one HTML + PNG per screen, see misc/design/README.md). Match the screens closely; desktop, mobile (max-width:640px) and dark palette. All pages must keep working with no JavaScript; JS only enhances. Keep one style.css and one app.js, no build step. Ready for implementation.

