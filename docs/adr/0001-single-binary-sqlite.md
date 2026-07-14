# ADR-0001: Single static Go binary with embedded SQLite

Date: 2026-07-14 · Status: accepted

## Context
The competitive research (July 2026) shows the market gap for AI-agent observability is specifically *operational weight*: Langfuse v3 requires Postgres + ClickHouse + Redis + S3; Opik, SigNoz, OpenLIT are ClickHouse-backed; LangSmith/Braintrust gate self-hosting behind enterprise contracts. The users we target run a few thousand to a few hundred thousand LLM calls per day — SQLite-scale, not ClickHouse-scale.

## Decision
- Go, compiled as a single static binary (CGO disabled). SQLite via `modernc.org/sqlite` (pure Go).
- One embedded database file (WAL mode). The web UI is embedded via `go:embed`.
- The product identity **is** this constraint: any feature whose design requires an external service gets redesigned or rejected.

## Consequences
- Deployment is `./otterscope serve` or one Docker container — our core differentiator.
- Analytics queries are bounded by SQLite; acceptable at target scale. If it becomes a real limit, revisit with a DuckDB/Parquet layer (deferred, see ROADMAP), never a mandatory server dependency.
- `modernc.org/sqlite` is slower than CGO sqlite3 (~2x on some workloads) — the price of static builds; benchmark before optimizing.
