# OpenAI Agents SDK → Otterscope

The Agents SDK is instrumented via OpenInference, which Otterscope
normalizes natively.

```sh
pip install openai-agents openinference-instrumentation-openai-agents \
    opentelemetry-sdk opentelemetry-exporter-otlp-proto-http
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
```

```python
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from openinference.instrumentation.openai_agents import OpenAIAgentsInstrumentor

provider = TracerProvider()
provider.add_span_processor(BatchSpanProcessor(OTLPSpanExporter()))
OpenAIAgentsInstrumentor().instrument(tracer_provider=provider)

# your normal Agents SDK code from here
from agents import Agent, Runner

agent = Agent(name="Triage Agent", instructions="Route support questions.")
result = Runner.run_sync(agent, "Where is my order A-1042?")
```

Agent handoffs show up as tool steps, guardrails as generic steps, and the
run inspector shows each LLM call's messages and tool calls.
