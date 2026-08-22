import { afterEach, describe, expect, it, vi } from "vitest";
import {
  apiFetch,
  csvTruncationNotice,
  fmtCost,
  fmtDuration,
  fmtStart,
  fmtTokens,
  readToken,
} from "./api";

afterEach(() => {
  vi.restoreAllMocks();
  vi.useRealTimers();
  localStorage.clear();
});

describe("fmtDuration", () => {
  it("uses ms below a second", () => {
    expect(fmtDuration(0)).toBe("0ms");
    expect(fmtDuration(999)).toBe("999ms");
  });

  it("uses seconds below a minute", () => {
    expect(fmtDuration(1000)).toBe("1.0s");
    expect(fmtDuration(59_940)).toBe("59.9s");
  });

  it("uses minutes and seconds above a minute", () => {
    expect(fmtDuration(60_000)).toBe("1m 0s");
    expect(fmtDuration(3_661_000)).toBe("61m 1s");
  });
});

describe("fmtTokens", () => {
  it("passes small counts through unchanged", () => {
    expect(fmtTokens(0)).toBe("0");
    expect(fmtTokens(999)).toBe("999");
  });

  it("abbreviates thousands and millions", () => {
    expect(fmtTokens(1000)).toBe("1.0k");
    expect(fmtTokens(12_500)).toBe("12.5k");
    expect(fmtTokens(1_000_000)).toBe("1.0M");
    expect(fmtTokens(2_450_000)).toBe("2.5M");
  });
});

describe("fmtCost", () => {
  // An unknown model has no cost. Showing "$0.00" would be a fabricated
  // number, which is exactly what the pricing table refuses to do.
  it("renders a dash when there is no cost, never a zero", () => {
    expect(fmtCost(undefined)).toBe("—");
    expect(fmtCost(null as unknown as number)).toBe("—");
  });

  it("scales precision to the magnitude", () => {
    expect(fmtCost(1.234)).toBe("$1.23");
    expect(fmtCost(0.0456)).toBe("$0.046");
    expect(fmtCost(0.000123)).toBe("$0.00012");
  });

  it("marks a partial total as a lower bound", () => {
    expect(fmtCost(1.5, true)).toBe("≥ $1.50");
    expect(fmtCost(1.5, false)).toBe("$1.50");
  });

  it("keeps the zero cost of a genuinely free call distinct from unknown", () => {
    expect(fmtCost(0)).toBe("$0.00000");
  });
});

describe("fmtStart", () => {
  it("omits the date for today and includes it otherwise", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-08-21T12:00:00Z"));

    const today = fmtStart(new Date("2026-08-21T09:30:00Z").toISOString());
    const earlier = fmtStart(new Date("2026-08-14T09:30:00Z").toISOString());

    expect(today).not.toContain("/");
    expect(earlier).toContain("/");
    // The older stamp is the same time-of-day string with a date prefixed.
    expect(earlier.endsWith(today)).toBe(true);
  });
});

describe("csvTruncationNotice", () => {
  const res = (headers: Record<string, string>) =>
    new Response("", { headers });

  it("is silent for a complete export", () => {
    expect(csvTruncationNotice(res({}))).toBeNull();
  });

  it("reports the cap when the export was truncated", () => {
    const notice = csvTruncationNotice(
      res({ "X-Otterscope-Truncated": "true", "X-Otterscope-Row-Limit": "10000" }),
    );
    expect(notice).toContain("10000");
    expect(notice).toMatch(/narrow the filters/i);
  });

  it("still warns when the limit header is missing", () => {
    const notice = csvTruncationNotice(res({ "X-Otterscope-Truncated": "true" }));
    expect(notice).not.toBeNull();
    expect(notice).not.toContain("null");
  });
});

describe("apiFetch", () => {
  it("sends no Authorization header when no token is stored", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response("{}"));
    vi.stubGlobal("fetch", fetchMock);

    await apiFetch("/api/runs");

    const headers = fetchMock.mock.calls[0][1].headers as Headers;
    expect(headers.get("Authorization")).toBeNull();
  });

  it("attaches the stored token", async () => {
    localStorage.setItem("otterscope_token", "tok-123");
    const fetchMock = vi.fn().mockResolvedValue(new Response("{}"));
    vi.stubGlobal("fetch", fetchMock);

    await apiFetch("/api/runs");

    const headers = fetchMock.mock.calls[0][1].headers as Headers;
    expect(headers.get("Authorization")).toBe("Bearer tok-123");
    expect(readToken()).toBe("tok-123");
  });

  it("prompts once on 401, stores the token, and retries", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(new Response("", { status: 401 }))
      .mockResolvedValueOnce(new Response("{}", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal("prompt", vi.fn().mockReturnValue("  entered-token  "));

    const res = await apiFetch("/api/runs");

    expect(res.status).toBe(200);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    // Trimmed before storing, or every request carries a broken header.
    expect(readToken()).toBe("entered-token");
    const retryHeaders = fetchMock.mock.calls[1][1].headers as Headers;
    expect(retryHeaders.get("Authorization")).toBe("Bearer entered-token");
  });

  it("gives up after one retry rather than prompting in a loop", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response("", { status: 401 }));
    vi.stubGlobal("fetch", fetchMock);
    const promptMock = vi.fn().mockReturnValue("nope");
    vi.stubGlobal("prompt", promptMock);

    const res = await apiFetch("/api/runs");

    expect(res.status).toBe(401);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(promptMock).toHaveBeenCalledTimes(1);
  });

  it("does not retry when the prompt is dismissed", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response("", { status: 401 }));
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal("prompt", vi.fn().mockReturnValue(null));

    const res = await apiFetch("/api/runs");

    expect(res.status).toBe(401);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
