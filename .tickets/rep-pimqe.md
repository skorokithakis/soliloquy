---
id: rep-pimqe
status: closed
deps: []
links: []
created: 2026-11-09T21:37:56Z
type: task
priority: 1
assignee: Stavros Korokithakis
---
# Design base: fonts, stylesheet, layout, header, footer

Objective: replace the visual base with the design's.
- Vendor IBM Plex Sans and IBM Plex Mono (400/500/600, latin subset, woff2) under internal/web/static/fonts/ with the OFL licence file. @font-face in style.css. No requests to other hosts.
- Replace internal/web/static/style.css with the design's tokens (light set, dark set) and shared components (header, nav, buttons, badges, labels, page title ~/path, lists, cards, forms, footer). Keep existing page content readable until later tickets restyle each page.
- layout.html: new header (logo ›soliloquy with cursor, nav threads/invites/admin with the current one marked, search box, + new thread, bell icon with unread badge, username). 'admin' nav item shows only to admins. Username links to /account (the page arrives in a later ticket; link anyway). Footer: soliloquy version, AGPL-3.0, source link (module repo URL), invite tree link to /invites. Mobile header per misc/design/10-*.
- Theme: the root element gets a class from a PageData field: 'auto' (follows prefers-color-scheme), 'light' or 'dark'. For now it is always 'auto'; the account ticket adds the stored per-user value.
- Layout variant with no header for logged-out pages (login, join, reset) per misc/design/08-* and 09-*.
- app.js: update selectors that the header change breaks (badge update).
Non-goals: per-page restyling beyond what the shared CSS gives; theme setting storage.
Design reference: misc/design/ (one HTML + PNG per screen, see misc/design/README.md). Match the screens closely; desktop, mobile (max-width:640px) and dark palette. All pages must keep working with no JavaScript; JS only enhances. Keep one style.css and one app.js, no build step. Ready for implementation.

