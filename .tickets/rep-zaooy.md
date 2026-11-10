---
id: rep-zaooy
status: closed
deps: []
links: []
created: 2026-11-10T00:40:43Z
type: feature
priority: 2
assignee: Stavros Korokithakis
external-ref: STA-659
---
# Post new threads to Discord webhooks (STA-659)

Ready for implementation.

Objective: when a thread is created, post a plain-text message to every Discord webhook URL that an admin has configured.

Scope:
- New package internal/discord: build the message and POST it to a webhook (JSON {content, allowed_mentions: {parse: []}}). Use an http.Client with a ~10s timeout.
- internal/db/settings.go: new setting 'discord_webhooks', stored as one value (newline-separated URLs) in the existing settings key/value table. No schema change. Expose as []string on Settings.
- /admin/settings: new panel with a textarea, one URL per line. Hint text warns that the title, link, author and start of the body of every new thread are sent to Discord, so content leaves the forum even if it is private. Empty = feature off.
- Validation on save: ignore blank lines, trim; every remaining line must parse as an https URL with a host. On error, re-render with an error that names the bad line.
- Test button on /admin/settings (separate POST route, admin-only, e.g. /admin/settings/discord-test): sends a fixed test message to the SAVED URLs synchronously and re-renders the page with a result saying which URLs failed (or that all succeeded / none configured).
- handleCreateThread: after the thread is created, read settings and send to each URL in background goroutines. Never block or fail thread creation. Log failures WITHOUT the webhook URL (it is a secret); log e.g. the index or the HTTP status.

Message content (plain text):
- Title, absolute thread link (cfg BaseURL + thread path), author username, external URL if set.
- Body: raw Markdown, cut to 300 characters (runes) with a trailing '…' if longer. Omit if empty.
- Whole message must stay within Discord's 2000-char limit (title max 200 + body 300 makes this safe in practice; still clamp defensively).

Non-goals: comments, edits, deletes; retries or delivery history; per-webhook options; Slack/generic webhooks; host allowlist; Markdown-to-text conversion.

Caveats: background sends must not use the request context (it is cancelled when the handler returns). Tests should use httptest servers; make the HTTP client/sender injectable on Server so thread-creation tests do not hit the network. Follow existing settings/admin page patterns and CSS classes; the design reference has no admin settings screen, so match the existing panels.

