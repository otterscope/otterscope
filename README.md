# Otterscope

**Lightweight, self-hosted observability and evals for AI agents.**

One static binary. One SQLite file. No ClickHouse, no Redis, no S3, no seat fees, no trace metering. Point your agent's OpenTelemetry exporter at Otterscope and get an agent-run-first view of everything it did — LLM calls, tool calls, loops, errors, latency, and cost — stored on your own disk with unlimited retention.

> Think *Plausible Analytics, but for AI agents*.

## Why

Existing LLM observability stacks are built for enterprises:

- **Langfuse** self-hosting requires Postgres + ClickHouse + Redis + S3 — six containers to log a few thousand LLM calls a day.
- **LangSmith** and **Braintrust** gate self-hosting behind enterprise contracts.
- **Phoenix** is ELv2-licensed with an upsell funnel.
- Generic OTel dashboards show you span soup, not agent runs.

Small teams and individuals running agents in production need something that installs in one command, speaks OpenTelemetry, and answers the questions that actually matter: *What did this run do? Where did it loop? What did it cost? Did quality regress?*

## What it does

- **OTLP-native ingestion** — drop-in `OTEL_EXPORTER_OTLP_ENDPOINT` compatibility. Normalizes the OTel GenAI semantic conventions (both current dialects) and OpenInference, so traces from the OpenAI Agents SDK, LangGraph, Pydantic AI, Vercel AI SDK, CrewAI, and hand-rolled agents all land coherently.
- **Agent-run-first data model** — runs → steps → LLM/tool calls are the primary objects, not raw spans. See tool loops, per-run cost, and failure points at a glance.
- **Evals fused into the trace store** — score live production runs with assertions and LLM-as-judge, compare across time and versions. No second product, no per-score fees.
- **Single binary, embedded SQLite** — `otterscope serve` and you're done. Also available as a single Docker container.

## Status

Early development — not yet usable. Follow the [roadmap](docs/ROADMAP.md).

## Quick start (target UX)

```sh
otterscope serve
# then in your agent's environment:
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
```

## License

[Apache-2.0](LICENSE)
