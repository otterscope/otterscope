import { useEffect, useState } from "react";

type Health = { status: string; version: string };

export default function App() {
  const [health, setHealth] = useState<Health | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    fetch("/healthz")
      .then((r) => r.json())
      .then(setHealth)
      .catch(() => setError(true));
  }, []);

  return (
    <div className="shell">
      <header>
        <h1>🦦 Otterscope</h1>
        <span className="tagline">
          Lightweight, self-hosted observability for AI agents
        </span>
      </header>
      <main>
        <p className="status">
          {health && (
            <>
              server <strong>{health.status}</strong> · version{" "}
              <strong>{health.version}</strong>
            </>
          )}
          {error && <>server unreachable</>}
          {!health && !error && <>connecting…</>}
        </p>
        <p className="hint">The run explorer arrives with milestone M2.</p>
      </main>
    </div>
  );
}
