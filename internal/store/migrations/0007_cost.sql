-- Cost in USD, computed at ingest from the pricing table then in effect
-- (historical costs stay frozen; Renormalize recomputes with the current
-- table). NULL = unknown model or not an LLM step — never a fabricated 0.
ALTER TABLE steps ADD COLUMN cost_usd REAL;
ALTER TABLE runs ADD COLUMN cost_usd REAL;
-- 1 when at least one llm step with token usage had no known price, so the
-- UI can show the run cost as a lower bound instead of lying.
ALTER TABLE runs ADD COLUMN cost_partial INTEGER NOT NULL DEFAULT 0;
