# Roadmap

Dates assume start 2026-07-14. Milestones map 1:1 to GitHub milestones; issues carry the detail. This file states *intent* — when reality diverges, update this file in the same PR that diverges.

## M0 — Foundation (week of Jul 14)
Repo, docs, CI, scaffold. `otterscope serve` starts, opens SQLite, serves a health endpoint and a placeholder UI.

## M1 — Ingest core (Jul 14 – Jul 27)
The moat. OTLP/HTTP receiver (protobuf + JSON) on `:4318`; normalization of OTel GenAI spans (old + `gen_ai_latest_experimental` dialects) and OpenInference into the domain model (Run → Step → LLMCall/ToolCall); SQLite schema + migrations; captured real-framework payloads as fixtures (Vercel AI SDK, Pydantic AI, OpenAI Agents SDK via OpenInference). Exit criterion: point a real instrumented agent at Otterscope and its runs land correctly in the DB.

## M2 — Run explorer UI (Jul 27 – Aug 10)
React UI embedded in the binary: runs list (status, duration, model, tokens, cost, error badge), run detail with step timeline, tool-loop visibility, LLM call inspector (messages in/out), live tail. Exit criterion: debugging a misbehaving agent via the UI is genuinely faster than reading logs.

## M3 — Cost, search, projects (Aug 10 – Aug 24)
Model-pricing table (maintained, overridable) → per-call/per-run cost; filter/search (model, status, time range, attribute); multiple projects with ingest API keys; retention config. Includes first debt-repayment pass.

## M4 — Evals v1 (Aug 24 – Sep 14)
Assertions (contains/regex/JSON-schema/latency/cost thresholds) and LLM-as-judge scoring over captured runs; score storage on the run; comparison view (this week vs last, version A vs B). Exit criterion: "did my prompt change make things worse?" answerable in-product.

## M5 — Ship it (Sep 14 – Sep 28)
Static binaries via goreleaser (linux/darwin/windows), official Docker image, install docs, demo instance with sample data, README polish with screenshots/GIF, docs for each supported framework. Launch: Show HN + r/selfhosted + awesome-selfhosted PR.

## Later / explicitly deferred
- DuckDB/Parquet analytics sidecar for large volumes (ADR-0002 keeps SQLite until proven insufficient)
- OTLP/gRPC receiver (HTTP covers the ecosystem; gRPC on demand)
- Alerting/webhooks, prompt management, multi-user auth/RBAC, managed cloud offering
