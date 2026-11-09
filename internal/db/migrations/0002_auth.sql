-- Accounts, sessions and invites.
--
-- Timestamps are Unix seconds in UTC. Users are softly deleted: the row stays
-- so the username remains reserved and content keeps its author.

CREATE TABLE users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT NOT NULL COLLATE NOCASE UNIQUE,
    password_hash TEXT NOT NULL,
    is_admin      INTEGER NOT NULL DEFAULT 0 CHECK (is_admin IN (0, 1)),
    invited_by    INTEGER REFERENCES users(id),
    created_at    INTEGER NOT NULL,
    disabled_at   INTEGER,
    deleted_at    INTEGER
);

CREATE INDEX users_invited_by_idx ON users(invited_by);

CREATE TABLE sessions (
    token_hash TEXT PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);

CREATE INDEX sessions_user_id_idx ON sessions(user_id);
CREATE INDEX sessions_expires_at_idx ON sessions(expires_at);

-- created_by is NULL for the bootstrap admin invite that is printed to the log
-- when the database has no users yet. Normal invites always have a creator.
CREATE TABLE invites (
    token      TEXT PRIMARY KEY,
    created_by INTEGER REFERENCES users(id) ON DELETE CASCADE,
    is_admin   INTEGER NOT NULL DEFAULT 0 CHECK (is_admin IN (0, 1)),
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    used_at    INTEGER,
    used_by    INTEGER REFERENCES users(id)
);

CREATE INDEX invites_created_by_idx ON invites(created_by);
CREATE INDEX invites_expires_at_idx ON invites(expires_at);
