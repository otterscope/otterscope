# Show HN draft (Kalin posts; edit voice as you like)

**Title:** Show HN: Otterscope – self-hosted AI agent observability in one binary

**Text:**

I kept wanting to see what my agents were actually doing in production —
which tool loops burned money, where runs derailed, whether a prompt change
made things worse. The self-hostable options wanted a ClickHouse + Redis +
S3 stack for what is, at my scale, a few thousand LLM calls a day; the
hosted ones want my users' conversations on their servers.

So: Otterscope. One static Go binary, one SQLite file. You point any
OpenTelemetry-instrumented agent at it (`OTEL_EXPORTER_OTLP_ENDPOINT`) and
get an agent-run-first view — runs → steps → LLM/tool calls with messages,
tokens, and cost — plus assertions and LLM-as-judge evals scored onto real
runs, and a compare view for "did this week get worse than last week".

Design choices that might interest HN:

- It normalizes three trace dialects (OTel GenAI old + experimental,
  OpenInference, Vercel AI SDK) because the ecosystem hasn't converged.
  Raw payloads are retained so old data re-normalizes as the conventions
  stabilize.
- Costs come from a maintained pricing table; unknown models show tokens
  and an explicit "partial" flag rather than a fabricated total.
- Judge evals call any OpenAI-compatible endpoint with a key from your
  env — keys never touch the database.
- Apache-2.0, no metering, no seat fees. If it ever grows a paid thing it
  will be managed hosting, never features clawed back.

Repo: https://github.com/otterscope/otterscope

Happy to answer anything about the SQLite schema, the dialect-normalization
mess, or why this isn't "just Grafana".
