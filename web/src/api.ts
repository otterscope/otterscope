export type Run = {
  id: string;
  service: string;
  agentName: string;
  status: "running" | "ok" | "error";
  start: string;
  durationMs: number;
  inputTokens: number;
  outputTokens: number;
  llmCalls: number;
  toolCalls: number;
  models: string;
  costUsd?: number;
  costPartial?: boolean;
  error: string;
};

export type Message = { role: string; content: string };

export type Step = {
  id: string;
  parentId: string;
  kind: "agent" | "llm" | "tool" | "generic";
  name: string;
  status: "running" | "ok" | "error";
  start: string;
  offsetMs: number;
  durationMs: number;
  error?: string;
  llm?: {
    provider: string;
    requestModel: string;
    responseModel: string;
    inputTokens: number;
    outputTokens: number;
    cacheReadTokens: number;
    reasoningTokens: number;
    costUsd?: number;
    inputMessages?: Message[];
    outputMessages?: Message[];
  };
  tool?: {
    name: string;
    callId: string;
    arguments?: string;
    result?: string;
  };
};

export type RunDetail = { run: Run; steps: Step[] };

export function fmtDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`;
  return `${Math.floor(ms / 60_000)}m ${Math.round((ms % 60_000) / 1000)}s`;
}

export function fmtTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`;
  return String(n);
}

export function fmtStart(iso: string): string {
  const d = new Date(iso);
  const sameDay = d.toDateString() === new Date().toDateString();
  return sameDay
    ? d.toLocaleTimeString()
    : `${d.toLocaleDateString()} ${d.toLocaleTimeString()}`;
}

export function fmtCost(usd?: number, partial?: boolean): string {
  if (usd === undefined || usd === null) return "\u2014";
  const p = partial ? "\u2265 " : "";
  if (usd >= 1) return `${p}$${usd.toFixed(2)}`;
  if (usd >= 0.01) return `${p}$${usd.toFixed(3)}`;
  return `${p}$${usd.toFixed(5)}`;
}
