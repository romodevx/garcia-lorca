import { afterEach, describe, expect, it, vi } from "vitest";
import { getCashUp, getExpected, listDeposits, previewDeposit, registerDeposit, voidDeposit } from "./api";

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

const deposit = {
  id: "dep-1",
  studentId: "stu-1",
  amountCents: 670000,
  method: "cash",
  reference: "",
  paidAt: "2026-10-10T00:00:00Z",
  calendarVersion: "abc123",
};

describe("registerDeposit", () => {
  it("sends the amount as integer cents and the day the money changed hands", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({ deposit, allocations: [], surcharges: [], unallocatedCents: 0, replayed: false }, 201),
    );
    vi.stubGlobal("fetch", fetchMock);

    const res = await registerDeposit(BASE, {
      studentId: "stu-1",
      amountCents: 670000,
      method: "cash",
      reference: "",
      paidAt: "2026-10-10",
      idempotencyKey: "k1",
    });

    expect(res.deposit.amountCents).toBe(670000);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/finance/deposits");
    expect(init.method).toBe("POST");
    const body = JSON.parse(init.body as string) as Record<string, unknown>;
    expect(body.paidAt).toBe("2026-10-10");
    expect(body.idempotencyKey).toBe("k1");
    expect(Number.isInteger(body.amountCents)).toBe(true);
  });

  it("omits an empty paidAt so the server stamps today, as it always did", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({ deposit, allocations: [], surcharges: [], unallocatedCents: 0, replayed: false }, 201),
    );
    vi.stubGlobal("fetch", fetchMock);

    await registerDeposit(BASE, { studentId: "stu-1", amountCents: 100000, method: "cash", reference: "" });

    const body = JSON.parse(lastCall(fetchMock)[1].body as string) as Record<string, unknown>;
    expect("paidAt" in body).toBe(false);
  });

  it("surfaces the gateway's own words when the money is refused", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("la fecha del pago no puede estar en el futuro", { status: 400 })));

    await expect(
      registerDeposit(BASE, { studentId: "stu-1", amountCents: 100000, method: "cash", reference: "", paidAt: "2027-01-01" }),
    ).rejects.toThrow("la fecha del pago no puede estar en el futuro");
  });
});

describe("previewDeposit", () => {
  it("asks what the money would cover without a method or a key — nothing is taken", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({ allocations: [{ id: "p1", chargeId: "ch1", amountCents: 320000, method: "cash", reference: "", paidAt: "" }], surcharges: [], unallocatedCents: 350000 }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const res = await previewDeposit(BASE, { studentId: "stu-1", amountCents: 670000, paidAt: "2026-10-10" });

    expect(res.allocations).toHaveLength(1);
    expect(res.unallocatedCents).toBe(350000);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/finance/deposits/preview");
    const body = JSON.parse(init.body as string) as Record<string, unknown>;
    expect("idempotencyKey" in body).toBe(false);
  });
});

describe("listDeposits", () => {
  it("unwraps the envelope", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ deposits: [deposit] })));

    const res = await listDeposits(BASE, "stu-1");

    expect(res).toHaveLength(1);
    expect(res[0]?.id).toBe("dep-1");
  });
});

describe("voidDeposit", () => {
  it("sends the reason, which the counter cannot skip", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await voidDeposit(BASE, "dep-1", "importe capturado mal");

    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/finance/deposits/dep-1/void");
    expect(JSON.parse(init.body as string)).toEqual({ reason: "importe capturado mal" });
  });
});

