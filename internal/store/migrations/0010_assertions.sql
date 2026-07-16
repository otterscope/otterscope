-- Evals fused into the trace store: assertions are defined per project and
-- their results live on the run.
CREATE TABLE assertions (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    project    TEXT NOT NULL DEFAULT 'default',
    name       TEXT NOT NULL,
    type       TEXT NOT NULL,
    config     TEXT NOT NULL DEFAULT '',
    enabled    INTEGER NOT NULL DEFAULT 1,
    created_ns INTEGER NOT NULL,
    UNIQUE (project, name)
);

CREATE TABLE assertion_results (
    run_id       TEXT NOT NULL,
    assertion_id INTEGER NOT NULL,
    pass         INTEGER NOT NULL,
    detail       TEXT NOT NULL DEFAULT '',
    evaluated_ns INTEGER NOT NULL,
    PRIMARY KEY (run_id, assertion_id)
);

CREATE INDEX assertion_results_assertion_idx ON assertion_results (assertion_id, pass);
