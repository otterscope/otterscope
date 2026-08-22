import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  filtersFromURL,
  filtersToQuery,
  syncURL,
  type FilterState,
} from "./filters";

const empty: FilterState = {
  q: "",
  project: "",
  status: "",
  model: "",
  service: "",
  prompt: "",
  range: "",
};

beforeEach(() => {
  window.history.replaceState(null, "", "/");
});

afterEach(() => {
  vi.useRealTimers();
});

describe("filtersToQuery", () => {
  it("always caps the page size", () => {
    expect(new URLSearchParams(filtersToQuery(empty)).get("limit")).toBe("100");
  });

  it("omits empty fields rather than sending blanks", () => {
    const params = new URLSearchParams(filtersToQuery(empty));
    expect([...params.keys()]).toEqual(["limit"]);
  });

  it("sends every filter the user can set", () => {
    const params = new URLSearchParams(
      filtersToQuery({
        q: "term",
        project: "alpha",
        status: "error",
        model: "claude",
        service: "svc",
        prompt: "p1",
        range: "24h",
      }),
    );
    // Every field of FilterState must reach the API in some form, or a
    // control in the UI does nothing (#113).
    for (const key of ["q", "project", "status", "model", "service", "prompt", "since"]) {
      expect(params.has(key), `${key} missing from the request`).toBe(true);
    }
  });

  it("passes every set field through", () => {
    const params = new URLSearchParams(
      filtersToQuery({
        ...empty,
        project: "alpha",
        status: "error",
        model: "claude",
        service: "svc",
        prompt: "p1",
      }),
    );
    expect(params.get("project")).toBe("alpha");
    expect(params.get("status")).toBe("error");
    expect(params.get("model")).toBe("claude");
    expect(params.get("service")).toBe("svc");
    expect(params.get("prompt")).toBe("p1");
  });

  it("turns a relative range into an RFC 3339 since bound", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-08-21T12:00:00.000Z"));

    const since = (range: string) =>
      new URLSearchParams(filtersToQuery({ ...empty, range })).get("since");

    expect(since("1h")).toBe("2026-08-21T11:00:00.000Z");
    expect(since("24h")).toBe("2026-08-20T12:00:00.000Z");
    expect(since("7d")).toBe("2026-08-14T12:00:00.000Z");
  });

  // The server now rejects a malformed bound instead of ignoring it, so an
  // unrecognised range must send no bound at all rather than something the
  // API will 400 on.
  it("sends no bound for an unrecognised range", () => {
    const params = new URLSearchParams(
      filtersToQuery({ ...empty, range: "not-a-range" }),
    );
    expect(params.has("since")).toBe(false);
  });

  // The search box was bound to state that never reached the API, so
  // searching silently returned everything (#113).
  it("sends the free-text search term", () => {
    const params = new URLSearchParams(filtersToQuery({ ...empty, q: "invoice" }));
    expect(params.get("q")).toBe("invoice");
  });

  it("leaves the term out when the box is empty", () => {
    expect(new URLSearchParams(filtersToQuery(empty)).has("q")).toBe(false);
  });

  it("does not mangle a term with spaces or punctuation", () => {
    const params = new URLSearchParams(
      filtersToQuery({ ...empty, q: 'error: "timed out" & retry' }),
    );
    expect(params.get("q")).toBe('error: "timed out" & retry');
  });
});

describe("filtersFromURL", () => {
  it("returns empty strings when the URL carries nothing", () => {
    expect(filtersFromURL()).toEqual(empty);
  });

  it("reads each field from the query string", () => {
    window.history.replaceState(
      null,
      "",
      "/?q=hi&project=alpha&status=error&model=claude&service=svc&prompt=p1&range=24h",
    );
    expect(filtersFromURL()).toEqual({
      q: "hi",
      project: "alpha",
      status: "error",
      model: "claude",
      service: "svc",
      prompt: "p1",
      range: "24h",
    });
  });

  it("ignores query params it does not own", () => {
    window.history.replaceState(null, "", "/?project=alpha&unrelated=x");
    expect(filtersFromURL().project).toBe("alpha");
  });
});

describe("syncURL and filtersFromURL round-trip", () => {
  it("restores exactly what was written, including special characters", () => {
    const f: FilterState = {
      q: "error & timeout",
      project: "alpha/beta",
      status: "error",
      model: "claude-sonnet-5",
      service: "svc name",
      prompt: "p 1",
      range: "7d",
    };
    syncURL(f);
    expect(filtersFromURL()).toEqual(f);
  });

  it("leaves a bare path when nothing is set, so a shared link stays clean", () => {
    syncURL(empty);
    expect(window.location.pathname).toBe("/");
    expect(window.location.search).toBe("");
  });

  it("clears a previously written filter", () => {
    syncURL({ ...empty, project: "alpha" });
    expect(filtersFromURL().project).toBe("alpha");
    syncURL(empty);
    expect(filtersFromURL().project).toBe("");
  });
});
