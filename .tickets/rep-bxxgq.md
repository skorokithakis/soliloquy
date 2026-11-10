---
id: rep-bxxgq
status: closed
deps: []
links: []
created: 2026-11-10T21:37:15Z
type: task
priority: 2
assignee: Stavros Korokithakis
external-ref: STA-671
---
# Account page: new design markup and CSS

ready for implementation

Objective: make /account (internal/web/templates/pages/account.html, internal/web/static/style.css) match the new design in misc/design/07-account.html and 07-account.png (already updated in the working copy). Read both. Keep every existing form, action, field name and behaviour; this is a visual change plus copy.

Structure (from the design): pagehead with title and log out; sticky left menu (nav.toc, links to #profile #appearance #notifications #password #feeds #delete; 'delete account' in red); cards on the right. Under 760px the menu becomes a horizontal scrolling row, as in the design CSS. Mark the first menu item aria-current in the markup; JS for scroll tracking comes in a later ticket, but add any hooks (ids, data attributes) you need.

Per card:
- profile: whoami term box as today; username note becomes '(fixed)'.
- appearance: card header 'appearance' + 'Saved to your account, so it follows you to other browsers.'; segmented control (.seg) with system/light/dark radios (values auto/light/dark as today); help 'The theme changes as soon as you pick one.'. Keep the existing save button inside the form, give it a hook (e.g. data-theme-save) so later JS can hide it.
- notifications: header text from the design. Sub-section 'desktop alerts' / 'this browser only' / status pill / help text / the existing data-desktop-alerts button. Replace the current data-desktop-alerts-status hint with the pill (keep the data-desktop-alerts-status hook on it; JS sets its text later; pill gets class 'on' only when on). Sub-section 'push to your phone' / 'all devices, using ntfy' / pill 'on' or 'off' from the saved NtfyURL (server side). ntfy URL input full width; help text from design; checkbox legend 'push me for', labels 'replies, comments and mentions' and 'new threads'; buttons 'save' and 'send a test push' plus 'the test uses the saved URL'. Keep the ntfy test result notice inside this sub-section.
- password: header 'password' + 'Changing it signs you out of all your other sessions.'; fields stacked, inputs max 340px as in the design; 'change password' button; help 'Forgot it? There is no email here. Ask an admin for a reset link.' Remove the old account-note paragraph.
- feeds and API: header text from design (keep the /account/api link); grid of label / read-only input / copy button (use the existing data-copy mechanism in app.js); divider; 'make a new token' button (existing reset form) and the 'where' text from the design.
- delete account: danger card as in the design; list text from design ('Your sessions, notifications, push settings, feed token and unused invite links are removed.'), real username and invited count as today; label 'type <b>username</b> to confirm' above the input (max 340px), 'delete my account' button below. Keep data-confirm-username / data-delete-account hooks and server validation.
- The .err box stays at the top of the page, under the pagehead.

CSS: add the design's classes (toc, card, card-h, card-b, sub, sub-h, where, help, seg, pill, checks, feeds, layout, stack) scoped so they cannot affect other pages (e.g. under .account-page) unless a same-named class already exists in style.css with a compatible meaning; check for name clashes first (.card exists at style.css ~712). Use the app's existing colour variables; check dark theme and mobile.
Remove CSS rules that only the account page used and that are now dead, but do not touch rules shared with admin_settings.html and api_docs.html (.panel.account-section, .account-form, .feed-links etc. are used there).

Non-goals: no change to admin settings or API docs pages; no footer change (deliberate deviation, see gn show design-reference); no new handlers; no JS in this ticket except reusing data-copy.

Update tests in internal/web that assert on old copy or markup.

## Design

Design deviations already decided by the owner: footer stays 'powered by soliloquy'; error box and ntfy test notice kept although the design does not show them; checkbox label 'replies, comments and mentions' instead of the design's 'replies and mentions' because the setting also covers comments on your threads.

