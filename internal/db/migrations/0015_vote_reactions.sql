-- Emoji reactions on comment votes.
--
-- A reaction is a labelled upvote, not a separate object: one vote row per
-- member per item stays the rule, so a member cannot vote and then react with
-- a second row. reaction is a short ASCII key from the fixed set in
-- internal/db/votes.go, or NULL for a plain upvote. Only comment votes carry a
-- reaction, so thread votes always have NULL.
--
-- The existing votes_item_idx covers the reaction counts, which filter by
-- item_type and item_id.

ALTER TABLE votes ADD COLUMN reaction TEXT;
