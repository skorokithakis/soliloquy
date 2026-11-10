---
id: rep-yanxv
status: closed
deps: [rep-xgpxi]
links: []
created: 2026-11-10T01:55:03Z
type: task
priority: 2
assignee: Stavros Korokithakis
external-ref: STA-663
---
# Thread form: URL first, auto-fill title on paste (STA-663)

Objective: in internal/web/templates/pages/thread_form.html, move the url field above the title field and move autofocus to url. In internal/web/static/app.js, when the url field gets a paste or change event and the title field is empty, POST the URL to /title (same fetch style as /preview) and fill the title if it is still empty when the response arrives. Update the title counter after filling. Show 'fetching title…' in the empty left span of the title hint while the request runs; clear it after. On failure, show nothing. Ignore stale responses (request counter, as the preview code does). The same template serves the edit form, so it gets the same behaviour.

Also update the help text in the aside if it reads oddly with the new order.

Non-goals: overwriting a non-empty title, any error message, changes without JS (the plain form must work as today).

## Design

Paste event fires before the input value updates; read the value on the next tick or rely on the input/change event after paste.


ready for implementation
