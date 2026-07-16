-- Distinct request models used by a run, comma-joined — the runs list shows
-- model identity without a per-row join against steps.
ALTER TABLE runs ADD COLUMN models TEXT NOT NULL DEFAULT '';
