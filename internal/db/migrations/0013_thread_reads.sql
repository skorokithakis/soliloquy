-- Per-member thread read state.
--
-- thread_reads records when each member last opened each thread. The thread is
-- keyed by its internal ID; threads are soft-deleted, so a row can outlive the
-- thread's visibility and is only removed when the member's account is.
--
-- users.unread_since is the point from which a member's unread threads are
-- counted: threads created after it that the member has never opened show as
-- unopened. Existing members get the migration time so that every thread they
-- could already have read counts as read. New accounts set the column when
-- they are created, so old rows keep their default only until then.

CREATE TABLE thread_reads (
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    thread_id    INTEGER NOT NULL REFERENCES threads(id),
    last_read_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, thread_id)
);

ALTER TABLE users ADD COLUMN unread_since INTEGER NOT NULL DEFAULT 0;
UPDATE users SET unread_since = CAST(strftime('%s', 'now') AS INTEGER);
