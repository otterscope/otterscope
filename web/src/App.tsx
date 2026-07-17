import { useEffect, useState } from "react";
import RunsList from "./RunsList";
import RunDetail from "./RunDetail";
import Compare from "./Compare";
import Alerts from "./Alerts";
import SharedRun from "./SharedRun";
import Audit from "./Audit";

type Health = { status: string; version: string };

// Two views don't justify a router dependency: path state + popstate.
function usePath(): [string, (p: string) => void] {
  const [path, setPath] = useState(window.location.pathname);
  useEffect(() => {
    const onPop = () => setPath(window.location.pathname);
    window.addEventListener("popstate", onPop);
    return () => window.removeEventListener("popstate", onPop);
  }, []);
  const navigate = (p: string) => {
    window.history.pushState(null, "", p);
    setPath(p);
  };
  return [path, navigate];
}

export default function App() {
  const [health, setHealth] = useState<Health | null>(null);
  const [path, navigate] = usePath();

  useEffect(() => {
    fetch("/healthz")
      .then((r) => r.json())
      .then(setHealth)
      .catch(() => {});
  }, []);

  const runMatch = path.match(/^\/runs\/([0-9a-f]+)$/);

  // Public shared-run view renders standalone (no nav, no other data).
  const shareMatch = path.match(/^\/s\/([0-9a-f]+)$/);
  if (shareMatch) {
    return <SharedRun token={shareMatch[1]} />;
  }

  return (
    <div className="shell">
      <header>
        <h1
          className="clickable"
          onClick={() => navigate("/")}
        >
          🦦 Otterscope
        </h1>
        <span className="tagline">{health ? `v${health.version}` : "…"}</span>
        <nav>
          <a
            href="/compare"
            onClick={(e) => {
              e.preventDefault();
              navigate("/compare");
            }}
          >
            compare
          </a>
          <a
            href="/alerts"
            onClick={(e) => {
              e.preventDefault();
              navigate("/alerts");
            }}
          >
            alerts
          </a>
          <a
            href="/audit"
            onClick={(e) => {
              e.preventDefault();
              navigate("/audit");
            }}
          >
            audit
          </a>
        </nav>
      </header>
      <main>
        {runMatch ? (
          <RunDetail id={runMatch[1]} navigate={navigate} />
        ) : path === "/compare" ? (
          <Compare />
        ) : path === "/alerts" ? (
          <Alerts />
        ) : path === "/audit" ? (
          <Audit />
        ) : (
          <RunsList navigate={navigate} />
        )}
      </main>
    </div>
  );
}
