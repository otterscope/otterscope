import { useEffect, useState } from "react";
import { type RunDetail as Detail } from "./api";
import RunView from "./RunView";

// SharedRun is the public, standalone view behind a share token — no nav, no
// other data, just the one run.
export default function SharedRun({ token }: { token: string }) {
  const [detail, setDetail] = useState<Detail | null>(null);
  const [missing, setMissing] = useState(false);

  useEffect(() => {
    fetch(`/api/shared/${token}`)
      .then((r) => {
        if (!r.ok) {
          setMissing(true);
          return null;
        }
        return r.json();
      })
      .then((data) => data && setDetail(data))
      .catch(() => setMissing(true));
  }, [token]);

  if (missing)
    return (
      <div className="shell">
        <p className="hint">This shared run doesn’t exist or was revoked.</p>
      </div>
    );
  if (!detail) return <p className="hint">loading…</p>;

  return (
    <div className="shell">
      <header>
        <h1>🦦 Otterscope</h1>
        <span className="tagline">shared run · read-only</span>
      </header>
      <main>
        <RunView detail={detail} />
      </main>
    </div>
  );
}
