-- Scope assertion results by project (#98). #49 rekeyed runs and steps by
-- (project, id) but assertion_results was left keyed on run_id alone, so two
-- projects holding the same trace id shared one row: whichever project was
-- evaluated last overwrote the other's verdict, and the stats join
-- (runs.id = ar.run_id) fanned out across projects and double-counted.
--
-- Backfill takes the project from the assertion that produced the result:
-- assertion_id → assertions.project is unambiguous, whereas run_id → runs is
-- exactly the ambiguity being fixed. Results whose assertion has since been
-- deleted are dropped; DeleteAssertion already removes results, so those are
-- orphans that can no longer be displayed.
CREATE TABLE assertion_results_new (
    project      TEXT NOT NULL DEFAULT 'default',
    run_id       TEXT NOT NULL,
    assertion_id INTEGER NOT NULL,
    pass         INTEGER NOT NULL,
    detail       TEXT NOT NULL DEFAULT '',
    evaluated_ns INTEGER NOT NULL,
    PRIMARY KEY (project, run_id, assertion_id)
);

INSERT OR REPLACE INTO assertion_results_new
    SELECT a.project, ar.run_id, ar.assertion_id, ar.pass, ar.detail, ar.evaluated_ns
    FROM assertion_results ar
    JOIN assertions a ON a.id = ar.assertion_id;

DROP TABLE assertion_results;
ALTER TABLE assertion_results_new RENAME TO assertion_results;

CREATE INDEX assertion_results_assertion_idx ON assertion_results (assertion_id, pass);
CREATE INDEX assertion_results_run_idx ON assertion_results (project, run_id);
