---
id: rep-fqoqc
status: closed
deps: []
links: []
created: 2026-11-10T20:18:44Z
type: task
priority: 2
assignee: Stavros Korokithakis
external-ref: STA-667
---
# Browser desktop alerts for personal notifications

Ready for implementation.

Objective: when a user gets a new in-app notification and a forum tab is open but hidden, the browser shows a desktop alert (Notification API). Kind 3 only (reply, thread_comment, mention); no alerts for new threads or all comments.

Server: notifications are created in internal/web/notifications.go (notifyNewComment, notifyMentions; the latter is also called for thread bodies and edits). Add one small server-side hook, called for every notification created, that publishes a new SSE event (e.g. 'notification') on the recipient's hub.UserTopic with {text, url}. Text: '<actor> replied to you in <thread title>' / '<actor> commented on your thread <title>' / '<actor> mentioned you in <title>'. URL: the same link as the notifications page. No comment body. Put the text builder where ticket 'ntfy push' can reuse it (that ticket sends the same text). The existing 'badge' event stays as it is.

Client (internal/web/static/app.js): on 'notification', if alerts are on in this browser, permission is granted and document.hidden, show new Notification(text, {tag: <fixed tag>}) and open the URL on click. The fixed tag collapses duplicates when several tabs are open.

Account page: new panel 'notifications' (same panel/button style as the theme panel) with a 'turn on desktop alerts' / 'turn off desktop alerts' button and a status line: on in this browser / off / blocked by the browser / not supported. The on/off state is stored in localStorage, not in the DB. Turning on calls Notification.requestPermission() from the click.

Non-goals: Web Push / service worker; alerts when no tab is open; sound; per-kind options.

