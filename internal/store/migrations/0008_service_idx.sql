-- Filtering by service is a first-class list operation (M3).
CREATE INDEX runs_service_idx ON runs (service, start_ns DESC);
