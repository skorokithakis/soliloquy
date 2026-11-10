---
id: rep-xgpxi
status: closed
deps: []
links: []
created: 2026-11-10T01:55:03Z
type: task
priority: 2
assignee: Stavros Korokithakis
external-ref: STA-663
---
# Server endpoint that fetches a page title (STA-663)

Objective: add POST /title. Logged-in members only, same Origin check as other POSTs (like POST /preview). Input: form value 'url'. Output: JSON {"title": "..."}. Any failure (bad URL, blocked address, timeout, non-200, non-HTML, no title) returns 200 with an empty title.

Scope: a small fetcher (new package such as internal/linktitle, or a file in internal/web) plus the handler and route in internal/web. Tests use httptest servers.

Fetch rules:
- Accept only absolute http/https URLs (reuse validateThreadURL).
- Timeout about 5s. At most about 5 redirects.
- Own User-Agent, e.g. 'Soliloquy/1.0 (+https://github.com/skorokithakis/soliloquy)'.
- Read at most about 2 MB. Stream-parse with golang.org/x/net/html (tokenizer) and stop at <body> or </head>. Do not buffer the whole page. 2 MB is needed: YouTube puts <title> about 840 KB into the page.
- Title choice: <meta property="og:title"> first, then <title>. Decode entities, collapse whitespace, trim, cut to 200 characters (rune-safe).
- Block private addresses: in net.Dialer.Control, refuse connections to loopback, private, link-local (incl. 169.254.169.254), unspecified, multicast and IPv4-mapped forms of these. Checking at connect time covers redirects and DNS rebinding. Tests must be able to turn this off (e.g. a field on the fetcher) because httptest listens on 127.0.0.1.

Non-goals: non-UTF-8 charset handling, caching, rate limiting, oEmbed or per-site code, description/image extraction, proxy support.

## Acceptance Criteria

og:title wins over <title>; blocked addresses (including after a redirect to one) return an empty title; a page larger than 2 MB or a slow server does not hang or exhaust memory.


ready for implementation
