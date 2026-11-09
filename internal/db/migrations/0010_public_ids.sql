-- Public IDs for threads and comments.
--
-- Threads and comments keep their integer primary key for internal references
-- (foreign keys, votes, notifications, search and hub topics). The random
-- public_id is what clients see in URLs, anchors and live-update payloads, so
-- content cannot be enumerated by incrementing an integer.
--
-- SQLite cannot add a NOT NULL UNIQUE column directly, so the column starts
-- empty and is backfilled. Existing rows use eight hex characters, which are a
-- subset of the Go alphabet and have collisions too unlikely to matter for
-- rows this migration will see. New rows are given a Go-generated ID; the
-- triggers reject an insert that omits one.

ALTER TABLE threads ADD COLUMN public_id TEXT NOT NULL DEFAULT '';
UPDATE threads SET public_id = upper(hex(randomblob(4)));
CREATE UNIQUE INDEX threads_public_id_idx ON threads(public_id);

CREATE TRIGGER threads_public_id_required BEFORE INSERT ON threads
WHEN NEW.public_id = '' BEGIN
    SELECT RAISE(ABORT, 'threads.public_id is required');
END;

ALTER TABLE comments ADD COLUMN public_id TEXT NOT NULL DEFAULT '';
UPDATE comments SET public_id = upper(hex(randomblob(4)));
CREATE UNIQUE INDEX comments_public_id_idx ON comments(public_id);

CREATE TRIGGER comments_public_id_required BEFORE INSERT ON comments
WHEN NEW.public_id = '' BEGIN
    SELECT RAISE(ABORT, 'comments.public_id is required');
END;
