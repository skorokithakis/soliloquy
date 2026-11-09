-- Threads.
--
-- The body is stored twice: the Markdown source for editing, and sanitized
-- HTML rendered when the thread is saved. Threads are softly deleted so their
-- rows, and their comments once those exist, stay in the database.
--
-- score starts at 1 for the author's implicit vote and has no vote row. Vote
-- code adjusts it. last_activity_at is the newest comment time, or created_at
-- when there are no comments, and comment code moves it forward. The comment
-- count is computed from the comments table when threads are read, so no
-- denormalized counter needs to be kept in step.

CREATE TABLE threads (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    author_id        INTEGER NOT NULL REFERENCES users(id),
    title            TEXT NOT NULL,
    url              TEXT NOT NULL DEFAULT '',
    body             TEXT NOT NULL DEFAULT '',
    body_html        TEXT NOT NULL DEFAULT '',
    created_at       INTEGER NOT NULL,
    edited_at        INTEGER,
    deleted_at       INTEGER,
    last_activity_at INTEGER NOT NULL,
    score            INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX threads_activity_idx ON threads(deleted_at, last_activity_at DESC, id DESC);
CREATE INDEX threads_author_idx ON threads(author_id);
