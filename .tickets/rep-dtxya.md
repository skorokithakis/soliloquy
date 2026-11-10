---
id: rep-dtxya
status: closed
deps: []
links: []
created: 2026-11-10T01:21:31Z
type: chore
priority: 2
assignee: Stavros Korokithakis
---
# Unify page title spacing and error message style (STA-660)

Ready for implementation.

Objective: one rule sets the gap below every page title, and one style shows every form error.

1. Page titles. Every page must put its <h1 class="pt"> inside the .pagehead wrapper (flex row, margin-bottom 10px), like index.html and account.html do. Today notifications.html, thread_form.html and comment_form.html have a bare h1, and .pt has margin 0, so content touches the title (visible on the empty notifications page and on the comment form). Remove the per-page corrections that this makes redundant: margin-top 16px on .thread-form-layout, and the mobile rule '.notifications-page .pt { margin: 10px 0 4px }' (replace with whatever keeps the mobile notifications page looking sane; the front page hides its pagehead on mobile, other pages do not).

2. Errors. Two styles exist: .err (bordered red box) and .error (plain red text). Use .err everywhere and delete the .error rule. .error is used in thread_form.html, comment_form.html and the comment form in thread.html. Follow the existing markup of the page-level use in account.html (<p class="err" role="alert">). Make sure the box has sensible space between it and the element after it (editor / form) in all three places, and compare with how account.html and admin_settings.html space it so there is one rule, not three.

Scope: internal/web/templates/pages/*.html, internal/web/static/style.css. Check Go tests that assert on markup.

Non-goals: do not change empty-state message styles ('No threads yet' etc.). Do not change the search page (it has no title). Do not change login/signup/reset error markup. No new classes unless unavoidable. No redesign of spacing elsewhere.

## Acceptance Criteria

Gap between title and first content is the same (10px from .pagehead) on notifications, thread form, comment form, as on the front page. No .error class remains in templates or CSS. Desktop and mobile (<=640px) both checked in a browser, light and dark.

