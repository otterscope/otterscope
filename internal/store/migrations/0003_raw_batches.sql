-- Raw ingested OTLP batches, retained so runs can be re-normalized after
-- normalizer improvements without data loss (ADR-0002). Payload is
-- gzip-compressed ExportTraceServiceRequest protobuf.
CREATE TABLE raw_batches (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    received_ns INTEGER NOT NULL,
    payload     BLOB NOT NULL
);

CREATE INDEX raw_batches_received_idx ON raw_batches (received_ns);
