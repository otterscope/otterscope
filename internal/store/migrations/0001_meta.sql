-- Instance metadata. Domain tables (runs, steps, calls) arrive with M1's
-- ingest schema migration.
CREATE TABLE meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT INTO meta (key, value) VALUES ('created_at', datetime('now'));
