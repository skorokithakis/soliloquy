---
id: rep-eqlvc
status: closed
deps: []
links: []
created: 2026-11-10T21:37:15Z
type: task
priority: 2
assignee: Stavros Korokithakis
external-ref: STA-671
---
# Account deletion clears ntfy URL and feed token

ready for implementation

Objective: db.DeleteAccount (internal/db/admin.go) also sets ntfy_url to empty and feed_token to empty/NULL for the deleted user, in the same transaction. Match how the columns represent 'not set' today (check users.go and SetFeedTokenIfEmpty). The new account page text promises: 'Your sessions, notifications, push settings, feed token and unused invite links are removed.'

Scope: internal/db/admin.go, extend TestDeleteAccountRemovesPrivateDataAndKeepsContent in internal/db/admin_test.go.

Non-goals: no migration, no change to DisableUser, no clearing of ntfy_replies/ntfy_threads flags (harmless).

