import { useEffect, useState } from "react";

type Health = { status: string; version: string };

type Run = {
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
  error: string;
};

const POLL_MS = 2000;

function fmtDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`;
  return `${Math.floor(ms / 60_000)}m ${Math.round((ms % 60_000) / 1000)}s`;
}

function fmtTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`;
  return String(n);
}

function fmtStart(iso: string): string {
  const d = new Date(iso);
  const sameDay = d.toDateString() === new Date().toDateString();
  return sameDay
    ? d.toLocaleTimeString()
    : `${d.toLocaleDateString()} ${d.toLocaleTimeString()}`;
}

export default function App() {
  const [health, setHealth] = useState<Health | null>(null);
  const [runs, setRuns] = useState<Run[] | null>(null);
  const [offline, setOffline] = useState(false);

  useEffect(() => {
    fetch("/healthz")
      .then((r) => r.json())
      .then(setHealth)
      .catch(() => {});
  }, []);

  useEffect(() => {
    let stop = false;
    const load = () =>
      fetch("/api/runs?limit=100")
        .then((r) => r.json())
        .then((data) => {
          if (!stop) {
            setRuns(data.runs);
            setOffline(false);
          }
        })
        .catch(() => !stop && setOffline(true));
    load();
    const timer = setInterval(load, POLL_MS);
    return () => {
      stop = true;
      clearInterval(timer);
    };
  }, []);

  return (
    <div className="shell">
      <header>
        <h1>🦦 Otterscope</h1>
        <span className="tagline">
          {health ? `v${health.version}` : "…"}
          {offline && " · server unreachable"}
        </span>
      </header>
      <main>
        {runs === null && <p className="hint">loading…</p>}
        {runs !== null && runs.length === 0 && (
          <div className="empty">
            <p>No runs yet.</p>
            <p className="hint">
              Point your agent&apos;s OpenTelemetry exporter here:
              <br />
              <code>
                export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
              </code>
            </p>
          </div>
        )}
        {runs !== null && runs.length > 0 && (
          <table className="runs">
            <thead>
              <tr>
                <th>status</th>
                <th>started</th>
                <th>duration</th>
                <th>service / agent</th>
                <th>models</th>
                <th className="num">tokens in</th>
                <th className="num">out</th>
                <th className="num">llm</th>
                <th className="num">tools</th>
              </tr>
            </thead>
            <tbody>
              {runs.map((r) => (
                <tr key={r.id} title={r.error || r.id}>
                  <td>
                    <span className={`badge ${r.status}`}>{r.status}</span>
                  </td>
                  <td>{fmtStart(r.start)}</td>
                  <td>{fmtDuration(r.durationMs)}</td>
                  <td>
                    {r.service || "—"}
                    {r.agentName && (
                      <span className="agent"> / {r.agentName}</span>
                    )}
                  </td>
                  <td>{r.models || "—"}</td>
                  <td className="num">{fmtTokens(r.inputTokens)}</td>
                  <td className="num">{fmtTokens(r.outputTokens)}</td>
                  <td className="num">{r.llmCalls}</td>
                  <td className="num">{r.toolCalls}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </main>
    </div>
  );
}
