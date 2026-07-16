import { useEffect, useState } from "react";
import { fmtDuration, fmtStart, fmtTokens, type Run } from "./api";

const POLL_MS = 2000;

export default function RunsList({
  navigate,
}: {
  navigate: (path: string) => void;
}) {
  const [runs, setRuns] = useState<Run[] | null>(null);

  useEffect(() => {
    let stop = false;
    const load = () =>
      fetch("/api/runs?limit=100")
        .then((r) => r.json())
        .then((data) => !stop && setRuns(data.runs))
        .catch(() => {});
    load();
    const timer = setInterval(load, POLL_MS);
    return () => {
      stop = true;
      clearInterval(timer);
    };
  }, []);

  if (runs === null) return <p className="hint">loading…</p>;
  if (runs.length === 0)
    return (
      <div className="empty">
        <p>No runs yet.</p>
        <p className="hint">
          Point your agent&apos;s OpenTelemetry exporter here:
          <br />
          <code>export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318</code>
        </p>
      </div>
    );

  return (
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
          <tr
            key={r.id}
            className="clickable"
            title={r.error || r.id}
            onClick={() => navigate(`/runs/${r.id}`)}
          >
            <td>
              <span className={`badge ${r.status}`}>{r.status}</span>
            </td>
            <td>{fmtStart(r.start)}</td>
            <td>{fmtDuration(r.durationMs)}</td>
            <td>
              {r.service || "—"}
              {r.agentName && <span className="agent"> / {r.agentName}</span>}
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
  );
}
