---
id: rep-nrjoy
status: closed
deps: []
links: []
created: 2026-11-09T23:05:16Z
type: task
priority: 2
assignee: Stavros Korokithakis
---
# Invites page: one column, invite box above the tree

Ready for implementation.

Objective: /invites is one column at all widths, in this order:
1. Page head (~/invites, people count) and the intro line.
2. 'Invite someone' panel, full width.
3. Tree and its summary line.
4. 'Your links' panel, full width.

Today the tree and the two panels are a wrapping flex row (.invites-layout, .invites-tree, .invites-side), which gives a 320px sidebar on desktop. Remove the side-by-side layout. Reference for look and vertical gaps: misc/design/05-invites-and-invite-tree.png (it shows both panels below the tree; we deliberately move 'invite someone' above the tree, the rest follows the screenshot).

Scope: internal/web/templates/pages/invites.html and the 'Invites' section plus the mobile rules for it in internal/web/static/style.css. Delete CSS rules that become unused. Keep sensible landmarks/aria labels after the split (the two panels are no longer one aside).

Non-goals: changes to the content or behaviour of the panels or the tree; Go handler changes; restyle of the mobile invite box to match design 13; a limit or paging for 'your links'.

Check the result in a browser at desktop (1280px) and mobile (390px) width, light and dark. .symphony/serve runs a QA server on port 8000 (admin/admin).

## Acceptance Criteria

At 1280px and 390px: no sidebar; order is head+intro, invite panel, tree+summary, your links; panels and tree use the full content width; no horizontal scroll.

