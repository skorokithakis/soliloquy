-- Full-text search over thread titles and bodies and comment bodies.
--
-- search_index is a regular FTS5 table (not external content) because it needs
-- to hold rows for two different source tables. kind, item_id and thread_id are
-- unindexed columns used to join results back to the source rows; title and
-- body are the searchable columns.
--
-- Triggers keep the index in step with the threads and comments tables. A
-- soft delete removes the affected rows from the index in the same transaction
-- as the update, so search never has to filter deleted content out of its own
-- rows (the query still checks the source tables as a safety net). Threads are
-- softly deleted, so the delete triggers watch deleted_at rather than DELETE.

CREATE VIRTUAL TABLE search_index USING fts5(
    kind UNINDEXED,
    item_id UNINDEXED,
    thread_id UNINDEXED,
    title,
    body
);

-- Backfill existing content. Deleted threads and comments are left out.
INSERT INTO search_index (kind, item_id, thread_id, title, body)
SELECT 'thread', id, id, title, body
FROM threads
WHERE deleted_at IS NULL;

INSERT INTO search_index (kind, item_id, thread_id, title, body)
SELECT 'comment', c.id, c.thread_id, '', c.body
FROM comments c
JOIN threads t ON t.id = c.thread_id
WHERE c.deleted_at IS NULL AND t.deleted_at IS NULL;

CREATE TRIGGER thread_search_insert AFTER INSERT ON threads BEGIN
    INSERT INTO search_index (kind, item_id, thread_id, title, body)
    VALUES ('thread', NEW.id, NEW.id, NEW.title, NEW.body);
END;

CREATE TRIGGER thread_search_update AFTER UPDATE OF title, body ON threads
WHEN NEW.deleted_at IS NULL BEGIN
    UPDATE search_index
    SET title = NEW.title, body = NEW.body
    WHERE kind = 'thread' AND item_id = NEW.id;
END;

CREATE TRIGGER thread_search_delete AFTER UPDATE OF deleted_at ON threads
WHEN NEW.deleted_at IS NOT NULL AND OLD.deleted_at IS NULL BEGIN
    DELETE FROM search_index WHERE kind = 'thread' AND item_id = NEW.id;
    DELETE FROM search_index WHERE kind = 'comment' AND thread_id = NEW.id;
END;

CREATE TRIGGER comment_search_insert AFTER INSERT ON comments BEGIN
    INSERT INTO search_index (kind, item_id, thread_id, title, body)
    VALUES ('comment', NEW.id, NEW.thread_id, '', NEW.body);
END;

CREATE TRIGGER comment_search_update AFTER UPDATE OF body ON comments
WHEN NEW.deleted_at IS NULL BEGIN
    UPDATE search_index
    SET body = NEW.body
    WHERE kind = 'comment' AND item_id = NEW.id;
END;

CREATE TRIGGER comment_search_delete AFTER UPDATE OF deleted_at ON comments
WHEN NEW.deleted_at IS NOT NULL AND OLD.deleted_at IS NULL BEGIN
    DELETE FROM search_index WHERE kind = 'comment' AND item_id = NEW.id;
END;
