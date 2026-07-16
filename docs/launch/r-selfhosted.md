# r/selfhosted draft (Kalin posts)

**Title:** Otterscope — observability + evals for AI agents in a single
binary + SQLite (no ClickHouse stack)

**Text:**

If you run LLM agents and want to see what they're doing without shipping
your prompts to a SaaS — I built Otterscope for exactly that.

- **Deploy:** one static binary (`./otterscope serve`) or one Docker
  container. Data is a single SQLite file in a volume. That's the whole
  stack.
- **Ingest:** standard OpenTelemetry — works with Pydantic AI, OpenAI
  Agents SDK, LangGraph, Vercel AI SDK, or hand-rolled OTLP.
- **Get:** live run list with filters, a per-run timeline (LLM calls with
  full messages, tool calls with args/results), token + cost tracking,
  assertions & LLM-judge evals on your real traffic, and a side-by-side
  compare view.
- **Retention:** unlimited by default (it's your disk); optional
  `-retention` sweep if you want cleanup.
- Apache-2.0. Multi-project ingest keys if you run several agents.

Compared to Langfuse self-hosted (which is solid but wants
Postgres + ClickHouse + Redis + S3), this is deliberately the
small-instance option: if your volume is "my agents", not "my company's
agents", the six-container stack is overkill.

GitHub: https://github.com/otterscope/otterscope — feedback very welcome,
especially on frameworks whose traces don't normalize cleanly yet (raw
payloads are retained, so fixes apply retroactively).
