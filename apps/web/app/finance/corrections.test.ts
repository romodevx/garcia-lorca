import { afterEach, describe, expect, it, vi } from "vitest";
import { correctDeposit, getCashUp, listDepositCorrections, listDeposits } from "./api";

const BASE = "http://gw";
const ID = "62000000-0000-0000-0000-000000000001";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

const original = {
  id: ID, studentId: "stu-1", amountCents: 50000, method: "cash", reference: "", paidAt: "2026-10-10T00:00:00Z",
  calendarVersion: "", folio: "P-2026-000001", supersededAt: "2026-10-11T00:00:00Z",
  supersededBy: "dep-2", supersededByFolio: "P-2026-000002",
};
const corrected = {
  id: "dep-2", studentId: "stu-1", amountCents: 100000, method: "cash", reference: "", paidAt: "2026-10-10T00:00:00Z",
  calendarVersion: "", folio: "P-2026-000002", correctsDepositId: ID, correctsFolio: "P-2026-000001",
};

describe("correctDeposit (T9)", () => {
  it("posts what the office typed, in cents, with the reason and the key", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({
      original, deposit: corrected, allocations: [], surcharges: [], unallocatedCents: 0, replayed: false,
      correction: { id: "c-1", originalDepositId: ID, correctedDepositId: "dep-2", reason: "monto", before: { amountCents: 50000, tenders: [], lines: [] }, after: { amountCents: 100000, tenders: [], lines: [] }, correctedAt: "2026-10-11T00:00:00Z" },
    }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await correctDeposit(BASE, ID, {
      amountCents: 100000, paidAt: "2026-10-10", reason: "monto", idempotencyKey: "k-1",
      tenders: [{ method: "cash", amountCents: 100000 }], chargeIds: ["ch-1"],
    });

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe(`http://gw/api/v1/finance/deposits/${ID}/correct`);
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body as string)).toEqual({
      amountCents: 100000, paidAt: "2026-10-10", reason: "monto", idempotencyKey: "k-1",
      tenders: [{ method: "cash", amountCents: 100000 }], chargeIds: ["ch-1"],
    });
    expect(res.original.supersededByFolio).toBe("P-2026-000002");
    expect(res.deposit.correctsFolio).toBe("P-2026-000001");
    expect(res.correction.reason).toBe("monto");
  });

  it("leaves out an empty date and an empty choice", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ original, deposit: corrected, correction: { before: {}, after: {} } }));
    vi.stubGlobal("fetch", fetchMock);
    await correctDeposit(BASE, ID, { amountCents: 100, paidAt: "", reason: "x", idempotencyKey: "k", tenders: [{ method: "cash", amountCents: 100 }], chargeIds: [] });
    const body = JSON.parse((fetchMock.mock.calls[0] as [string, RequestInit])[1].body as string) as Record<string, unknown>;
    expect(body).not.toHaveProperty("paidAt");
    expect(body).not.toHaveProperty("chargeIds");
  });

  it("surfaces the gateway's reason when it refuses", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("este pago ya fue corregido", { status: 409 })));
    await expect(correctDeposit(BASE, ID, { amountCents: 1, paidAt: "", reason: "x", idempotencyKey: "k", tenders: [], chargeIds: [] }))
      .rejects.toThrow("este pago ya fue corregido");
  });

  it("refuses an id that is not one before it reaches the network", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    await expect(correctDeposit(BASE, "../x", { amountCents: 1, paidAt: "", reason: "x", idempotencyKey: "k", tenders: [], chargeIds: [] })).rejects.toThrow();
    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe("listDepositCorrections (T9)", () => {
  it("reads the history", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ corrections: [{
      id: "c-1", originalDepositId: ID, correctedDepositId: "dep-2", reason: "monto", correctedByName: "Directora",
      before: { depositId: ID, folio: "P-1", amountCents: 50000, tenders: [{ method: "cash", amountCents: 50000 }], lines: [{ chargeId: "ch-1", description: "Septiembre", amountCents: 50000 }] },
      after: { depositId: "dep-2", folio: "P-2", amountCents: 100000, tenders: [{ method: "cash", amountCents: 100000 }], lines: [] },
      correctedAt: "2026-10-11T00:00:00Z",
    }] }));
    vi.stubGlobal("fetch", fetchMock);

    const got = await listDepositCorrections(BASE, ID);

    expect((fetchMock.mock.calls[0] as [string])[0]).toBe(`http://gw/api/v1/finance/deposits/${ID}/corrections`);
    expect(got).toHaveLength(1);
    expect(got[0]?.before.lines[0]?.description).toBe("Septiembre");
    expect(got[0]?.correctedByName).toBe("Directora");
  });
});

describe("deposits carry the correction link", () => {
  it("on the list and on the cash-up", async () => {
    vi.stubGlobal("fetch", vi.fn()
      .mockResolvedValueOnce(jsonResponse({ deposits: [original, corrected] }))
      .mockResolvedValueOnce(jsonResponse({ deposits: [corrected], cancelled: [], superseded: [original], payments: [] })));

    const [a, b] = await listDeposits(BASE, "stu-1");
    expect(a?.supersededByFolio).toBe("P-2026-000002");
    expect(a?.supersededAt).toBe("2026-10-11T00:00:00Z");
    expect(b?.correctsFolio).toBe("P-2026-000001");

    const cut = await getCashUp(BASE, "");
    expect(cut.superseded).toHaveLength(1);
    expect(cut.superseded[0]?.supersededByFolio).toBe("P-2026-000002");
  });
});
