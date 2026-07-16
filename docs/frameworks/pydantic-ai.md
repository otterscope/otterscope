# Pydantic AI → Otterscope

Pydantic AI emits OpenTelemetry natively — no extra instrumentation
package needed.

```sh
pip install pydantic-ai opentelemetry-sdk opentelemetry-exporter-otlp-proto-http
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
```

```python
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry import trace

from pydantic_ai import Agent
from pydantic_ai.models.instrumented import InstrumentationSettings

provider = TracerProvider()
provider.add_span_processor(BatchSpanProcessor(OTLPSpanExporter()))
trace.set_tracer_provider(provider)

agent = Agent(
    "anthropic:claude-sonnet-5",
    # include_content records prompts/completions so the inspector can
    # show conversations; omit it to keep content out of telemetry
    instrument=InstrumentationSettings(include_content=True),
)

result = agent.run_sync("Where is my order A-1042?")
```

Run your agent, then open http://localhost:8317 — the run appears within a
couple of seconds (BatchSpanProcessor flushes periodically; call
`provider.force_flush()` in short-lived scripts).
