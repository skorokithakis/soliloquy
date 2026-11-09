---
id: rep-ypqii
status: closed
deps: []
links: []
created: 2026-11-09T23:56:39Z
type: task
priority: 2
assignee: Stavros Korokithakis
---
# Thread edit/delete in meta line; admins can edit threads

Ready for implementation.

Objective:
1. Thread page (internal/web/templates/pages/thread.html): move the edit link and delete form out of the separate .thread-actions row into the .meta line, after the comment count and after the 'edited' marker. Each control gets a '·' separator span before it, like other meta items. Result: 'author · 33m · 2 comments · edited · edit · delete'. Delete stays a POST inline-form button styled like a link (muted, accent on hover, like the old .thread-actions styles). Remove the .thread-actions markup and its CSS in internal/web/static/style.css, including the mobile grid rule (grid-row: 4).
2. Admins can edit any thread (title, URL, body) with the existing edit page. In internal/web/threads_handlers.go: handleEditThreadPage and handleUpdateThread allow author OR admin (same check style as handleDeleteThread). Add a CanEdit flag to the thread page data (author || admin) and use it in the template for the edit link, replacing the IsAuthor use there.

Caveats:
- db.UpdateThread filters on author_id. The handler already passes thread.AuthorID (the thread's owner, not the current user), so admin edits work without a DB change. Keep it that way; update the UpdateThread doc comment so it no longer implies only the owner can edit.
- notifyMentions keeps thread.AuthorID as the actor. Accepted.

Non-goals: admin editing of comments; 'edited by admin' attribution; any change to comment action rows; any DB schema change.

## Acceptance Criteria

Admin sees 'edit · delete' in the meta line on any live thread and can save an edit; a non-author non-admin gets 403 on GET and POST /threads/{id}/edit; author behaviour unchanged; tests cover admin edit and non-admin 403.

