-- In-app notifications.
--
-- A notification records who caused it (actor), where it happened (thread and
-- optionally the comment), and when it was read. Recipients are only active
-- users at creation time, but rows are kept if a user is later deleted or
-- disabled; the account-deletion task removes them explicitly.

CREATE TABLE notifications (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    recipient_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL CHECK (kind IN ('reply', 'thread_comment', 'mention')),
    actor_id     INTEGER NOT NULL REFERENCES users(id),
    thread_id    INTEGER NOT NULL REFERENCES threads(id),
    comment_id   INTEGER REFERENCES comments(id),
    created_at   INTEGER NOT NULL,
    read_at      INTEGER
);

CREATE INDEX notifications_recipient_created_idx
    ON notifications(recipient_id, created_at DESC);

CREATE INDEX notifications_recipient_unread_idx
    ON notifications(recipient_id) WHERE read_at IS NULL;
