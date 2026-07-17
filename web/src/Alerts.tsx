import { useEffect, useState } from "react";

type Alert = {
  id: number;
  project: string;
  name: string;
  type: string;
  threshold: number;
  windowSecs: number;
  config: string;
  webhookUrl: string;
  enabled: boolean;
  firing: boolean;
};

const TYPES = [
  { v: "error_rate", label: "error rate >", hint: "fraction 0–1 (e.g. 0.1 = 10%)" },
  { v: "cost", label: "window cost > $", hint: "USD total in window" },
  { v: "p95_latency", label: "p95 latency > ", hint: "milliseconds" },
  { v: "assertion_fail_rate", label: "assertion fail rate >", hint: "fraction 0–1; set assertion name" },
];

const WINDOWS = [
  { v: 900, label: "15 min" },
  { v: 3600, label: "1 hour" },
  { v: 86400, label: "24 hours" },
];

export default function Alerts() {
  const [alerts, setAlerts] = useState<Alert[] | null>(null);
  const [form, setForm] = useState({
    name: "",
    type: "error_rate",
    threshold: "0.1",
    windowSecs: 3600,
    config: "",
    webhookUrl: "",
  });
  const [error, setError] = useState("");

  const load = () =>
    fetch("/api/alerts")
      .then((r) => r.json())
      .then((d) => setAlerts(d.alerts))
      .catch(() => {});

  useEffect(() => {
    load();
    const t = setInterval(load, 5000);
    return () => clearInterval(t);
  }, []);

  const create = async () => {
    setError("");
    const res = await fetch("/api/alerts", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ ...form, threshold: Number(form.threshold) }),
    });
    if (res.ok) {
      setForm({ ...form, name: "", webhookUrl: "" });
      load();
    } else {
      setError((await res.json()).error ?? "failed");
    }
  };

  const remove = async (id: number) => {
    await fetch(`/api/alerts/${id}`, { method: "DELETE" });
    load();
  };

  const typeInfo = TYPES.find((t) => t.v === form.type)!;

  return (
    <div>
      <h2 className="compare-title">Alerts</h2>
      <p className="hint">
        Otterscope evaluates each rule over its window and notifies your
        webhook when it starts (and stops) firing. Slack and Discord webhook
        URLs are auto-formatted; anything else gets a JSON payload.
      </p>

      <div className="alert-form">
        <input
          placeholder="name"
          value={form.name}
          onChange={(e) => setForm({ ...form, name: e.target.value })}
        />
        <select
          value={form.type}
          onChange={(e) => setForm({ ...form, type: e.target.value })}
        >
          {TYPES.map((t) => (
            <option key={t.v} value={t.v}>
              {t.label}
            </option>
          ))}
        </select>
        <input
          placeholder="threshold"
          title={typeInfo.hint}
          value={form.threshold}
          onChange={(e) => setForm({ ...form, threshold: e.target.value })}
          style={{ width: "6rem" }}
        />
        {form.type === "assertion_fail_rate" && (
          <input
            placeholder="assertion name"
            value={form.config}
            onChange={(e) => setForm({ ...form, config: e.target.value })}
          />
        )}
        <select
          value={form.windowSecs}
          onChange={(e) => setForm({ ...form, windowSecs: Number(e.target.value) })}
        >
          {WINDOWS.map((w) => (
            <option key={w.v} value={w.v}>
              over {w.label}
            </option>
          ))}
        </select>
        <input
          placeholder="webhook URL (Slack/Discord/…)"
          value={form.webhookUrl}
          onChange={(e) => setForm({ ...form, webhookUrl: e.target.value })}
          style={{ flex: 1, minWidth: "16rem" }}
        />
        <button onClick={create} disabled={!form.name || !form.webhookUrl}>
          add alert
        </button>
      </div>
      {error && <p className="run-error">{error}</p>}

      {alerts && alerts.length > 0 && (
        <table className="runs">
          <thead>
            <tr>
              <th>state</th>
              <th>name</th>
              <th>condition</th>
              <th>window</th>
              <th>webhook</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {alerts.map((a) => (
              <tr key={a.id}>
                <td>
                  <span className={`badge ${a.firing ? "error" : "ok"}`}>
                    {a.firing ? "firing" : "ok"}
                  </span>
                </td>
                <td>{a.name}</td>
                <td>
                  {a.type}
                  {a.config && ` (${a.config})`} &gt; {a.threshold}
                </td>
                <td>{Math.round(a.windowSecs / 60)}m</td>
                <td className="truncate" title={a.webhookUrl}>
                  {a.webhookUrl.replace(/^https?:\/\//, "").slice(0, 28)}…
                </td>
                <td>
                  <button className="link-btn" onClick={() => remove(a.id)}>
                    delete
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {alerts && alerts.length === 0 && (
        <p className="hint">No alerts yet.</p>
      )}
    </div>
  );
}
