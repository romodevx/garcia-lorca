import { afterEach, describe, expect, it, vi } from "vitest";
import { createNonWorkingPeriod, deleteNonWorkingPeriod, listNonWorkingPeriods } from "./api";

const BASE = "http://gw";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

function lastCall(mock: ReturnType<typeof vi.fn>): [string, RequestInit] {
  const call = mock.mock.calls[0] as [string, RequestInit];
  return [call[0], call[1]];
}

describe("school calendar", () => {
  it("sends a single-day holiday with one date", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({ id: "per-1", cycleId: "c1", label: "Independencia", kind: "holiday", startsOn: "2026-09-16", endsOn: "2026-09-16" }, 201),
    );
    vi.stubGlobal("fetch", fetchMock);

    const period = await createNonWorkingPeriod(BASE, {
      cycleId: "c1", label: "Independencia", kind: "holiday", startsOn: "2026-09-16",
    });

    expect(period.id).toBe("per-1");
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/config/calendar");
    const body = JSON.parse(init.body as string) as Record<string, unknown>;
    expect(body.startsOn).toBe("2026-09-16");
    expect("endsOn" in body).toBe(false);
  });

  it("lists a cycle's closed days", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ periods: [{ id: "per-1", cycleId: "c1", label: "Independencia", kind: "holiday", startsOn: "2026-09-16", endsOn: "2026-09-16" }] }));
    vi.stubGlobal("fetch", fetchMock);

    const periods = await listNonWorkingPeriods(BASE, "c1");

    expect(periods).toHaveLength(1);
    expect(lastCall(fetchMock)[0]).toBe("http://gw/api/v1/config/calendar?cycleId=c1");
  });

  it("removes a day entered by mistake", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await deleteNonWorkingPeriod(BASE, "per-1");

    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/config/calendar/per-1");
    expect(init.method).toBe("DELETE");
  });
});
