import { afterEach, describe, expect, it, vi } from "vitest";

import { listDepositCorrections, listDeposits } from "./api";
import { depositDay } from "./depositDay";

// ADR 0035: a deposit's day (paidOn) travels apart from its instant (paidAt).
// A live payment at 19:00 in Morelia on the 1st is 01:00 UTC on the 2nd.

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const json = (body: unknown): Response =>
  new Response(JSON.stringify(body), { status: 200, headers: { "Content-Type": "application/json" } });

const liveEvening = {
  id: "dep-1", studentId: "stu-1", amountCents: 100, method: "cash", reference: "",
  paidAt: "2026-10-02T01:00:00Z", paidOn: "2026-10-01", calendarVersion: "",
};

describe("paidOn", () => {
  it("reaches the deposit as sent", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json({ deposits: [liveEvening] })));
    const [d] = await listDeposits("http://gw", "stu-1");
    expect(d?.paidOn).toBe("2026-10-01");
    expect(d?.paidAt).toBe("2026-10-02T01:00:00Z");
  });

  it("reaches a correction's snapshots", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json({ corrections: [{
      id: "c", before: { depositId: "a", paidAt: "2026-10-02T01:00:00Z", paidOn: "2026-10-01" },
      after: { depositId: "b", paidAt: "2026-10-02T01:00:00Z" },
    }] })));
    const [c] = await listDepositCorrections("http://gw", "62000000-0000-0000-0000-000000000001");
    expect(c?.before.paidOn).toBe("2026-10-01");
    expect(c?.after.paidOn).toBeUndefined();
  });
});

describe("depositDay", () => {
  const cases: ReadonlyArray<[string, { paidAt?: string; paidOn?: string }, string]> = [
    ["the day the server sent", { paidAt: "2026-10-02T01:00:00Z", paidOn: "2026-10-01" }, "2026-10-01"],
    ["an older server: the stored date of paidAt", { paidAt: "2026-09-09T00:00:00Z" }, "2026-09-09"],
    // Migration 0028's rule for a snapshot written before it: exactly midnight
    // UTC is a day picked at the counter (its UTC date); any other instant is
    // live, dated on the school's day — 19:00 in Morelia on the 1st.
    ["an older snapshot of a live evening payment: Morelia's day", { paidAt: "2026-10-02T01:00:00Z" }, "2026-10-01"],
    ["an older snapshot at exactly midnight UTC: the day picked", { paidAt: "2026-10-02T00:00:00.000Z" }, "2026-10-02"],
    ["an older snapshot with an offset: Morelia's day", { paidAt: "2026-10-01T19:00:00-06:00" }, "2026-10-01"],
    ["nothing at all", {}, ""],
  ];
  it.each(cases)("%s", (_name, d, want) => {
    expect(depositDay(d)).toBe(want);
  });
});
