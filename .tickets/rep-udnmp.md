---
id: rep-udnmp
status: closed
deps: [rep-booey]
links: []
created: 2026-11-09T17:54:18Z
type: task
priority: 2
assignee: Stavros Korokithakis
---
# Invite tree, account deletion and admin actions

Ready for implementation. Spec: docs/PRD.md (approved).
Objective: PRD §3.2, account deletion from §3.1, password reset links, and §3.9.
Scope: invite tree page (nested by invited_by, join date, disabled marked, deleted shown as '[deleted user]'). Delete own account (needs password; content and votes stay; sessions, notifications, unused invites removed; username stays reserved; last active admin cannot). Admin: delete any thread/comment, disable user (ends sessions, cancels unused invites; not self), reset link (single-use, 7 days; using it sets password and ends all sessions), grant/revoke admin (not own). Admin controls live on the invite tree page; no separate dashboard.
Tests: permission checks for every admin action and the self/last-admin rules, reset link rules.

