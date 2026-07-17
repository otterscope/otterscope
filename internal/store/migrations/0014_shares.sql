-- Opt-in public read-only shares of a single run. The token is the only
-- capability; it maps to exactly one (project, run_id).
CREATE TABLE shared_runs (
    token      TEXT PRIMARY KEY,
    project    TEXT NOT NULL,
    run_id     TEXT NOT NULL,
    created_ns INTEGER NOT NULL
);

CREATE INDEX shared_runs_run_idx ON shared_runs (project, run_id);
