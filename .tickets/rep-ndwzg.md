---
id: rep-ndwzg
status: closed
deps: []
links: []
created: 2026-11-10T21:12:37Z
type: bug
priority: 2
assignee: Stavros Korokithakis
external-ref: STA-670
---
# Uniform spacing in account/admin/API panels (STA-670)

Objective: the panels (class 'panel account-section') on the account page, the admin settings page and the API docs page have no spacing rule of their own. Spacing comes from '.panel .hint { margin: 8px 0 0 }', the 12px flex gap of '.account-form', and a few one-off margins. The result: buttons touch the text above them (0px), a hint in a form gets 12+8=20px, and dl.kv lists leave extra space at the bottom of a panel.

Give these panels one spacing rule, in internal/web/static/style.css:
- Blocks in a panel are 12px apart.
- A hint (.hint) is 8px below the item before it.
- The panel heading (h2.label) keeps 10px below it, whatever follows.
- No other vertical margins between the direct children of a panel, and none at the top or bottom of the panel content. The same rule applies inside a form.account-form that is in a panel (account page), so that gap and margin cannot add up.

Also:
- Admin settings: panels are 18px apart (now 30px: 12px form gap + 18px margin). The 'save settings' row is 18px below the last panel (now 12px).
- The first panel on the admin settings and API pages is 10px below .pagehead, like the .term box on the account page (.pagehead alone sets the gap below the title; now 28px).
- Remove one-off rules that the new rule makes redundant (for example .feed-links margin-bottom, '.account-form .panel > .notice' margin).

Scope: internal/web/static/style.css. Template changes only if the CSS cannot do it cleanly.

Non-goals: button sizes (32/36/28px come from the design); fonts (kv lists in panels stay as they are); the .term 'whoami' box and the red .dz 'delete my account' box (both are in the design); panels on the invites page and the thread form help box; any page not named above.

Caveats:
- 'turn on desktop alerts' and its status line have the 'hidden' attribute until JS shows them. They must stay hidden, and the spacing must be right in both states.
- form.inline-form (reset feed link) is display:inline, and buttons are inline-flex. Vertical margins and line boxes behave differently there; measure, do not assume.
- form.account-form is also the outer form on admin settings, where its children are panels.
- Check desktop (1280px) and mobile (390px).

Run the app with .symphony/serve (QA login admin/admin on port 8000) and measure the gaps in a browser before and after.

## Design

Suggested shape (the developer may choose another if it is simpler): make the panel a flex column, so that every direct child is a flex item and takes margins the same way, then use adjacent-sibling margins ('> * + *' 12px, '> * + .hint' 8px, '> .label + *' 10px) instead of 'gap', because gap cannot express the 8px hint case. Buttons need align-self: flex-start so that they do not stretch.

## Acceptance Criteria

Measured gaps, account page. Notifications panel: heading 10 hint 12 button 8 status 12 ntfy field 12 checkboxes 8 hint 12 save/send test. Change password: fields 12 apart, 12 to the button, 8 to the hint. Feeds: heading 10 hint 12 links 12 button 8 hint. Theme panel unchanged.
API page: no 0px gap between hint and example; all panels have the same bottom padding (about 15px), also those that end in a dl.kv.
Admin settings: inside panels heading 10 field 8 hint; panels 18px apart; action row 18px below the last panel.
Other pages look the same as before. go test ./... passes.

ready for implementation

