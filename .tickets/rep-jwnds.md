---
id: rep-jwnds
status: closed
deps: [rep-nsgwo, rep-fybiz]
links: []
created: 2026-11-09T17:54:18Z
type: task
priority: 2
assignee: Stavros Korokithakis
---
# Threads and front page

Ready for implementation. Spec: docs/PRD.md (approved).
Objective: PRD §3.3, plus thread edit/delete from §3.4.
Scope: threads table (title, url, body source + rendered HTML, author, created_at, edited_at, deleted_at, last_activity_at, score). Score starts at 1 (author's implicit vote; no vote row). Create, view, edit (shows 'edited'), delete by author (soft; hidden from front page; thread page 404). Front page: 30 per page, ordered by last_activity_at desc, row shows title (linked to URL if present, with domain), author ('[deleted user]' if deleted), score, comment count, relative time. Server checks author permission on every write.
Leave a clear hook for admin delete (admin task) and for comment/vote code to update last_activity_at and score.
Non-goals: comments, votes, notifications.

