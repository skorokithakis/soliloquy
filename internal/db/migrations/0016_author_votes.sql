-- The author's initial upvote as a real vote row.
--
-- New threads and comments insert this row with the item. Existing rows get
-- one here. Scores already counted the author's implicit vote, so the backfill
-- only adds rows and leaves every score alone.
--
-- The NOT EXISTS clauses leave an author's vote alone when it already exists,
-- so the migration is safe to apply to a database that has had votes written by
-- newer code.

INSERT INTO votes (user_id, item_type, item_id)
SELECT t.author_id, 'thread', t.id
FROM threads t
WHERE NOT EXISTS (
    SELECT 1 FROM votes v
    WHERE v.user_id = t.author_id
      AND v.item_type = 'thread'
      AND v.item_id = t.id
);

INSERT INTO votes (user_id, item_type, item_id)
SELECT c.author_id, 'comment', c.id
FROM comments c
WHERE NOT EXISTS (
    SELECT 1 FROM votes v
    WHERE v.user_id = c.author_id
      AND v.item_type = 'comment'
      AND v.item_id = c.id
);
