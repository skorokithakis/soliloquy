---
id: rep-cdkmt
status: closed
deps: [rep-pimqe]
links: []
created: 2026-11-09T21:38:26Z
type: task
priority: 2
assignee: Stavros Korokithakis
---
# Design: log in, join and reset pages

Objective: logged-out pages per misc/design/08-* and 09-*.
- Login card: logo, 'a private forum · members only', host of the base URL. When throttled, show the wait in seconds from the throttle (it already returns it), and a disabled 'wait Ns' button that counts down in JS and then enables.
- Join page: box with '<inviter> invited you', link expiry in days, and the inviter's chain from the root down to 'you'. Username field with live checks: length 3 to 20, allowed characters, available. Add 'password again'; the server rejects a mismatch.
- New endpoint GET /invite/{token}/check?username=... returns JSON {valid, available}; it requires a valid, unused invite token. Reuse the existing username validation.
- Reset page in the same card style.
Design reference: misc/design/ (one HTML + PNG per screen, see misc/design/README.md). Match the screens closely; desktop, mobile (max-width:640px) and dark palette. All pages must keep working with no JavaScript; JS only enhances. Keep one style.css and one app.js, no build step. Ready for implementation.

