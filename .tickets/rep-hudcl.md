---
id: rep-hudcl
status: closed
deps: [rep-ndwzg]
links: []
created: 2026-11-10T21:12:37Z
type: bug
priority: 2
assignee: Stavros Korokithakis
external-ref: STA-670
---
# Fix spacing and button label defects found in site review (STA-670)

Objective: fix small layout defects found in a review of all pages. Each one is a deviation from misc/design or from the rest of the app.

1. Search page, desktop: the input in .bigsearch gets the general 32rem max-width for text inputs, so the 'search' button sits in the middle of the box. The input must fill the box and the button must sit at the right end, as in misc/design/04-search.png. Mobile is already correct.
2. Thread form help box (aside.thread-help): the second label ('how it shows') touches the cheat sheet above it (0px). The rule '.thread-help .label + .label' never matches, because a pre.cheat is between the labels. It must have 14px above it, as in misc/design/02-new-thread.png.
3. Thread page, mobile (max-width layout): the row with the vote box and the meta line touches the thread body (0px). It must be 10px below the body, as in misc/design/11-mobile-thread-dark (the row has margin-top: 10px there). Check a thread without a body too.
4. Account page, 'delete my account' row: the label 'type your username to confirm' gets margin-bottom: 12px from the general 'form > label' rule. On desktop it sits 6px above the centre of the input; on mobile it adds 12px below the label. Remove that margin so the row matches misc/design/07-account.png.
5. Comment reply and comment edit pages (templates/pages/comment_form.html, comments_handlers.go): the submit buttons say 'Post reply' and 'Save changes' and the h1 says '~/Reply' and '~/Edit comment'. Every other button and h1 in the app is lowercase ('post thread', 'save changes' on thread edit, '~/account'). Make the button labels and the visible h1 lowercase. Keep the browser tab title (<title>) as it is now.
6. Admin settings, 'visibility' panel: the 'public forum' checkbox label is bold and offset, unlike the checkboxes on the account page ('replies to me'). Use the same style (.checkbox-row) so both look the same.

Scope: internal/web/static/style.css, the two templates named above, comments_handlers.go, and tests that assert the old labels.

Non-goals: button sizes; fonts; anything not listed. Do not add per-page margins below the page title (.pagehead owns that gap).

Run the app with .symphony/serve (QA login admin/admin on port 8000) and check each item in a browser at 1280px and 390px.

## Acceptance Criteria

Each of the 6 items is fixed and checked in a browser. No other page changes. go test ./... passes.

ready for implementation

