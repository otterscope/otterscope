-- Alert rules with firing state, so the watcher notifies on transitions
-- rather than re-spamming every evaluation.
CREATE TABLE alerts (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    project       TEXT NOT NULL DEFAULT 'default',
    name          TEXT NOT NULL,
    type          TEXT NOT NULL,          -- error_rate | cost | p95_latency | assertion_fail_rate
    threshold     REAL NOT NULL,
    window_secs   INTEGER NOT NULL DEFAULT 3600,
    config        TEXT NOT NULL DEFAULT '', -- e.g. assertion name for assertion_fail_rate
    webhook_url   TEXT NOT NULL,
    enabled       INTEGER NOT NULL DEFAULT 1,
    firing        INTEGER NOT NULL DEFAULT 0,
    last_fired_ns INTEGER NOT NULL DEFAULT 0,
    created_ns    INTEGER NOT NULL,
    UNIQUE (project, name)
);
