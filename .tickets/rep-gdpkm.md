---
id: rep-gdpkm
status: closed
deps: []
links: []
created: 2026-11-09T22:58:42Z
type: task
priority: 2
assignee: Stavros Korokithakis
---
# Add favicon, touch icon and Open Graph image/tags

ready for implementation

Objective: give the app a favicon, an iOS touch icon and link previews (Open Graph) on every page.

Mark: amber '›' chevron followed by the amber block cursor (as in the header logo), on a fixed dark square. Colours: background #0f110e, mark #ffb23e. No text anywhere: admins can rename the forum, so the name must not be baked into images. Draw the chevron and cursor as SVG shapes (no font dependency).

Files in internal/web/static/:
- favicon.svg: hand-written, the source of truth.
- favicon.ico: 16+32 px, rasterised from the SVG with ImageMagick.
- apple-touch-icon.png: 180x180.
- og.png: 1200x630, the mark large and centred on the dark background.
Commit the rasters. No build step. Put the exact regeneration command in a short comment (e.g. at the top of favicon.svg or in the README).

layout.html <head>: icon links (svg, ico, apple-touch-icon); og:title (page title), og:site_name (ForumName), og:type=website, og:image as an absolute URL built from the configured base URL, twitter:card=summary_large_image. No og:description.

Routing: serve GET /favicon.ico at the site root (browsers and bots request it by default). Today the login middleware (internal/web/middleware.go loadUser) redirects it to /login for logged-out users; let it through like /static/.

Tests: /favicon.ico returns an icon when logged out; layout contains og:image with the absolute base URL and og:site_name.

Non-goals: per-forum or per-thread images, web manifest/PWA icons, admin icon upload, cache-busting, adaptive light/dark favicon.

## Acceptance Criteria

Logged-out GET /favicon.ico returns the icon, not a redirect. Every page has the icon links and OG tags. Images contain no forum name.

