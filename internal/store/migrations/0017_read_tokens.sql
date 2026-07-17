-- Bearer tokens for the read API (distinct from per-project ingest keys).
CREATE TABLE read_tokens (
    token      TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    created_ns INTEGER NOT NULL
);