// Regression: ISSUE-003 — the cash-up dropped who took the money and when it
// was keyed, so the "capturado el …" mark never reached the screen.
// Found by /qa on 2026-09-24
describe("getCashUp · who took the money (tarea 7)", () => {
  it("keeps the receiver and the moment of capture", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({
      deposits: [{
        id: "d1", studentId: "s1", amountCents: 280000, method: "cash", reference: "",
        paidAt: "2026-09-01T00:00:00Z", calendarVersion: "", receivedByName: "Director", capturedAt: "2026-09-24T21:57:13Z",
      }],
      cancelled: [], payments: [], cashCents: 280000, cardCents: 0, transferCents: 0, totalCents: 280000,
    }));
    vi.stubGlobal("fetch", fetchMock);

    const cut = await getCashUp(BASE, "2026-09-01");

    expect(cut.deposits[0]).toMatchObject({ receivedByName: "Director", capturedAt: "2026-09-24T21:57:13Z" });
  });
});

// Task 8: the debt by concept survives the client's mapping, and the chosen
// charges reach the server.
describe("the counter's choice of charges (tarea 8)", () => {
  it("keeps the expected lines", async () => {
    const line = {
      chargeId: "ch-sep", description: "Colegiatura septiembre", dueDate: "2026-09-10",
      owedCents: 320000, surchargeCents: 30000, discountCents: 0, totalCents: 350000, due: true,
    };
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ amountCents: 350000, lines: [line] })));
    const expected = await getExpected(BASE, "s1");
    // A gateway older than T18 sends no scholarship: it reads as 0.
    expect(expected.lines).toEqual([{ ...line, scholarshipCents: 0 }]);
  });

  it("sends the chosen charges on the deposit and on its preview", async () => {
    const fetchMock = vi.fn().mockImplementation(async () => jsonResponse({ deposit, allocations: [], surcharges: [], unallocatedCents: 0 }));
    vi.stubGlobal("fetch", fetchMock);
    await registerDeposit(BASE, { studentId: "s1", amountCents: 100, method: "cash", reference: "", chargeIds: ["ch-oct"] });
    await previewDeposit(BASE, { studentId: "s1", amountCents: 100, chargeIds: ["ch-oct"] });
    for (const call of fetchMock.mock.calls) {
      expect(JSON.parse((call[1] as RequestInit).body as string).chargeIds).toEqual(["ch-oct"]);
    }
  });
});

// T8: a credit note is a tender, and a payment may be split across several.
describe("tenders (tarea 8 · nota de crédito + pago combinado)", () => {
  const tenders = [
    { method: "credit_note" as const, amountCents: 50000, reference: "NC-12" },
    { method: "card" as const, amountCents: 100000, cardType: "debit" as const },
  ];

  it("sends the breakdown on the deposit", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ deposit, allocations: [], surcharges: [], unallocatedCents: 0 }, 201));
    vi.stubGlobal("fetch", fetchMock);

    await registerDeposit(BASE, { studentId: "s1", amountCents: 150000, method: "credit_note", reference: "", tenders });

    const body = JSON.parse(lastCall(fetchMock)[1].body as string) as Record<string, unknown>;
    expect(body.tenders).toEqual(tenders);
    expect(body.amountCents).toBe(150000);
  });

  it("reads a deposit's tenders, and backfills one for a gateway that sends none", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({
      deposits: [
        { ...deposit, id: "d-mixed", amountCents: 150000, method: "mixed", tenders },
        { ...deposit, id: "d-old", method: "card", cardType: "credit", reference: "V-9" },
      ],
    })));

    const [mixed, old] = await listDeposits(BASE, "s1");

    expect(mixed?.method).toBe("mixed");
    expect(mixed?.tenders).toEqual(tenders);
    expect(old?.tenders).toEqual([{ method: "card", amountCents: 670000, reference: "V-9", cardType: "credit" }]);
  });

  it("keeps the credit notes of the day apart from the money", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({
      deposits: [], cancelled: [], payments: [], cashCents: 100000, cardCents: 0, transferCents: 0, totalCents: 100000, creditNoteCents: 50000,
    })));

    const cut = await getCashUp(BASE, "");

    expect(cut.creditNoteCents).toBe(50000);
    expect(cut.totalCents).toBe(100000);
  });

  it("reads a missing creditNoteCents as zero", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ deposits: [] })));
    expect((await getCashUp(BASE, "")).creditNoteCents).toBe(0);
  });
});
