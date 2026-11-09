---
id: rep-fybiz
status: closed
deps: [rep-bsxwl]
links: []
created: 2026-11-09T17:54:18Z
type: task
priority: 2
assignee: Stavros Korokithakis
---
# Markdown rendering and mention parsing

Ready for implementation. Spec: docs/PRD.md (approved).
Objective: a small package (internal/markdown) with no DB access: render Markdown source to sanitized HTML (goldmark + bluemonday UGC policy, links get rel=nofollow noopener), and extract the set of @usernames from the source using the goldmark AST, skipping code spans and code blocks. Username syntax per PRD §3.1. Return lowercase, deduplicated names.
Tests: script/onerror/javascript: URLs removed, mentions in code ignored, mentions next to punctuation found, email-like 'a@b' not a mention.
Non-goals: rendering mentions as links, any DB lookup.

