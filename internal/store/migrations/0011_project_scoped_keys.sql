-- Rekey steps and runs by (project, id) so one project cannot overwrite
-- another's data by forging trace/span IDs (audit #49). SQLite can't add a
-- column to a PK in place, so rebuild both tables.
CREATE TABLE steps_new (
    project        TEXT NOT NULL DEFAULT 'default',
    id             TEXT NOT NULL,
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
    provider       TEXT NOT NULL DEFAULT '',
    request_model  TEXT NOT NULL DEFAULT '',
    response_model TEXT NOT NULL DEFAULT '',
    input_tokens   INTEGER NOT NULL DEFAULT 0,
    output_tokens  INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens     INTEGER NOT NULL DEFAULT 0,
    cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
    reasoning_tokens      INTEGER NOT NULL DEFAULT 0,
    tool_name      TEXT NOT NULL DEFAULT '',
    tool_call_id   TEXT NOT NULL DEFAULT '',
    detail         TEXT NOT NULL DEFAULT '',
    cost_usd       REAL,
    PRIMARY KEY (project, id)
);

INSERT INTO steps_new
    SELECT project, id, run_id, parent_id, kind, name, service, agent_name,
           status, start_ns, end_ns, error, provider, request_model,
           response_model, input_tokens, output_tokens, cache_read_tokens,
           cache_creation_tokens, reasoning_tokens, tool_name, tool_call_id,
           detail, cost_usd
    FROM steps;

DROP TABLE steps;
ALTER TABLE steps_new RENAME TO steps;
CREATE INDEX steps_run_idx ON steps (project, run_id, start_ns);

CREATE TABLE runs_new (
    project       TEXT NOT NULL DEFAULT 'default',
    id            TEXT NOT NULL,
    service       TEXT NOT NULL DEFAULT '',
    agent_name    TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'running',
    start_ns      INTEGER NOT NULL,
    end_ns        INTEGER NOT NULL DEFAULT 0,
    input_tokens  INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    llm_calls     INTEGER NOT NULL DEFAULT 0,
    tool_calls    INTEGER NOT NULL DEFAULT 0,
    models        TEXT NOT NULL DEFAULT '',
    cost_usd      REAL,
    cost_partial  INTEGER NOT NULL DEFAULT 0,
    error         TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (project, id)
);

INSERT INTO runs_new
    SELECT project, id, service, agent_name, status, start_ns, end_ns,
           input_tokens, output_tokens, llm_calls, tool_calls, models,
           cost_usd, cost_partial, error
    FROM runs;

DROP TABLE runs;
ALTER TABLE runs_new RENAME TO runs;
CREATE INDEX runs_start_idx ON runs (start_ns DESC);
CREATE INDEX runs_status_idx ON runs (status, start_ns DESC);
CREATE INDEX runs_service_idx ON runs (service, start_ns DESC);
CREATE INDEX runs_project_idx ON runs (project, start_ns DESC);
