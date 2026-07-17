-- Prompt identity (name/version) for A/B prompt-regression tracking.
ALTER TABLE steps ADD COLUMN prompt   TEXT NOT NULL DEFAULT '';
ALTER TABLE runs  ADD COLUMN prompts  TEXT NOT NULL DEFAULT '';
