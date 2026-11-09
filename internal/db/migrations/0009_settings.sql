-- Site-wide settings edited by admins. Missing rows fall back to the defaults
-- in internal/db, so a fresh database has no rows here.

CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
