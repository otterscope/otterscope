import { type FilterState } from "./filters";

// Controlled: the parent owns the filter state so saved views can drive it.
export default function Filters({
  value: f,
  onChange,
}: {
  value: FilterState;
  onChange: (f: FilterState) => void;
}) {
  const update = (patch: Partial<FilterState>) => onChange({ ...f, ...patch });

  return (
    <div className="filters">
      <input
        className="search"
        placeholder="search messages & tool i/o…"
        value={f.q}
        onChange={(e) => update({ q: e.target.value })}
      />
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
      <input
        placeholder="prompt…"
        value={f.prompt}
        onChange={(e) => update({ prompt: e.target.value })}
      />
    </div>
  );
}
