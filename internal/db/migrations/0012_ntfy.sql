-- Per-user ntfy push notification settings.
--
-- The topic URL is a secret: anyone who knows the topic name can read its
-- messages, so the server never includes it in errors or logs. An empty URL
-- turns pushes off. Pushes for personal notifications (replies, comments on
-- your threads and mentions) are on by default; pushes for every new thread
-- are off, because they can be noisy.

ALTER TABLE users ADD COLUMN ntfy_url TEXT NOT NULL DEFAULT '';

ALTER TABLE users ADD COLUMN ntfy_replies INTEGER NOT NULL DEFAULT 1
    CHECK (ntfy_replies IN (0, 1));

ALTER TABLE users ADD COLUMN ntfy_threads INTEGER NOT NULL DEFAULT 0
    CHECK (ntfy_threads IN (0, 1));
