---
id: sol-skgak
status: closed
deps: []
links: []
created: 2026-11-10T02:08:25Z
type: task
priority: 2
assignee: Stavros Korokithakis
external-ref: STA-663
---
# YouTube titles via oEmbed (STA-663)

Objective: YouTube serves a bot-check page to data-centre IPs. Its <title> is ' - YouTube' and it has no og:title, so the fetcher fills the title with ' - YouTube'. Confirmed from the production host; YouTube's oEmbed API still returns the real title from there.

Scope: internal/linktitle only, plus tests.
1. For URLs whose host is youtube.com, www.youtube.com, m.youtube.com, music.youtube.com or youtu.be, get the title from GET https://www.youtube.com/oembed?url=<url-encoded original URL>&format=json. Read the 'title' field from the JSON. Use the same client (timeout, guard, User-Agent) and a small read limit, e.g. 64 KB. Clean it with cleanTitle. If oEmbed fails or gives no title, fall back to the normal page fetch.
2. In the normal path, if the chosen title is empty after removing a trailing ' - YouTube', return an empty title. Keep this generic rule small; do not add rules for other sites.
Tests: the oEmbed base URL must be overridable (unexported field) so tests can point it at an httptest server.

Non-goals: generic oEmbed discovery, other sites, thumbnails or author names.


## Design (owner asked for a generic shape, more sites will follow)

- Add a per-site rules table in internal/linktitle (e.g. sites.go), one entry per site:
  - hosts: hostnames matched exactly or as a parent domain (youtube.com matches www.youtube.com, m.youtube.com, music.youtube.com; youtu.be separate).
  - title (optional): func(ctx, *Fetcher, *url.URL) (string, error). If it returns a non-empty title, use it; otherwise fall back to the normal page fetch.
  - junkTitles (optional): titles (after cleanTitle) that mean "no real title"; the page-fetch result is replaced by "" if it matches.
- A reusable helper fetchOEmbed(ctx, endpoint, pageURL) that returns the oEmbed 'title' (same client, ~64 KB limit), so other oEmbed sites (Vimeo etc.) are one table entry.
- Initial entries:
  - YouTube: title via oEmbed endpoint https://www.youtube.com/oembed; junkTitles ["- YouTube"].
  - Reddit: no custom fetch; junkTitles ["Reddit"] (bot-check page title).
- This replaces item 2 above (no generic ' - YouTube' stripping outside the table).
- Endpoints must be overridable in tests (e.g. unexported field or table copy on the Fetcher).

ready for implementation
