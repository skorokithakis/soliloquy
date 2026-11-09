---
id: rep-nsgwo
status: closed
deps: [rep-bsxwl]
links: []
created: 2026-11-09T17:54:18Z
type: task
priority: 2
assignee: Stavros Korokithakis
---
# Auth, sessions and invites

Ready for implementation. Spec: docs/PRD.md (approved).
Objective: PRD §3.1 except account deletion and admin password reset (those are in the admin task).
Scope: users table (username unique case-insensitive, bcrypt hash, is_admin, disabled_at, deleted_at, invited_by), sessions, invites. Login/logout, sign-up from invite link, first admin invite printed to log on empty DB, change own password (ends other sessions), 'my invites' page listing unused unexpired links, create invite. Login throttle per username in memory (doubling wait, cap ~1 min, reset on success, prune expired entries). Origin-check middleware on all POSTs (must match base URL; missing Origin = reject). Request middleware that loads the current user and requires login for every page except login and sign-up. Disabled or deleted users cannot log in.
Sessions: random token in HttpOnly, SameSite=Lax cookie (Secure when base URL is https), sliding 1-year expiry, expiry written at most once a day.
Tests: invite single-use and 7-day expiry, username rules, throttle timing, Origin check.
Non-goals: email, rate limiting other than login.

