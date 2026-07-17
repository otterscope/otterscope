import { useEffect, useState } from "react";
import { type RunDetail as Detail, apiFetch } from "./api";
import { useLiveTick } from "./live";
import RunView from "./RunView";

const POLL_MS = 2000;

export default function RunDetail({
  id,
  navigate,
}: {
  id: string;
  navigate: (path: string) => void;
}) {
  const [detail, setDetail] = useState<Detail | null>(null);
  const [missing, setMissing] = useState(false);
  const [shareUrl, setShareUrl] = useState<string | null>(null);
  const tick = useLiveTick();

  useEffect(() => {
    let stop = false;
    const load = () =>
      apiFetch(`/api/runs/${id}`)
        .then((r) => {
          if (r.status === 404) {
            setMissing(true);
            return null;
          }
          return r.json();
        })
        .then((data) => data && !stop && setDetail(data))
        .catch(() => {});
    load();
    const timer = setInterval(load, POLL_MS);
    return () => {
      stop = true;
      clearInterval(timer);
    };
  }, [id, tick]);

  const share = async () => {
    const res = await apiFetch(`/api/runs/${id}/share`, { method: "POST" });
    if (res.ok) {
      const { url } = await res.json();
      setShareUrl(window.location.origin + url);
    }
  };

  if (missing) return <p className="hint">Run not found.</p>;
  if (!detail) return <p className="hint">loading…</p>;

  return (
    <div>
      <p className="run-toolbar">
        <a
          href="/"
          onClick={(e) => {
            e.preventDefault();
            navigate("/");
          }}
        >
          ← runs
        </a>
        <button className="link-btn accent" onClick={share}>
          share
        </button>
      </p>
      {shareUrl && (
        <p className="share-link">
          Public read-only link:{" "}
          <a href={shareUrl} target="_blank" rel="noreferrer">
            {shareUrl}
          </a>
        </p>
      )}
      <RunView detail={detail} />
    </div>
  );
}
