import { useEffect, useMemo, useState } from "react";
import {
  fmtCost,
  fmtDuration,
  fmtStart,
  fmtTokens,
  type RunDetail as Detail,
  type Step,
} from "./api";

const POLL_MS = 2000;

// depth by walking parent links; unknown parents count as roots so partial
// traces still render.
function depthOf(step: Step, byId: Map<string, Step>): number {
  let d = 0;
  let cur = step;
  while (cur.parentId && byId.has(cur.parentId) && d < 32) {
    cur = byId.get(cur.parentId)!;
    d++;
  }
  return d;
}

export default function RunDetail({
  id,
  navigate,
}: {
  id: string;
  navigate: (path: string) => void;
}) {
  const [detail, setDetail] = useState<Detail | null>(null);
  const [missing, setMissing] = useState(false);
  const [selected, setSelected] = useState<string | null>(null);

  useEffect(() => {
    let stop = false;
    const load = () =>
      fetch(`/api/runs/${id}`)
        .then((r) => {
          if (r.status === 404) {
            setMissing(true);
            return null;
          }
          return r.json();
        })
        .then((data) => data && !stop && setDetail(data))
        .catch(() => {});
    load();
    const timer = setInterval(load, POLL_MS);
    return () => {
      stop = true;
      clearInterval(timer);
    };
  }, [id]);

  const byId = useMemo(
    () => new Map((detail?.steps ?? []).map((s) => [s.id, s])),
    [detail],
  );

  if (missing) return <p className="hint">Run not found.</p>;
  if (!detail) return <p className="hint">loading…</p>;

  const { run, steps } = detail;
  const total = Math.max(run.durationMs, 1);
  const sel = selected ? (byId.get(selected) ?? null) : null;

  return (
    <div>
      <p>
        <a
          href="/"
          onClick={(e) => {
            e.preventDefault();
            navigate("/");
          }}
        >
          ← runs
        </a>
      </p>
      <div className="run-head">
        <span className={`badge ${run.status}`}>{run.status}</span>
        <strong>{run.service || run.id.slice(0, 12)}</strong>
        {run.agentName && <span className="agent">/ {run.agentName}</span>}
        <span className="stat">{fmtStart(run.start)}</span>
        <span className="stat">{fmtDuration(run.durationMs)}</span>
        <span className="stat">
          {fmtTokens(run.inputTokens)} in / {fmtTokens(run.outputTokens)} out
        </span>
        <span className="stat">
          {run.llmCalls} llm · {run.toolCalls} tools
        </span>
        {run.costUsd !== undefined && (
          <span className="stat">{fmtCost(run.costUsd, run.costPartial)}</span>
        )}
      </div>
      {run.error && <p className="run-error">{run.error}</p>}

      <div className="timeline">
        {steps.map((s) => (
          <div
            key={s.id}
            className={`step ${selected === s.id ? "selected" : ""}`}
            onClick={() => setSelected(selected === s.id ? null : s.id)}
          >
            <span
              className="step-name"
              style={{ paddingLeft: `${depthOf(s, byId) * 1.2}rem` }}
            >
              <span className={`kind ${s.kind}`}>{s.kind}</span>
              {s.name}
              {s.status === "error" && <span className="badge error">!</span>}
            </span>
            <span className="step-duration">{fmtDuration(s.durationMs)}</span>
            <span className="bar-track">
              <span
                className={`bar ${s.kind}`}
                style={{
                  left: `${(s.offsetMs / total) * 100}%`,
                  width: `${Math.max((s.durationMs / total) * 100, 0.5)}%`,
                }}
              />
            </span>
          </div>
        ))}
      </div>

      {sel && (
        <div className="inspector">
          <h2>
            <span className={`kind ${sel.kind}`}>{sel.kind}</span> {sel.name}
          </h2>
          {sel.error && <p className="run-error">{sel.error}</p>}
          {sel.llm && (
            <>
              <p className="hint">
                {sel.llm.provider && `${sel.llm.provider} · `}
                {sel.llm.requestModel} · {fmtTokens(sel.llm.inputTokens)} in
                {sel.llm.cacheReadTokens > 0 &&
                  ` (${fmtTokens(sel.llm.cacheReadTokens)} cached)`}{" "}
                / {fmtTokens(sel.llm.outputTokens)} out
                {sel.llm.reasoningTokens > 0 &&
                  ` (${fmtTokens(sel.llm.reasoningTokens)} reasoning)`}
                {sel.llm.costUsd !== undefined &&
                  ` · ${fmtCost(sel.llm.costUsd)}`}
              </p>
              {(sel.llm.inputMessages ?? []).map((m, i) => (
                <div key={`in-${i}`} className="msg">
                  <span className="role">{m.role}</span>
                  <pre>{m.content}</pre>
                </div>
              ))}
              {(sel.llm.outputMessages ?? []).map((m, i) => (
                <div key={`out-${i}`} className="msg out">
                  <span className="role">{m.role} (output)</span>
                  <pre>{m.content}</pre>
                </div>
              ))}
              {!sel.llm.inputMessages && !sel.llm.outputMessages && (
                <p className="hint">
                  No message content recorded — enable content capture in your
                  instrumentation to see prompts here.
                </p>
              )}
            </>
          )}
          {sel.tool && (
            <>
              <p className="hint">
                {sel.tool.name}
                {sel.tool.callId && ` · ${sel.tool.callId}`}
              </p>
              {sel.tool.arguments && (
                <div className="msg">
                  <span className="role">arguments</span>
                  <pre>{sel.tool.arguments}</pre>
                </div>
              )}
              {sel.tool.result && (
                <div className="msg out">
                  <span className="role">result</span>
                  <pre>{sel.tool.result}</pre>
                </div>
              )}
            </>
          )}
        </div>
      )}
    </div>
  );
}
