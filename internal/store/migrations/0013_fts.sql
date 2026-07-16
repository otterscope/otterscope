-- Full-text search over step content (messages, tool i/o, names). Standalone
-- FTS5 table kept in sync from the step write path; backfilled here from
-- existing rows (detail holds message/tool JSON, so its words are indexed).
CREATE VIRTUAL TABLE steps_fts USING fts5(
    project UNINDEXED,
    run_id  UNINDEXED,
    step_id UNINDEXED,
    content
);

INSERT INTO steps_fts (project, run_id, step_id, content)
    SELECT CASE WHEN project = '' THEN 'default' ELSE project END,
           run_id, id,
           name || ' ' || provider || ' ' || request_model || ' ' ||
           tool_name || ' ' || tool_call_id || ' ' || detail
    FROM steps;
