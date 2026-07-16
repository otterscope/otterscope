import { useEffect, useState } from "react";

// useLiveTick opens an SSE connection to /api/stream and returns a counter
// that increments whenever the server pushes an ingest tick. Components
// refetch when it changes — instant updates instead of constant polling.
// A slow interval is kept as a safety net for when SSE is proxied/buffered
// or the connection drops.
export function useLiveTick(): number {
  const [tick, setTick] = useState(0);

  useEffect(() => {
    let es: EventSource | null = null;
    let closed = false;

    const bump = () => setTick((t) => t + 1);

    const connect = () => {
      if (closed) return;
      es = new EventSource("/api/stream");
      es.addEventListener("runs", bump);
      es.onerror = () => {
        // Browser auto-reconnects EventSource; if it hard-fails, the safety
        // interval below keeps things fresh.
        es?.close();
        es = null;
        setTimeout(connect, 5000);
      };
    };
    connect();

    // Safety refresh (slow — the SSE tick does the real work).
    const safety = setInterval(bump, 15000);

    return () => {
      closed = true;
      clearInterval(safety);
      es?.close();
    };
  }, []);

  return tick;
}
