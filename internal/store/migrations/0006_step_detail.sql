-- Kind-specific step payloads as one JSON column: llm steps store
-- input/output messages, tool steps store arguments/result. Never filtered
-- in SQL — the run-detail inspector reads it whole — so a blob beats
-- per-field migrations.
ALTER TABLE steps ADD COLUMN detail TEXT NOT NULL DEFAULT '';
