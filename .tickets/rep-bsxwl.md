---
id: rep-bsxwl
status: closed
deps: []
links: []
created: 2026-11-09T17:54:18Z
type: task
priority: 2
assignee: Stavros Korokithakis
---
# Project skeleton

Ready for implementation. Spec: docs/PRD.md (approved).
Objective: runnable Go server with nothing but a base page.
Scope: go.mod (module github.com/skorokithakis/soliloquy, Go 1.24), main.go, config (PRD §4 Configuration: listen addr, DB path, base URL; flags with env var fallback), SQLite via modernc.org/sqlite in WAL mode with foreign keys on, schema migrations embedded in the binary (numbered SQL files, tracked with PRAGMA user_version), html/template layout with header placeholder, one CSS file with dark mode via prefers-color-scheme, one empty JS file, all embedded. Multi-stage Dockerfile producing a static binary.
Suggested layout: main.go; internal/db (open, migrate, queries); internal/web (handlers, templates, static). Other packages are added by later tasks.
Non-goals: any feature tables or pages beyond a placeholder front page. Later tasks add their own migrations.

