# Claude Agent SDK / Claude Code → Otterscope

The Agent SDK runs the Claude Code CLI as a child process, and the CLI has
OpenTelemetry built in. The SDK emits no telemetry itself — it passes
configuration through, and the CLI exports to your collector directly. So the
same setup works for `claude -p` on the command line.

**Traces are opt-in twice.** `CLAUDE_CODE_ENABLE_TELEMETRY=1` turns telemetry
on, but spans additionally need `CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1`.
Without the second flag you get metrics and log events and Otterscope stays
empty — this is the most common reason nothing shows up.

```sh
export CLAUDE_CODE_ENABLE_TELEMETRY=1
export CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1   # required for spans
export OTEL_TRACES_EXPORTER=otlp
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
export OTEL_SERVICE_NAME=my-agent              # default is "claude-code"

# only when you created a project key:
export OTEL_EXPORTER_OTLP_HEADERS="Authorization=Bearer <ingest-key>"

# tool inputs/outputs in the run inspector (see Content, below)
export OTEL_LOG_TOOL_DETAILS=1
export OTEL_LOG_TOOL_CONTENT=1
```

That is the whole integration — run your agent and the run appears at
http://localhost:8317.

Set `OTEL_METRICS_EXPORTER` / `OTEL_LOGS_EXPORTER` only if you also run a
metrics/logs backend. Otterscope ingests traces; it does not accept the
metrics or logs protocols, and pointing them at `:4318` just logs export
errors.

## From Python or TypeScript

Environment variables set before your process starts are inherited by the CLI,
which is the simplest option. To scope telemetry per call instead:

```python
from claude_agent_sdk import query, ClaudeAgentOptions

OTEL = {
    "CLAUDE_CODE_ENABLE_TELEMETRY": "1",
    "CLAUDE_CODE_ENHANCED_TELEMETRY_BETA": "1",
    "OTEL_TRACES_EXPORTER": "otlp",
    "OTEL_EXPORTER_OTLP_PROTOCOL": "http/protobuf",
    "OTEL_EXPORTER_OTLP_ENDPOINT": "http://localhost:4318",
    "OTEL_SERVICE_NAME": "my-agent",
    # short scripts exit before the default 5s export tick
    "OTEL_TRACES_EXPORT_INTERVAL": "1000",
}

async for message in query(
    prompt="Find and fix the failing test",
    options=ClaudeAgentOptions(env=OTEL),
):
    print(message)
```

```ts
import { query } from "@anthropic-ai/claude-agent-sdk";

// In TypeScript `env` REPLACES the inherited environment — spread process.env
// first or the child loses PATH and ANTHROPIC_API_KEY.
const env = { ...process.env, CLAUDE_CODE_ENABLE_TELEMETRY: "1", /* ...as above */ };

for await (const message of query({ prompt: "…", options: { env } })) {
  console.log(message);
}
```

**Never set an exporter to `console` under the SDK.** The SDK uses stdout as
its message channel, and console telemetry corrupts it. To eyeball telemetry
locally, point the endpoint at Otterscope.

## What a run looks like

Claude Code's spans become Otterscope steps like this:

| Span | Otterscope step |
|---|---|
| `claude_code.interaction` | the run root — one agent turn |
| `claude_code.llm_request` | an **LLM call**: model, tokens, cache tokens, cost |
| `claude_code.tool` | a **tool call**: name, `tool_use_id`, input/output |
| `claude_code.tool.blocked_on_user` | generic step — time awaiting a permission decision |
| `claude_code.tool.execution` | generic step — time actually running the tool |
| `claude_code.hook` | generic step (needs detailed beta tracing) |

Subagents nest: an Agent-tool span contains the subagent's own model and tool
spans, so the whole delegation chain is one run and one timeline.

Cache tokens are reported here under bare keys (`cache_read_tokens`,
`cache_creation_tokens`) rather than `gen_ai.usage.*`. Otterscope reads them,
which matters because a Claude Code run is usually mostly cache reads — cost
would be overstated several-fold otherwise.

A failed API call reports `success="false"` with an `error` attribute while
often leaving the span status unset. Otterscope marks those steps failed
anyway, so retried/overloaded requests show up as errors rather than as
suspiciously fast successes.

## Content

Traces are structural by default: durations, model names, tool names, and
token counts, but no prompt or file content. Two flags add content that
reaches Otterscope:

- `OTEL_LOG_TOOL_DETAILS=1` — the command, file path, or skill name a tool was
  called with.
- `OTEL_LOG_TOOL_CONTENT=1` — full tool input and output bodies (truncated at
  60 KB), which is what fills in the tool inspector.

**Conversation content is not available over traces at all.** Claude Code
sends prompts and API bodies as OTLP *log records* (`OTEL_LOG_USER_PROMPTS`,
`OTEL_LOG_RAW_API_BODIES`), not spans, and Otterscope ingests traces only. So
a Claude Code run shows the full structure, timing, tokens and cost, and tool
i/o — but the LLM message inspector stays empty. Frameworks in the other
guides here put messages on spans, so those runs do show conversations.
Tracked in [#119](https://github.com/otterscope/otterscope/issues/119).

Leave the content flags off unless Otterscope's disk is an acceptable place
for whatever your agent reads.

## Caveats

- **Tracing is beta upstream.** Span names and attributes may change between
  Claude Code releases. Otterscope keeps every raw payload, so run
  `otterscope renormalize` after upgrading Otterscope to re-apply an improved
  normalizer to traces you already collected.
- **Export is silent on failure.** If the endpoint is unreachable the agent
  runs normally and the telemetry is dropped with no error. Set
  `CLAUDE_CODE_OTEL_DIAG_STDERR=1` to surface exporter errors.
- **Short-lived processes lose spans.** Traces export every 5 seconds by
  default; lower `OTEL_TRACES_EXPORT_INTERVAL` for scripts that finish faster
  than that.
- Spans carry a `session.id`; several `query()` calls against one session are
  separate Otterscope runs unless your application wraps them in a span of its
  own (the SDK propagates `TRACEPARENT` into the CLI, so an outer span makes
  them one run).
