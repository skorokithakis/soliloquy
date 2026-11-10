---
id: rep-itzbp
status: closed
deps: []
links: []
created: 2026-11-10T20:18:44Z
type: task
priority: 2
assignee: Stavros Korokithakis
external-ref: STA-667
---
# Personal notifications RSS feed

Ready for implementation.

Objective: add GET /feeds/notifications, an RSS 2.0 feed of the requesting user's in-app notifications (reply, thread_comment, mention), the 50 newest, newest first, no paging. It is the same data as the notifications page (db.ListNotifications).

Scope: internal/web/feeds_handlers.go, internal/web/middleware.go (allowFeed), internal/web/server.go routes, account page template + handler, tests.

Items: title like '<actor> replied to you in <thread title>' / '<actor> commented on your thread <title>' / '<actor> mentioned you in <title>'; link = absolute comment anchor, or thread link for a mention in a thread body (same as notificationView.Link); dc:creator = actor name (handles deleted users); pubDate = notification time; description = body HTML (already empty for deleted content). GUID: must be unique per notification, not the comment URL; use a non-permalink GUID based on the notification ID.

Access (the important part): the feed identifies its user from ?token= (active user only) or from the session. It must NEVER fall back to the public-forum rule: no valid token and no session = 404, also when the forum is public. Today allowFeed only grants or denies and does not tell the handler who the token belongs to; restructure so the handler gets the user (e.g. put the token user in the request context), without changing behaviour of the threads/comments feeds.

Account page: add a third row 'notifications' to the existing feeds panel, with the tokenized URL. Update the hint texts (they say 'new threads and comments' and 'replaces both links') to cover three links.

Non-goals: no rel=alternate link in the layout for this feed; reading the feed does not mark notifications read; no filters by kind.

## Acceptance Criteria

Tests cover: user sees only own notifications; invalid token, deleted user's token and no token return 404 on a private and on a public forum; threads/comments feed access is unchanged.

