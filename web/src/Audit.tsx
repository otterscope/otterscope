import { useEffect, useState } from "react";
import { apiFetch } from "./api";
import { fmtStart } from "./api";

type Entry = { at: string; action: string; entity: string; detail: string };

export default function Audit() {
  const [entries, setEntries] = useState<Entry[] | null>(null);

  useEffect(() => {
    apiFetch("/api/audit?limit=500")
      .then((r) => r.json())
      .then((d) => setEntries(d.entries))
      .catch(() => {});
  }, []);

  if (entries === null) return <p className="hint">loading…</p>;

  return (
    <div>
      <h2 className="compare-title">Audit log</h2>
      <p className="hint">
        Every create/delete of assertions, alerts, projects, tokens, views, and
        shares — from the UI, API, or CLI.
      </p>
      {entries.length === 0 ? (
        <p className="hint">No changes recorded yet.</p>
      ) : (
        <table className="runs">
          <thead>
            <tr>
              <th>when</th>
              <th>action</th>
              <th>entity</th>
              <th>detail</th>
            </tr>
          </thead>
          <tbody>
            {entries.map((e, i) => (
              <tr key={i}>
                <td>{fmtStart(e.at)}</td>
                <td>
                  <span className={`badge ${e.action === "delete" ? "error" : "ok"}`}>
                    {e.action}
                  </span>
                </td>
                <td>{e.entity}</td>
                <td className="truncate" title={e.detail}>
                  {e.detail}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
