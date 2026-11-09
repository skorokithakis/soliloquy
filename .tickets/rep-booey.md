---
id: rep-booey
status: closed
deps: [rep-jwnds]
links: []
created: 2026-11-09T17:54:18Z
type: task
priority: 2
assignee: Stavros Korokithakis
---
# Comments and votes

Ready for implementation. Spec: docs/PRD.md (approved).
Objective: PRD §3.4 and §3.5.
Scope: comments table (thread, parent nullable, author, source + HTML, created_at, edited_at, deleted_at, score starting at 1). Load all comments of a thread in one query and build the tree in memory (pure function, unit tested). Reply order: score desc, then created_at asc. Deleted comment renders '[deleted]' when it has visible descendants, otherwise hidden (decided at render). New comment updates thread last_activity_at. Edit/delete own comment. Front-page comment count excludes deleted comments.
Votes: votes table (user, item), unique per user per item, toggle on/off, not on own posts, adjusts score. Votes work as plain form POST; the JS file adds fetch-based voting with no reload.
Tests: tree build and sort, deleted-comment visibility, vote rules.
Non-goals: notifications, live updates.

