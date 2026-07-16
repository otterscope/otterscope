import { useState } from "react";

export type FilterState = {
  project: string;
  status: string;
  model: string;
  service: string;
  range: string; // "", "1h", "24h", "7d"
};

export function filtersFromURL(): FilterState {
  const q = new URLSearchParams(window.location.search);
  return {
    project: q.get("project") ?? "",
    status: q.get("status") ?? "",
    model: q.get("model") ?? "",
    service: q.get("service") ?? "",
    range: q.get("range") ?? "",
  };
}

export function filtersToQuery(f: FilterState): string {
  const q = new URLSearchParams();
  q.set("limit", "100");
  if (f.project) q.set("project", f.project);
  if (f.status) q.set("status", f.status);
  if (f.model) q.set("model", f.model);
  if (f.service) q.set("service", f.service);
  if (f.range) {
    const hours = { "1h": 1, "24h": 24, "7d": 168 }[f.range];
    if (hours) {
      q.set("since", new Date(Date.now() - hours * 3600_000).toISOString());
    }
  }
  return q.toString();
}

function syncURL(f: FilterState) {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(f)) if (v) q.set(k, v);
  const qs = q.toString();
  window.history.replaceState(null, "", qs ? `/?${qs}` : "/");
}

export default function Filters({
  onChange,
}: {
  onChange: (f: FilterState) => void;
}) {
  const [f, setF] = useState<FilterState>(filtersFromURL);

  const update = (patch: Partial<FilterState>) => {
    const next = { ...f, ...patch };
    setF(next);
    syncURL(next);
    onChange(next);
  };

  return (
    <div className="filters">
      <select
        value={f.status}
        onChange={(e) => update({ status: e.target.value })}
      >
        <option value="">any status</option>
        <option value="ok">ok</option>
        <option value="error">error</option>
        <option value="running">running</option>
      </select>
      <select value={f.range} onChange={(e) => update({ range: e.target.value })}>
        <option value="">all time</option>
        <option value="1h">last hour</option>
        <option value="24h">last 24h</option>
        <option value="7d">last 7 days</option>
      </select>
      <input
        placeholder="model…"
        value={f.model}
        onChange={(e) => update({ model: e.target.value })}
      />
      <input
        placeholder="project…"
        value={f.project}
        onChange={(e) => update({ project: e.target.value })}
      />
      <input
        placeholder="service…"
        value={f.service}
        onChange={(e) => update({ service: e.target.value })}
      />
    </div>
  );
}
