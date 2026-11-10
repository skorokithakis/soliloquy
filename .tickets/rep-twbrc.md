---
id: rep-twbrc
status: closed
deps: []
links: []
created: 2026-11-10T01:51:36Z
type: feature
priority: 2
assignee: Stavros Korokithakis
---
# Cache-busting filenames for static assets (STA-660)

Ready for implementation.

Problem: static files are embedded in the binary and served at fixed URLs (/static/style.css) with no Cache-Control, ETag or Last-Modified. After a deploy, a browser or a proxy/CDN in front of the app can keep serving the old stylesheet.

Objective: every static asset that a template links gets a URL whose filename contains a hash of the file content, e.g. /static/style.3fa9c1d2.css. The URL changes when the content changes, so such responses can be cached forever.

Design:
- At server start (internal/web/server.go, where staticFS is built), hash the content of each top-level file in the embedded static dir (sha256, first 8-10 hex chars) and build a map: logical name -> hashed name, plus the reverse. Insert the hash before the extension. No build step, no generated files; the hashes are computed from the embed.FS at startup.
- Templates get one helper to turn a logical name into the hashed URL (a template func, e.g. {{static "style.css"}}, is preferred; all template parse sites must register it). Use it in layout.html for style.css, app.js, favicon.svg, apple-touch-icon.png and og.png (og:image stays absolute with BaseURL).
- The /static/ handler: a request for a known hashed name serves the real file with 'Cache-Control: public, max-age=31536000, immutable'. Any other path behaves as today (plain file server, no cache header), so unhashed URLs keep working for the fonts and for links to /static/og.png that other sites may already hold. A hashed name that is not in the map (old or wrong hash) must return 404, never the current file, otherwise a cache could store new content under an old immutable URL.
- Unknown logical name passed to the helper: fail loudly (template error), do not emit a broken URL silently.

Non-goals:
- Fonts (static/fonts/*): style.css refers to them by absolute URL; hashing them would need CSS rewriting at startup. They do not change. Leave as is.
- /favicon.ico: fixed conventional path, leave as is.
- No cache headers on HTML pages or on unhashed static paths. No ETag support. No config option. No change to Dockerfile.

Scope: internal/web/server.go (or a small new file next to it), internal/web/templates/layout.html, tests in internal/web. Check existing tests that assert on /static/ URLs in rendered HTML.

## Acceptance Criteria

Rendered pages (logged in and logged out) link hashed URLs for the five assets; each hashed URL returns 200 with the right content, Content-Type and the immutable Cache-Control header; /static/style.css and a font URL still return 200 without that header; a URL with a wrong hash returns 404. Tests cover these cases. Hash is stable across restarts of the same binary.

