import { useEffect, useState } from "react";
import RunsList from "./RunsList";
import RunDetail from "./RunDetail";

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
      </header>
      <main>
        {runMatch ? (
          <RunDetail id={runMatch[1]} navigate={navigate} />
        ) : (
          <RunsList navigate={navigate} />
        )}
      </main>
    </div>
  );
}
