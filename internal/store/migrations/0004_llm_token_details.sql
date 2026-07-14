-- Token subsets from the gen_ai_latest_experimental dialect, required for
-- accurate cost math in M3 (cached input is billed differently).
ALTER TABLE steps ADD COLUMN cache_read_tokens     INTEGER NOT NULL DEFAULT 0;
ALTER TABLE steps ADD COLUMN cache_creation_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE steps ADD COLUMN reasoning_tokens      INTEGER NOT NULL DEFAULT 0;
