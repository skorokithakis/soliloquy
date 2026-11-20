# Changelog

## [0.4.0](https://github.com/skorokithakis/soliloquy/compare/v0.3.0...v0.4.0) (2026-11-20)


### Features

* add a table of contents to long thread posts (STA-747) ([233f998](https://github.com/skorokithakis/soliloquy/commit/233f998694827257d87eefd158e21d2fc3cc4216))
* add emoji reactions on comments that count as upvotes (STA-744) ([a3132ac](https://github.com/skorokithakis/soliloquy/commit/a3132ac45e3b814f142cf2ef712a8e573e4cfd48))
* add keyboard shortcuts to jump between unread comments (STA-742) ([4822077](https://github.com/skorokithakis/soliloquy/commit/4822077cbb2093b41eb87db8b1e7b0a0c09c41d9))
* add the parent context area to the single-comment page (STA-746) ([292bd42](https://github.com/skorokithakis/soliloquy/commit/292bd42e627af046955bf1b98493013b22e57df4))
* add the single-comment page and timestamp links (STA-746) ([a0c84e7](https://github.com/skorokithakis/soliloquy/commit/a0c84e73eea970416a74d5fb607b9ca2373d7c2c))
* cover the author's own vote on the single-comment page (STA-746) ([177fc30](https://github.com/skorokithakis/soliloquy/commit/177fc30a40fa3cd6e512fcc144b68ea505d6afc1))
* fit the single-comment page to the new reaction row (STA-746) ([5e87d2e](https://github.com/skorokithakis/soliloquy/commit/5e87d2e24634a1080662be9134d37f76151eec62))
* follow a thread to get notified about new comments (STA-741) ([42b0b62](https://github.com/skorokithakis/soliloquy/commit/42b0b62046ec3d929d30190c39912c60b54c3550))
* give the comment page a sticky thread bar and full parents (STA-754) ([7bc1bb6](https://github.com/skorokithakis/soliloquy/commit/7bc1bb6888603f3eca8381d701099374b944543f))
* keep unposted text as drafts in the browser (STA-745) ([7d51358](https://github.com/skorokithakis/soliloquy/commit/7d51358568108ee7e9b0b1e2f5e2bad19a885fd8))
* make the author's upvote a real vote that can be removed (STA-749) ([10c1f4e](https://github.com/skorokithakis/soliloquy/commit/10c1f4ec548a586c500e24b4c316aca11b59b46c))
* open comment notifications on the single-comment page (STA-746) ([203a3e9](https://github.com/skorokithakis/soliloquy/commit/203a3e9edb89d641417d63168d490e14daf340a1))
* polish the single-comment page (STA-746) ([1213ac1](https://github.com/skorokithakis/soliloquy/commit/1213ac1616769c1d2fe65256c44a8253eb8e79d4))
* quote selected text when replying (STA-740) ([38797c2](https://github.com/skorokithakis/soliloquy/commit/38797c21b4acf549fb8b112f9c3afe3f37dd74ec))
* show edits, deletes and vote counts live on the thread page (STA-743) ([5560b93](https://github.com/skorokithakis/soliloquy/commit/5560b93a99efd4e567f0465c60b5b049b3320c83))
* show reaction chips in the comment action row with the new emoji look (STA-748) ([3e2e72e](https://github.com/skorokithakis/soliloquy/commit/3e2e72ef2a062e07c7ca576a8a0ecb5d27905299))


### Bug Fixes

* close the react menu on a tap outside it or on Escape (STA-744) ([ed07cd3](https://github.com/skorokithakis/soliloquy/commit/ed07cd3e53506819b52a18bf63cb71d510171156))
* line up the author name with the body on mobile comments without an upvote arrow (STA-749) ([0a7c354](https://github.com/skorokithakis/soliloquy/commit/0a7c354d1bddfe9ec67927943ca0c51b680c8331))
* make reaction chips 22px high on mobile (STA-748) ([8c73151](https://github.com/skorokithakis/soliloquy/commit/8c73151b4880e3060d70ec96ae9fd451ecdef139))
* put the same gap after react as after the other comment actions on mobile (STA-748) ([b4eb205](https://github.com/skorokithakis/soliloquy/commit/b4eb205375ba48913d2b535942cbb204655bd536))
* stop marking comments new on a first visit and on your own live comments (STA-750) ([b65e6e9](https://github.com/skorokithakis/soliloquy/commit/b65e6e912f50f724ce9d2c3efa103dc617696f45))

## [0.3.0](https://github.com/skorokithakis/soliloquy/compare/v0.2.0...v0.3.0) (2026-11-19)


### Features

* add Prometheus metrics endpoint (STA-738) ([6dfff53](https://github.com/skorokithakis/soliloquy/commit/6dfff533451c97fd0a8916d2a8eb4c8c0205354f))

## [0.2.0](https://github.com/skorokithakis/soliloquy/compare/v0.1.0...v0.2.0) (2026-11-15)


### Features

* add member profile pages (STA-725) ([4615d50](https://github.com/skorokithakis/soliloquy/commit/4615d50970747fd449149e832ce766dba442f6bb))
* confirm deletes and other destructive actions (STA-672) ([a58b233](https://github.com/skorokithakis/soliloquy/commit/a58b2330e453d9b105d9a5317d0041fea2b9569d))
* let admins re-enable disabled users (STA-727) ([5eceb80](https://github.com/skorokithakis/soliloquy/commit/5eceb809248cdfe571c0ee4d43623cb0146fa638))
* paginate search results (STA-729) ([17cc674](https://github.com/skorokithakis/soliloquy/commit/17cc6741cabbfd018aeb4c4559caec515544475d))
* preview on comment forms (STA-724) ([9ac4cf0](https://github.com/skorokithakis/soliloquy/commit/9ac4cf008a5dacbc6eb67e4b1f877afdb5a67966))
* render GitHub-flavoured Markdown in posts and comments (STA-723) ([606c842](https://github.com/skorokithakis/soliloquy/commit/606c842afa04f2594e8169eaa6c7211d3a550206))
* show unread markers for threads and comments (STA-721) ([4c24d52](https://github.com/skorokithakis/soliloquy/commit/4c24d5262be1fc7eda00dcbe81756d29b557610f))


### Bug Fixes

* hide notifications for deleted threads and comments (STA-728) ([f3f60de](https://github.com/skorokithakis/soliloquy/commit/f3f60de459b992d7b338e884521cdfccdd88ed3b))
* make profile and form headings read as titles (STA-726) ([7759a5a](https://github.com/skorokithakis/soliloquy/commit/7759a5a80564098b956650dde0889f6477fd7b75))

## 0.1.0 (2026-11-10)


### Features

* publish release binaries and Docker images with Release Please ([7694b53](https://github.com/skorokithakis/soliloquy/commit/7694b53e19639ceb96c3cbdaf424d8b972ca76d8))
