---
id: rep-voqmx
status: closed
deps: []
links: []
created: 2026-11-09T23:25:05Z
type: task
priority: 2
assignee: Stavros Korokithakis
---
# Shorten invite tokens to 8 alphanumeric characters

Objective: invite links (normal invites and the first-admin bootstrap invite) use an 8-char token from [A-Za-z0-9] instead of the 43-char base64url token.

Scope: add auth.GenerateInviteToken() in internal/auth/token.go (crypto/rand, rejection sampling so there is no modulo bias). Use it in handleCreateInvite (internal/web/invites_handlers.go) and ensureFirstAdminInvite (main.go). Add a unit test for length and alphabet.

Non-goals: do not change GenerateToken() or its use for sessions and password resets. No schema change, no migration (old long invites keep working; lookup is exact match). No retry on token collision. No rate limiting.

Caveat: the tokenSuffix display on the invites page will now show the full token for new invites; that is fine, leave it.

ready for implementation

