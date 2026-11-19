-- Thread follows and the 'followed' notification kind.
--
-- thread_follows records which members follow which threads. Like thread_reads,
-- the thread is keyed by its internal ID and may be soft-deleted, so a row can
-- outlive the thread's visibility and is only removed when the member's account
-- is. Disabling a member keeps the row; inactive members receive no
-- notifications.
--
-- The notifications kind CHECK cannot be altered in place, so the table is
-- rebuilt. Every row, both indexes and the AUTOINCREMENT high-water mark are
-- kept: notification IDs appear in feed GUIDs and must never be reused.

CREATE TABLE thread_follows (
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    thread_id  INTEGER NOT NULL REFERENCES threads(id),
    created_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, thread_id)
);

-- thread_follows is read per thread when a comment is posted; the primary key
-- starts with user_id, so it does not help that lookup.
CREATE INDEX thread_follows_thread_idx ON thread_follows(thread_id);

-- Save the sequence before dropping the table, which deletes its
-- sqlite_sequence row. The value can be higher than MAX(id) when the newest
-- notifications were deleted.
CREATE TEMP TABLE notifications_sequence AS
SELECT COALESCE((SELECT seq FROM sqlite_sequence WHERE name = 'notifications'), 0) AS seq;

CREATE TABLE notifications_new (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    recipient_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL CHECK (kind IN ('reply', 'thread_comment', 'mention', 'followed')),
    actor_id     INTEGER NOT NULL REFERENCES users(id),
    thread_id    INTEGER NOT NULL REFERENCES threads(id),
    comment_id   INTEGER REFERENCES comments(id),
    created_at   INTEGER NOT NULL,
    read_at      INTEGER
);

INSERT INTO notifications_new (
    id, recipient_id, kind, actor_id, thread_id, comment_id, created_at, read_at
)
SELECT id, recipient_id, kind, actor_id, thread_id, comment_id, created_at, read_at
FROM notifications;

DROP TABLE notifications;
ALTER TABLE notifications_new RENAME TO notifications;

CREATE INDEX notifications_recipient_created_idx
    ON notifications(recipient_id, created_at DESC);

CREATE INDEX notifications_recipient_unread_idx
    ON notifications(recipient_id) WHERE read_at IS NULL;

-- Restore the high-water mark. The rename carries over the new table's
-- sequence, which is only MAX(id) after the copy. That is lower than the old
-- mark when the newest notifications were deleted before this migration;
-- taking the larger of the two keeps the next ID from being reused.
DELETE FROM sqlite_sequence WHERE name = 'notifications';
INSERT INTO sqlite_sequence (name, seq)
SELECT 'notifications', MAX(
    (SELECT seq FROM notifications_sequence),
    (SELECT COALESCE(MAX(id), 0) FROM notifications)
);

DROP TABLE notifications_sequence;
