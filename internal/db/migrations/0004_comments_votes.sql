-- Comments and votes.
--
-- Comments nest through parent_id with no depth limit. All comments of a
-- thread are loaded in one query and assembled into a tree in memory. Comments
-- are softly deleted so replies keep their place; rendering decides whether a
-- deleted comment is shown.
--
-- score starts at 1 for the author's implicit vote and has no vote row. A vote
-- row adds 1. A user can vote at most once on an item, and cannot vote on their
-- own posts.

CREATE TABLE comments (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    thread_id  INTEGER NOT NULL REFERENCES threads(id),
    parent_id  INTEGER REFERENCES comments(id),
    author_id  INTEGER NOT NULL REFERENCES users(id),
    body       TEXT NOT NULL,
    body_html  TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    edited_at  INTEGER,
    deleted_at INTEGER,
    score      INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX comments_thread_idx ON comments(thread_id, deleted_at);
CREATE INDEX comments_parent_idx ON comments(parent_id);
CREATE INDEX comments_author_idx ON comments(author_id);

CREATE TABLE votes (
    user_id   INTEGER NOT NULL REFERENCES users(id),
    item_type TEXT NOT NULL CHECK (item_type IN ('thread', 'comment')),
    item_id   INTEGER NOT NULL,
    PRIMARY KEY (user_id, item_type, item_id)
);

CREATE INDEX votes_item_idx ON votes(item_type, item_id);
