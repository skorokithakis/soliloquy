---
id: sol-dyvij
status: closed
deps: []
links: []
created: 2026-11-10T02:14:56Z
type: task
priority: 2
assignee: Stavros Korokithakis
external-ref: STA-663
---
# Add popular oEmbed sites to the title table (STA-663)

Objective: owner wants oEmbed used wherever a popular site offers it, instead of page scraping. Extend internal/linktitle/sites.go.

1. Add an 'oEmbed string' endpoint field to site. If set, Fetch uses fetchOEmbed with it (falls back to the page fetch as today). Keep the 'title func' field for custom code. Convert YouTube to the new field.
2. Add these entries. All were tested by hand (Nov 2026) and return a 'title':
- Vimeo: hosts vimeo.com; https://vimeo.com/api/oembed.json
- Reddit: hosts reddit.com; https://www.reddit.com/oembed; keep junk title 'Reddit'. (redd.it short links fail on oEmbed; leave them to the page fetch, do not add redd.it.)
- SoundCloud: soundcloud.com; https://soundcloud.com/oembed
- Spotify: open.spotify.com; https://open.spotify.com/oembed
- Flickr: flickr.com, flic.kr; https://www.flickr.com/services/oembed
- TikTok: tiktok.com; https://www.tiktok.com/oembed
- Dailymotion: dailymotion.com, dai.ly; https://www.dailymotion.com/services/oembed
- TED: ted.com; https://www.ted.com/services/v1/oembed.json
- Mixcloud: mixcloud.com; https://app.mixcloud.com/oembed/
- Streamable: streamable.com; https://api.streamable.com/oembed.json
- Speaker Deck: speakerdeck.com; https://speakerdeck.com/oembed.json
- Giphy: giphy.com; https://giphy.com/services/oembed
- Scribd: scribd.com; https://www.scribd.com/services/oembed
- Loom: loom.com; https://www.loom.com/v1/oembed
- SlideShare: slideshare.net; https://www.slideshare.net/api/oembed/2
3. Keep tests small: one table-driven test that every entry with oEmbed has an absolute https endpoint and at least one host, plus the existing behaviour tests. No live network in tests.

Non-goals (checked, no usable title field or needs auth): X/Twitter, Bluesky, Tumblr, Instagram, Facebook, CodePen, Kickstarter, Imgur, GitHub Gist, Twitch, Bandcamp. Mastodon (per-instance endpoints).

ready for implementation

