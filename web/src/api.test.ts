import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { apiFetch, fmtCost, fmtDuration, fmtStart, fmtTokens, readToken } from "./api";

describe("fmtDuration", () => {
  it("uses milliseconds below a second", () => {
    expect(fmtDuration(0)).toBe("0ms");
    expect(fmtDuration(999)).toBe("999ms");
  });

  it("uses seconds with one decimal below a minute", () => {
    expect(fmtDuration(1000)).toBe("1.0s");
    expect(fmtDuration(59_940)).toBe("59.9s");
  });

  it("uses minutes and seconds from a minute up", () => {
    expect(fmtDuration(60_000)).toBe("1m 0s");
    expect(fmtDuration(3_723_000)).toBe("62m 3s");
  });
});

describe("fmtTokens", () => {
  it("prints small counts verbatim", () => {
    expect(fmtTokens(0)).toBe("0");
    expect(fmtTokens(999)).toBe("999");
  });

  it("abbreviates thousands and millions", () => {
    expect(fmtTokens(1000)).toBe("1.0k");
    expect(fmtTokens(15_400)).toBe("15.4k");
    expect(fmtTokens(1_000_000)).toBe("1.0M");
    expect(fmtTokens(2_450_000)).toBe("2.5M");
  });
});

describe("fmtCost", () => {
  it("renders an em dash when the cost is unknown", () => {
    expect(fmtCost(undefined)).toBe("—");
    expect(fmtCost(null as unknown as number)).toBe("—");
  });

  it("scales precision to the magnitude", () => {
    expect(fmtCost(1.5)).toBe("$1.50");
    expect(fmtCost(0.25)).toBe("$0.250");
    expect(fmtCost(0.000012)).toBe("$0.00001");
  });

  it("marks a partial cost as a lower bound", () => {
    expect(fmtCost(1.5, true)).toBe("≥ $1.50");
    expect(fmtCost(0.25, true)).toBe("≥ $0.250");
  });
});

describe("fmtStart", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 0, 15, 12, 0, 0));
  });
  afterEach(() => vi.useRealTimers());

  it("shows only the time for a timestamp from today", () => {
    const today = new Date(2026, 0, 15, 9, 30, 0);
    expect(fmtStart(today.toISOString())).toBe(today.toLocaleTimeString());
  });

  it("prefixes the date for a timestamp from another day", () => {
    const other = new Date(2026, 0, 14, 9, 30, 0);
    expect(fmtStart(other.toISOString())).toBe(
      `${other.toLocaleDateString()} ${other.toLocaleTimeString()}`,
    );
  });
});

describe("apiFetch", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.restoreAllMocks();
  });

  it("sends no Authorization header when no token is stored", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response("{}", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    await apiFetch("/api/runs");

    const headers = fetchMock.mock.calls[0][1].headers as Headers;
    expect(headers.has("Authorization")).toBe(false);
  });

  it("attaches the stored token as a bearer credential", async () => {
    localStorage.setItem("otterscope_token", "secret");
    const fetchMock = vi.fn().mockResolvedValue(new Response("{}", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    await apiFetch("/api/runs");

    const headers = fetchMock.mock.calls[0][1].headers as Headers;
    expect(headers.get("Authorization")).toBe("Bearer secret");
  });

  it("prompts once on 401, stores the trimmed token and retries", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(new Response("", { status: 401 }))
      .mockResolvedValueOnce(new Response("{}", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    const prompt = vi.fn().mockReturnValue("  entered  ");
    vi.stubGlobal("prompt", prompt);

    const res = await apiFetch("/api/runs");

    expect(res.status).toBe(200);
    expect(prompt).toHaveBeenCalledOnce();
    expect(readToken()).toBe("entered");
    const retryHeaders = fetchMock.mock.calls[1][1].headers as Headers;
    expect(retryHeaders.get("Authorization")).toBe("Bearer entered");
  });

  it("gives up after one retry instead of prompting in a loop", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response("", { status: 401 }));
    vi.stubGlobal("fetch", fetchMock);
    const prompt = vi.fn().mockReturnValue("bad");
    vi.stubGlobal("prompt", prompt);

    const res = await apiFetch("/api/runs");

    expect(res.status).toBe(401);
    expect(prompt).toHaveBeenCalledOnce();
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("returns the 401 untouched when the prompt is dismissed", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response("", { status: 401 }));
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal("prompt", vi.fn().mockReturnValue(null));

    const res = await apiFetch("/api/runs");

    expect(res.status).toBe(401);
    expect(fetchMock).toHaveBeenCalledOnce();
    expect(readToken()).toBe("");
  });
});
