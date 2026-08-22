// Filter state and its URL <-> query-string mapping. Kept out of the
// component file so it can be unit-tested without rendering, and so
// Filters.tsx exports only a component (react-refresh keeps working).
export type FilterState = {
  q: string;
  project: string;
  status: string;
  model: string;
  service: string;
  prompt: string;
  range: string; // "", "1h", "24h", "7d"
};

export function filtersFromURL(): FilterState {
  const q = new URLSearchParams(window.location.search);
  return {
    q: q.get("q") ?? "",
    project: q.get("project") ?? "",
    status: q.get("status") ?? "",
    model: q.get("model") ?? "",
    service: q.get("service") ?? "",
    prompt: q.get("prompt") ?? "",
    range: q.get("range") ?? "",
  };
}

export function filtersToQuery(f: FilterState): string {
  const q = new URLSearchParams();
  q.set("limit", "100");
  if (f.q) q.set("q", f.q);
  if (f.project) q.set("project", f.project);
  if (f.status) q.set("status", f.status);
  if (f.model) q.set("model", f.model);
  if (f.service) q.set("service", f.service);
  if (f.prompt) q.set("prompt", f.prompt);
  if (f.range) {
    const hours = { "1h": 1, "24h": 24, "7d": 168 }[f.range];
    if (hours) {
      q.set("since", new Date(Date.now() - hours * 3600_000).toISOString());
    }
  }
  return q.toString();
}

export function syncURL(f: FilterState) {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(f)) if (v) q.set(k, v);
  const qs = q.toString();
  window.history.replaceState(null, "", qs ? `/?${qs}` : "/");
}
