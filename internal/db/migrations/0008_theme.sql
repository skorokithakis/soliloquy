-- Per-user theme preference. The layout puts this value on the root element.
-- "auto" follows the operating system preference.

ALTER TABLE users ADD COLUMN theme TEXT NOT NULL DEFAULT 'auto'
    CHECK (theme IN ('auto', 'light', 'dark'));
