import { afterEach, describe, expect, it, vi } from "vitest";
import { getOverdue, listCharges } from "./api";

// T14: a cycle holds thousands of charges (567 pupils x 9 months = 5,103) and
// the gateway serves them 500 at a time. These helpers keep their signatures —
// every caller still gets ALL the rows — by following nextPageToken, and they
// fail loudly rather than hand back a silent first page.

const BASE = "http://gw";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function wireCharge(id: string) {
  return {
    id,
    studentId: "st1",
    cycleId: "cy1",
    conceptId: "co1",
    description: "Colegiatura",
    amountCents: 100000,
    discountCents: 0,
    dueDate: "2026-09-10",
    status: "pending",
    paidCents: 0,
    remainingCents: 100000,
    scholarshipCents: 0,
  };
}

function urlsOf(mock: ReturnType<typeof vi.fn>): string[] {
  return mock.mock.calls.map((call) => String(call[0]));
}

describe("listCharges walks every page", () => {
  it("follows nextPageToken until it is absent and returns all rows in order", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ charges: [wireCharge("a"), wireCharge("b")], nextPageToken: "t1" }))
      .mockResolvedValueOnce(jsonResponse({ charges: [wireCharge("c")], nextPageToken: "t2" }))
      .mockResolvedValueOnce(jsonResponse({ charges: [wireCharge("d")] }));
    vi.stubGlobal("fetch", fetchMock);

    const rows = await listCharges(BASE, { cycleId: "cy1" });

    expect(rows.map((c) => c.id)).toEqual(["a", "b", "c", "d"]);
    expect(urlsOf(fetchMock)).toEqual([
      "http://gw/api/v1/finance/charges?cycleId=cy1",
      "http://gw/api/v1/finance/charges?cycleId=cy1&pageToken=t1",
      "http://gw/api/v1/finance/charges?cycleId=cy1&pageToken=t2",
    ]);
  });

  it("treats an empty nextPageToken as the last page", async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(jsonResponse({ charges: [wireCharge("a")], nextPageToken: "" }));
    vi.stubGlobal("fetch", fetchMock);

    const rows = await listCharges(BASE, { studentId: "st1" });

    expect(rows.map((c) => c.id)).toEqual(["a"]);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("throws instead of truncating when the pages never end", async () => {
    let n = 0;
    const fetchMock = vi.fn().mockImplementation(async () => {
      n += 1;
      return jsonResponse({ charges: [wireCharge(`c${n}`)], nextPageToken: `t${n}` });
    });
    vi.stubGlobal("fetch", fetchMock);

    await expect(listCharges(BASE, { cycleId: "cy1" })).rejects.toThrow(/página|page/i);
    expect(fetchMock).toHaveBeenCalledTimes(100);
  });

  it("throws when the gateway repeats a token instead of advancing", async () => {
    const fetchMock = vi.fn().mockImplementation(async () =>
      jsonResponse({ charges: [wireCharge("a")], nextPageToken: "same" }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(listCharges(BASE, { cycleId: "cy1" })).rejects.toThrow();
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("propagates a failed page instead of returning the pages before it", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ charges: [wireCharge("a")], nextPageToken: "t1" }))
      .mockResolvedValueOnce(new Response("invalid page token", { status: 400 }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(listCharges(BASE, { cycleId: "cy1" })).rejects.toThrow("invalid page token");
  });
});

describe("getOverdue walks every page", () => {
  it("keeps going past an EMPTY page that still carries a token", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ overdue: [wireCharge("a")], nextPageToken: "t1" }))
      .mockResolvedValueOnce(jsonResponse({ overdue: [], nextPageToken: "t2" }))
      .mockResolvedValueOnce(jsonResponse({ overdue: [wireCharge("z")] }));
    vi.stubGlobal("fetch", fetchMock);

    const rows = await getOverdue(BASE, "cy1");

    expect(rows.map((c) => c.id)).toEqual(["a", "z"]);
    expect(urlsOf(fetchMock)).toEqual([
      "http://gw/api/v1/finance/overdue?cycleId=cy1",
      "http://gw/api/v1/finance/overdue?cycleId=cy1&pageToken=t1",
      "http://gw/api/v1/finance/overdue?cycleId=cy1&pageToken=t2",
    ]);
  });

  it("throws instead of truncating when the pages never end", async () => {
    let n = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation(async () => {
        n += 1;
        return jsonResponse({ overdue: [], nextPageToken: `t${n}` });
      }),
    );

    await expect(getOverdue(BASE)).rejects.toThrow(/página|page/i);
  });
});
