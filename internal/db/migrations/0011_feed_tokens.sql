-- Per-user secret tokens for the RSS feeds.
--
-- The token is stored in plain text on purpose. It only grants read-only
-- access to content that is already in the database, and storing it raw lets
-- the account page show the feed URLs again. A unique index keeps one token
-- from belonging to two users; NULL means no token has been created yet.
-- SQLite allows any number of NULLs in a unique index.

ALTER TABLE users ADD COLUMN feed_token TEXT;

CREATE UNIQUE INDEX users_feed_token_idx ON users(feed_token);
