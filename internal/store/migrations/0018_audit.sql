-- Change history: what mutated and when (who/actor lands with auth, #59).
CREATE TABLE audit_log (
    id     INTEGER PRIMARY KEY AUTOINCREMENT,
    at_ns  INTEGER NOT NULL,
    action TEXT NOT NULL,          -- create | delete
    entity TEXT NOT NULL,          -- assertion | alert | project | token | view | share
    detail TEXT NOT NULL DEFAULT ''
);
CREATE INDEX audit_log_at_idx ON audit_log (at_ns DESC);
