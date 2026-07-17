import { useEffect, useState } from "react";
import type { FilterState } from "./Filters";

type View = { id: number; name: string; params: FilterState };

// SavedViews renders named filter presets: click to apply, save the current
// filters under a name, or delete one.
export default function SavedViews({
  current,
  onApply,
}: {
  current: FilterState;
  onApply: (f: FilterState) => void;
}) {
  const [views, setViews] = useState<View[]>([]);

  const load = () =>
    fetch("/api/views")
      .then((r) => r.json())
      .then((d) => setViews(d.views))
      .catch(() => {});

  useEffect(() => {
    load();
  }, []);

  const save = async () => {
    const name = window.prompt("Name this view (e.g. 'prod errors 24h')");
    if (!name) return;
    const res = await fetch("/api/views", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name, params: current }),
    });
    if (res.ok) load();
    else alert((await res.json()).error ?? "could not save view");
  };

  const remove = async (id: number) => {
    await fetch(`/api/views/${id}`, { method: "DELETE" });
    load();
  };

  return (
    <div className="views">
      {views.map((v) => (
        <span key={v.id} className="view-chip">
          <button className="view-apply" onClick={() => onApply(v.params)}>
            {v.name}
          </button>
          <button
            className="view-del"
            title="delete view"
            onClick={() => remove(v.id)}
          >
            ×
          </button>
        </span>
      ))}
      <button className="view-save" onClick={save}>
        + save view
      </button>
    </div>
  );
}
