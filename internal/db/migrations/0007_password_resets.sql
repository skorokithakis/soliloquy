-- Password reset links.
--
-- An admin creates a link for a user. Like invites, each token is random,
-- single-use and expires seven days after creation. Using a link changes the
-- user's password and ends all of their sessions. Old rows stay for the audit
-- trail; only used_at is set.

CREATE TABLE password_resets (
    token      TEXT PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    used_at    INTEGER
);

CREATE INDEX password_resets_user_id_idx ON password_resets(user_id);
CREATE INDEX password_resets_expires_at_idx ON password_resets(expires_at);
