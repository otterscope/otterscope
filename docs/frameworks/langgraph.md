# LangGraph / LangChain → Otterscope

Use the OpenInference instrumentor (broadest span coverage for LangChain
runnables and LangGraph nodes).

```sh
pip install openinference-instrumentation-langchain \
    opentelemetry-sdk opentelemetry-exporter-otlp-proto-http
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
```

```python
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from openinference.instrumentation.langchain import LangChainInstrumentor

provider = TracerProvider()
provider.add_span_processor(BatchSpanProcessor(OTLPSpanExporter()))
LangChainInstrumentor().instrument(tracer_provider=provider)

# your normal LangGraph graph from here
```

Graph nodes appear as generic/chain steps, model calls as LLM steps with
messages, tool nodes as tool steps.
