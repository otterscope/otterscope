import { useEffect, useState } from "react";
import { fmtCost, fmtDuration, fmtTokens, apiFetch } from "./api";

type AssertionRate = {
  assertionId: number;
  name: string;
  passed: number;
  total: number;
};

type Stats = {
  runs: number;
  errors: number;
  avgDurationMs: number;
  p50DurationMs: number;
  p95DurationMs: number;
  totalCostUsd?: number;
  avgCostUsd?: number;
  inputTokens: number;
  outputTokens: number;
  assertionRates: AssertionRate[];
};

type Side = {
  status: string;
  model: string;
  service: string;
  prompt: string;
  range: string;
};

function sideFromURL(prefix: string): Side {
  const q = new URLSearchParams(window.location.search);
  return {
    status: q.get(`${prefix}_status`) ?? "",
    model: q.get(`${prefix}_model`) ?? "",
    service: q.get(`${prefix}_service`) ?? "",
    prompt: q.get(`${prefix}_prompt`) ?? "",
    range: q.get(`${prefix}_range`) ?? "",
  };
}

function sideQuery(s: Side): string {
  const q = new URLSearchParams();
  if (s.status) q.set("status", s.status);
  if (s.model) q.set("model", s.model);
  if (s.service) q.set("service", s.service);
  if (s.prompt) q.set("prompt", s.prompt);
  if (s.range) {
    const hours = { "1h": 1, "24h": 24, "7d": 168 }[s.range];
    if (hours)
      q.set("since", new Date(Date.now() - hours * 3600_000).toISOString());
  }
  return q.toString();
}

function syncURL(a: Side, b: Side) {
  const q = new URLSearchParams();
  for (const [prefix, side] of [
    ["a", a],
    ["b", b],
  ] as const) {
    for (const [k, v] of Object.entries(side)) if (v) q.set(`${prefix}_${k}`, v);
  }
  const qs = q.toString();
  window.history.replaceState(null, "", qs ? `/compare?${qs}` : "/compare");
}

function SideForm({
  side,
  onChange,
}: {
  side: Side;
  onChange: (s: Side) => void;
}) {
  const set = (patch: Partial<Side>) => onChange({ ...side, ...patch });
  return (
    <div className="filters">
      <select value={side.status} onChange={(e) => set({ status: e.target.value })}>
        <option value="">any status</option>
        <option value="ok">ok</option>
        <option value="error">error</option>
      </select>
      <select value={side.range} onChange={(e) => set({ range: e.target.value })}>
        <option value="">all time</option>
        <option value="1h">last hour</option>
        <option value="24h">last 24h</option>
        <option value="7d">last 7 days</option>
      </select>
      <input
        placeholder="model…"
        value={side.model}
        onChange={(e) => set({ model: e.target.value })}
      />
      <input
        placeholder="service…"
        value={side.service}
        onChange={(e) => set({ service: e.target.value })}
      />
      <input
        placeholder="prompt version…"
        value={side.prompt}
        onChange={(e) => set({ prompt: e.target.value })}
      />
    </div>
  );
}

function pct(n: number, d: number): string {
  return d === 0 ? "—" : `${((100 * n) / d).toFixed(1)}%`;
}

export default function Compare() {
  const [a, setA] = useState<Side>(() => sideFromURL("a"));
  const [b, setB] = useState<Side>(() => sideFromURL("b"));
  const [statsA, setStatsA] = useState<Stats | null>(null);
  const [statsB, setStatsB] = useState<Stats | null>(null);

  useEffect(() => {
    syncURL(a, b);
    apiFetch(`/api/stats?${sideQuery(a)}`)
      .then((r) => r.json())
      .then(setStatsA)
      .catch(() => {});
    apiFetch(`/api/stats?${sideQuery(b)}`)
      .then((r) => r.json())
      .then(setStatsB)
      .catch(() => {});
  }, [a, b]);

  const rows: [string, (s: Stats) => string][] = [
    ["runs", (s) => String(s.runs)],
    ["error rate", (s) => pct(s.errors, s.runs)],
    ["p50 duration", (s) => fmtDuration(s.p50DurationMs)],
    ["p95 duration", (s) => fmtDuration(s.p95DurationMs)],
    ["avg cost / run", (s) => fmtCost(s.avgCostUsd)],
    ["total cost", (s) => fmtCost(s.totalCostUsd)],
    ["tokens in / out", (s) => `${fmtTokens(s.inputTokens)} / ${fmtTokens(s.outputTokens)}`],
  ];

  const assertNames = new Map<number, string>();
  for (const s of [statsA, statsB]) {
    for (const r of s?.assertionRates ?? []) assertNames.set(r.assertionId, r.name);
  }
  const rateFor = (s: Stats | null, id: number) => {
    const r = s?.assertionRates.find((x) => x.assertionId === id);
    return r ? pct(r.passed, r.total) : "—";
  };

  return (
    <div>
      <h2 className="compare-title">Compare</h2>
      <div className="compare-grid">
        <div>
          <h3>Side A</h3>
          <SideForm side={a} onChange={setA} />
        </div>
        <div>
          <h3>Side B</h3>
          <SideForm side={b} onChange={setB} />
        </div>
      </div>
      <table className="runs compare-table">
        <thead>
          <tr>
            <th>metric</th>
            <th>A</th>
            <th>B</th>
          </tr>
        </thead>
        <tbody>
          {rows.map(([label, get]) => (
            <tr key={label}>
              <td>{label}</td>
              <td>{statsA ? get(statsA) : "…"}</td>
              <td>{statsB ? get(statsB) : "…"}</td>
            </tr>
          ))}
          {[...assertNames.entries()].map(([id, name]) => (
            <tr key={`assert-${id}`}>
              <td>✓ {name}</td>
              <td>{rateFor(statsA, id)}</td>
              <td>{rateFor(statsB, id)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
