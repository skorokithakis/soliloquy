---
id: rep-kftxt
status: closed
deps: [rep-bxxgq]
links: []
created: 2026-11-10T21:37:15Z
type: task
priority: 2
assignee: Stavros Korokithakis
external-ref: STA-671
---
# Account page JS: instant theme, menu scroll tracking, desktop alerts pill

ready for implementation

Objective, in internal/web/static/app.js, only on the account page:
1. Theme: when a theme radio changes, set the theme class on <html> at once (layout.html uses class 'sq auto|light|dark') and POST the form to /account/theme with fetch in the background (same form data, CSRF/origin rules as the normal submit; check middleware). JS hides the save button; without JS the button and normal submit still work. On fetch failure, fall back to a normal form submit.
2. Menu scroll tracking: move aria-current to the menu link of the section that is in view as the user scrolls (IntersectionObserver or a scroll handler, keep it small). Clicking a link also sets it. Without JS the first item stays marked.
3. Desktop alerts: the existing status logic now drives the pill from ticket 2. Pill text: 'on', 'off', 'blocked by the browser', 'not supported'; class 'on' only when on. Button text 'turn on' / 'turn off' (hidden when not supported, disabled when blocked, as today).

Non-goals: no changes to other pages, no new endpoints. If /account/theme needs a non-redirect response for fetch, a 303 followed by fetch is fine; do not add a JSON endpoint unless needed.


## Notes

**2026-11-10T21:45:29Z**

Also fix from rep-bxxgq: at 1280px the feeds card 'make a new token' button and its 'where' text must sit on one row (text left-aligned next to the button) as in misc/design/07-account.png; today the text wraps below and is centred.
