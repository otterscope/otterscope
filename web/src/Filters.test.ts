import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { filtersFromURL, filtersToQuery, syncURL, type FilterState } from "./Filters";

const empty: FilterState = {
  q: "",
  project: "",
  status: "",
  model: "",
  service: "",
  prompt: "",
  range: "",
};

function goto(search: string) {
  window.history.replaceState(null, "", `/${search}`);
}

describe("filtersFromURL", () => {
  it("defaults every field to the empty string", () => {
    goto("");
    expect(filtersFromURL()).toEqual(empty);
  });

  it("reads each supported param", () => {
    goto("?q=timeout&project=api&status=error&model=gpt-4o&service=web&prompt=sys&range=24h");
    expect(filtersFromURL()).toEqual({
      q: "timeout",
      project: "api",
      status: "error",
      model: "gpt-4o",
      service: "web",
      prompt: "sys",
      range: "24h",
    });
  });

  it("ignores params it does not know", () => {
    goto("?status=ok&bogus=1");
    expect(filtersFromURL()).toEqual({ ...empty, status: "ok" });
  });
});

describe("filtersToQuery", () => {
  it("always caps the page size", () => {
    expect(new URLSearchParams(filtersToQuery(empty)).get("limit")).toBe("100");
  });

  it("omits empty fields rather than sending blanks", () => {
    const q = new URLSearchParams(filtersToQuery(empty));
    expect([...q.keys()]).toEqual(["limit"]);
  });

  it("passes through the set fields", () => {
    const q = new URLSearchParams(
      filtersToQuery({ ...empty, project: "api", status: "error", model: "gpt-4o" }),
    );
    expect(q.get("project")).toBe("api");
    expect(q.get("status")).toBe("error");
    expect(q.get("model")).toBe("gpt-4o");
  });

  describe("range", () => {
    beforeEach(() => {
      vi.useFakeTimers();
      vi.setSystemTime(new Date("2026-01-15T12:00:00.000Z"));
    });
    afterEach(() => vi.useRealTimers());

    it("turns a relative range into an absolute RFC 3339 lower bound", () => {
      const since = (range: string) =>
        new URLSearchParams(filtersToQuery({ ...empty, range })).get("since");
      expect(since("1h")).toBe("2026-01-15T11:00:00.000Z");
      expect(since("24h")).toBe("2026-01-14T12:00:00.000Z");
      expect(since("7d")).toBe("2026-01-08T12:00:00.000Z");
    });

    it("sends no bound for all-time or an unknown range", () => {
      expect(new URLSearchParams(filtersToQuery(empty)).has("since")).toBe(false);
      expect(
        new URLSearchParams(filtersToQuery({ ...empty, range: "42y" })).has("since"),
      ).toBe(false);
    });
  });
});

describe("syncURL", () => {
  it("writes the set filters back to the address bar", () => {
    goto("");
    syncURL({ ...empty, status: "error", range: "1h" });
    expect(window.location.search).toBe("?status=error&range=1h");
  });

  it("collapses to a bare path when nothing is filtered", () => {
    goto("?status=error");
    syncURL(empty);
    expect(window.location.pathname).toBe("/");
    expect(window.location.search).toBe("");
  });

  it("round-trips through filtersFromURL", () => {
    const f: FilterState = {
      q: "timeout",
      project: "api",
      status: "error",
      model: "gpt-4o",
      service: "web",
      prompt: "sys",
      range: "7d",
    };
    goto("");
    syncURL(f);
    expect(filtersFromURL()).toEqual(f);
  });
});
