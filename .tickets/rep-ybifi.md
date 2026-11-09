---
id: rep-ybifi
status: closed
deps: []
links: []
created: 2026-11-09T23:30:49Z
type: task
priority: 2
assignee: Stavros Korokithakis
---
# Random public IDs for threads and comments

Objective: threads and comments get a random 8-char [A-Za-z0-9] public ID. Every thread/comment ID the client can see uses it instead of the sequential integer. Internally (PKs, FKs, votes, notifications, search index, hub topics) keep the integer id.

Client-visible places that must switch to the public ID:
- All /threads/{id}/... and /comments/{id}/... routes (handlers look up by public ID, then use the integer id).
- Link builders (threadPath, commentAnchorPath, commentAction, notification links, search result links).
- Templates: thread.html, index.html, comment.html (id="c..", data-comment-id, data-parent-id, #c.. anchors), search.html.
- Live updates: SSE subscribe query (?thread_id= in layout.html, eventThreadID in events.go) and the SSE payload (comment_id, thread_id, parent_id) plus app.js that reads it.
- Public-mode check in middleware.go that today parses /threads/{id} as an integer.

Scope: new migration 0010 adding public_id to threads and comments with a UNIQUE index; set it on insert. Rename auth.GenerateInviteToken to a general name (e.g. GenerateShortID) and use it for invites, bootstrap invite, threads and comments.

Non-goals: no change to integer PKs or FKs. No redirect for old numeric URLs (they 404). No retry on public ID collision. No change to user IDs or admin routes.

ready for implementation

## Design

SQLite ALTER TABLE cannot add a NOT NULL UNIQUE column directly. Suggested: ADD COLUMN public_id TEXT, backfill existing rows with upper(hex(randomblob(4))) (8 chars, a subset of the alphabet, collisions negligible for the existing few rows), then CREATE UNIQUE INDEX. New rows always get a Go-generated ID; an insert without one should fail rather than silently succeed, so either add a NOT NULL check or make sure the insert functions require it.

