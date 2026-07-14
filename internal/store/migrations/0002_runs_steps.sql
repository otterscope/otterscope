-- Runs (traces) and steps (spans), normalized by internal/ingest.
-- Flat columns rather than JSON blobs: M3's filters and cost rollups query
-- these directly. Times are unix nanoseconds.
CREATE TABLE runs (
    id            TEXT PRIMARY KEY,
    service       TEXT NOT NULL DEFAULT '',
    agent_name    TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'running',
    start_ns      INTEGER NOT NULL,
    end_ns        INTEGER NOT NULL DEFAULT 0,
    input_tokens  INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    llm_calls     INTEGER NOT NULL DEFAULT 0,
    tool_calls    INTEGER NOT NULL DEFAULT 0,
    error         TEXT NOT NULL DEFAULT ''
);

CREATE INDEX runs_start_idx ON runs (start_ns DESC);
CREATE INDEX runs_status_idx ON runs (status, start_ns DESC);

CREATE TABLE steps (
    id             TEXT PRIMARY KEY,
    run_id         TEXT NOT NULL,
    parent_id      TEXT NOT NULL DEFAULT '',
    kind           TEXT NOT NULL,
    name           TEXT NOT NULL,
    service        TEXT NOT NULL DEFAULT '',
    agent_name     TEXT NOT NULL DEFAULT '',
    status         TEXT NOT NULL,
    start_ns       INTEGER NOT NULL,
    end_ns         INTEGER NOT NULL,
    error          TEXT NOT NULL DEFAULT '',
    -- llm steps
    provider       TEXT NOT NULL DEFAULT '',
    request_model  TEXT NOT NULL DEFAULT '',
    response_model TEXT NOT NULL DEFAULT '',
    input_tokens   INTEGER NOT NULL DEFAULT 0,
    output_tokens  INTEGER NOT NULL DEFAULT 0,
    -- tool steps
    tool_name      TEXT NOT NULL DEFAULT '',
    tool_call_id   TEXT NOT NULL DEFAULT ''
);

CREATE INDEX steps_run_idx ON steps (run_id, start_ns);
