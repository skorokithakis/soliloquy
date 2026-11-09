---
id: rep-csykn
status: closed
deps: [rep-booey]
links: []
created: 2026-11-09T17:54:18Z
type: task
priority: 2
assignee: Stavros Korokithakis
---
# Full-text search

Ready for implementation. Spec: docs/PRD.md (approved).
Objective: PRD §3.8.
Scope: FTS5 table(s) over thread title/body and comment bodies (Markdown source), updated on create/edit/delete in the same transaction. Search page with query box, up to 50 results, ranked by bm25, linking to thread or comment anchor. Deleted content and content in deleted threads excluded. User input must be safe as an FTS5 query (quote terms; do not crash on syntax errors). Any snippet is HTML-escaped before display.
Non-goals: pagination of results, highlighting beyond a simple snippet.

