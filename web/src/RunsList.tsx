import { useEffect, useState } from "react";
import { fmtCost, fmtDuration, fmtStart, fmtTokens, type Run } from "./api";
import Filters, { filtersFromURL, filtersToQuery, type FilterState } from "./Filters";
import { useLiveTick } from "./live";

export default function RunsList({
  navigate,
}: {
  navigate: (path: string) => void;
}) {
  const [runs, setRuns] = useState<Run[] | null>(null);
  const [filters, setFilters] = useState<FilterState>(filtersFromURL);
  const tick = useLiveTick();

  useEffect(() => {
    let stop = false;
    fetch(`/api/runs?${filtersToQuery(filters)}`)
      .then((r) => r.json())
      .then((data) => !stop && setRuns(data.runs))
      .catch(() => {});
    return () => {
      stop = true;
    };
  }, [filters, tick]);

  const hasFilters =
    filters.project ||
    filters.status ||
    filters.model ||
    filters.service ||
    filters.range;

  return (
    <>
      <Filters onChange={setFilters} />
      {runs === null && <p className="hint">loading…</p>}
      {runs !== null && runs.length === 0 && (
        <div className="empty">
          <p>{hasFilters ? "No runs match these filters." : "No runs yet."}</p>
          {!hasFilters && (
            <p className="hint">
              Point your agent&apos;s OpenTelemetry exporter here:
              <br />
              <code>
                export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
              </code>
            </p>
          )}
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
          <th className="num">cost</th>
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
            <td className="num">{fmtCost(r.costUsd, r.costPartial)}</td>
            <td className="num">{fmtTokens(r.inputTokens)}</td>
            <td className="num">{fmtTokens(r.outputTokens)}</td>
            <td className="num">{r.llmCalls}</td>
            <td className="num">{r.toolCalls}</td>
          </tr>
        ))}
      </tbody>
      </table>
      )}
    </>
  );
}
