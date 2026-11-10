---
id: rep-lmeod
status: closed
deps: [rep-fqoqc]
links: []
created: 2026-11-10T20:18:44Z
type: task
priority: 2
assignee: Stavros Korokithakis
external-ref: STA-667
---
# ntfy push notifications

Ready for implementation.

Objective: each user can set an ntfy topic URL. The server POSTs a push to it for their personal notifications (on by default) and, if chosen, for new threads (off by default). Never for all comments.

Data: new migration (next after 0011) adding per-user columns: ntfy URL (empty = off), notify on replies (default 1), notify on new threads (default 0).

Sending: a small client in its own package, like internal/discord (plain-text POST body, Title header optional, Click header = absolute forum URL, timeout, errors never contain the URL since it is a secret). Behind an interface so tests avoid the network (see webhookSender). Fire and forget in a goroutine with context.Background(), like notifyDiscordThread: no retries, no queue, failures logged without the URL. Text for kind 3: reuse the text builder from the desktop-alerts ticket. New thread: 'New thread: <title>' plus the author; never send to the thread's own author; only active users with a URL and the option on.

Hook kind 3 into the same per-notification hook that the desktop-alerts ticket adds. Hook new threads next to the notifyDiscordThread call in threads_handlers.go.

Validation: absolute https URL with a host (same rule as validateDiscordWebhooks), or empty to turn off. Any host is allowed (owner decision).

Account page: in the 'notifications' panel, a URL field, checkboxes 'replies to me' and 'new threads', a 'save' button and a 'send test' button (sends a test push to the saved URL and shows the result, like the Discord test on admin settings). Hint: anyone who knows the topic name can read it, so use a hard-to-guess name.

Non-goals: Gotify/Pushover/other services; ntfy auth tokens; pushes for all comments; comment body in pushes.

