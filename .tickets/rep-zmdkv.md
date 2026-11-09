---
id: rep-zmdkv
status: closed
deps: [rep-booey]
links: []
created: 2026-11-09T17:54:18Z
type: task
priority: 2
assignee: Stavros Korokithakis
---
# Notifications and live updates

Ready for implementation. Spec: docs/PRD.md (approved).
Objective: PRD §3.6 and §3.7.
Scope: notifications table (recipient, kind, actor, thread, comment, read_at). Create on: reply to your comment, top-level comment on your thread, new mention (on edit, only names not in the previous version; use internal/markdown). Never for own actions; only active users. Notifications page (marks as read) and unread badge in header.
Live: in-process pub/sub hub (internal/hub). One SSE endpoint per page, subscribes to the user's badge and, if given a thread id, that thread's new comments. Event for a new comment carries the server-rendered comment HTML and parent id; JS appends it as last child of the parent (or top level). Send X-Accel-Buffering: no, keep-alive comments, unsubscribe on disconnect.
Caveat: rendered comment HTML must not include viewer-specific controls (edit/delete/vote state) that would be wrong for other viewers, or JS must handle that.
Non-goals: live edits, deletes, votes.

