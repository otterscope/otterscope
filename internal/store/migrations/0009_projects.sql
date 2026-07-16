-- Projects: routing + isolation dimension for multi-agent installs.
-- The keyless 'default' project keeps zero-config ingest working.
CREATE TABLE projects (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL UNIQUE,
    ingest_key TEXT NOT NULL UNIQUE,
    created_ns INTEGER NOT NULL
);

INSERT INTO projects (name, ingest_key, created_ns) VALUES ('default', '', 0);

ALTER TABLE steps ADD COLUMN project TEXT NOT NULL DEFAULT 'default';
ALTER TABLE runs  ADD COLUMN project TEXT NOT NULL DEFAULT 'default';

CREATE INDEX runs_project_idx ON runs (project, start_ns DESC);

-- Renormalize rebuilds steps from raw batches, so batches must remember
-- which project their ingest key resolved to.
ALTER TABLE raw_batches ADD COLUMN project TEXT NOT NULL DEFAULT 'default';
