---
id: rep-plwkp
status: closed
deps: [rep-pimqe]
links: []
created: 2026-11-09T21:38:26Z
type: task
priority: 2
assignee: Stavros Korokithakis
---
# Design: new and edit thread form with preview

Objective: thread form per misc/design/02-*.
- Title (counter / 200), url (optional), body with write/preview tabs, counter / 20000, hint 'you need a url, a body, or both', post and cancel, Markdown help panel on the right.
- New endpoint POST /preview (logged in only; origin check applies as for all POSTs): takes Markdown, returns JSON with the sanitized HTML from the same renderer used on save, and the list of usernames that would get a mention notification (existing, active, not the current user). The form shows '@name will get a notification' from it.
- Preview is fetched when the user opens the preview tab and for the mention line (debounced on typing). Without JS the tabs are hidden and the textarea shows.
Non-goals: preview on comment forms.
Design reference: misc/design/ (one HTML + PNG per screen, see misc/design/README.md). Match the screens closely; desktop, mobile (max-width:640px) and dark palette. All pages must keep working with no JavaScript; JS only enhances. Keep one style.css and one app.js, no build step. Ready for implementation.

