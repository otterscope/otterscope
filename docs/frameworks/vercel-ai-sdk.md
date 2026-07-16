# Vercel AI SDK → Otterscope

The AI SDK's `experimental_telemetry` emits OTel spans that Otterscope
normalizes, including its `ai.*` wrapper/tool spans.

```sh
npm install @vercel/otel   # or wire @opentelemetry/sdk-node yourself
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
```

```ts
// instrumentation.ts (Next.js) — or any OTel NodeSDK setup
import { registerOTel } from "@vercel/otel";
export function register() {
  registerOTel({ serviceName: "my-agent" });
}
```

```ts
import { generateText } from "ai";
import { anthropic } from "@ai-sdk/anthropic";

const result = await generateText({
  model: anthropic("claude-sonnet-5"),
  prompt: "Where is my order A-1042?",
  experimental_telemetry: {
    isEnabled: true,
    // include inputs/outputs so the inspector shows conversations
    recordInputs: true,
    recordOutputs: true,
  },
});
```

Provider calls (`*.doGenerate`) become LLM steps with tokens and cost;
`ai.toolCall` spans become tool steps with arguments and results.
