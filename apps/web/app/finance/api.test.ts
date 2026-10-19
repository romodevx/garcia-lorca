import { afterEach, describe, expect, it, vi } from "vitest";
import {
  applyDiscount,
  cancelCharge,
  createCharge,
  createConcept,
  generateCharges,
  getAccount,
  getExpected,
  getIncome,
  getOverdue,
  listCharges,
  listConcepts,
  registerPayment,
  updateConcept,
} from "./api";

const BASE = "http://gw";

// `restoreAllMocks` does NOT undo `stubGlobal` — only `unstubAllGlobals` puts the
// real `fetch` back, so a stub cannot leak into the next test file.
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

/** Returns the [url, init] of the single fetch call the helper made. */
function lastCall(mock: ReturnType<typeof vi.fn>): [string, RequestInit] {
  const call = mock.mock.calls[0] as [string, RequestInit];
  return [call[0], call[1]];
}

const concept = {
  id: "c1",
  name: "Colegiatura",
  defaultAmountCents: 185000,
  recurrence: "monthly",
  active: true,
  kind: "custom",
};

const charge = {
  id: "ch1",
  studentId: "st1",
  cycleId: "cy1",
  conceptId: "c1",
  description: "Colegiatura",
  amountCents: 185000,
  discountCents: 0,
  dueDate: "2026-09-05",
  status: "pending",
  paidCents: 0,
  remainingCents: 185000,
};

const payment = {
  id: "p1",
  chargeId: "ch1",
  amountCents: 185000,
  method: "cash",
  reference: "REC-001",
  paidAt: "2026-09-01T10:00:00Z",
};

describe("registerPayment", () => {
  it("POSTs {chargeId, amountCents, method, reference} with the amount as a JSON NUMBER of cents", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ payment, charge: { ...charge, paidCents: 185000, remainingCents: 0, status: "paid" } }, 201));
    vi.stubGlobal("fetch", fetchMock);

    const res = await registerPayment(BASE, {
      chargeId: "ch1",
      amountCents: 185000,
      method: "cash",
      reference: "REC-001",
    });

    expect(res.payment).toEqual(payment);
    expect(res.charge.remainingCents).toBe(0);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/finance/payments");
    expect(init.method).toBe("POST");
    expect(init.credentials).toBe("include");
    const body = JSON.parse(init.body as string);
    expect(body).toEqual({ chargeId: "ch1", amountCents: 185000, method: "cash", reference: "REC-001" });
    // Money travels as integer CENTS in a JSON number — never a peso string/float.
    expect(typeof body.amountCents).toBe("number");
    expect(Number.isInteger(body.amountCents)).toBe(true);
  });

  it("sends the idempotency key so a retried submit cannot charge twice", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ payment, charge }, 201));
    vi.stubGlobal("fetch", fetchMock);

    await registerPayment(BASE, {
      chargeId: "ch1",
      amountCents: 185000,
      method: "cash",
      reference: "REC-001",
      idempotencyKey: "attempt-1",
    });

    const [, init] = lastCall(fetchMock);
    expect(JSON.parse(init.body as string)).toEqual({
      chargeId: "ch1",
      amountCents: 185000,
      method: "cash",
      reference: "REC-001",
      idempotencyKey: "attempt-1",
    });
  });

  it("throws with the gateway's error text so the ventanilla can surface it", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("payment exceeds remaining balance", { status: 400 })));
    await expect(
      registerPayment(BASE, { chargeId: "ch1", amountCents: 999999, method: "cash", reference: "" }),
    ).rejects.toThrow("payment exceeds remaining balance");
  });

  it("throws on a non-2xx with an empty body (falls back to the status)", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 403 })));
    await expect(
      registerPayment(BASE, { chargeId: "ch1", amountCents: 100, method: "cash", reference: "" }),
    ).rejects.toThrow("HTTP 403");
  });
});

describe("generateCharges", () => {
  it("POSTs the bulk request and returns {created, skipped}", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ created: 24, skipped: 3 }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await generateCharges(BASE, {
      conceptId: "c1",
      cycleId: "cy1",
      groupId: "g1",
      amountCentsOverride: 190000,
      dueDate: "2026-09-05",
    });

    expect(res).toEqual({ created: 24, skipped: 3 });
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/finance/charges/generate");
    expect(init.method).toBe("POST");
    const body = JSON.parse(init.body as string);
    expect(body).toEqual({
      conceptId: "c1",
      cycleId: "cy1",
      groupId: "g1",
      amountCentsOverride: 190000,
      dueDate: "2026-09-05",
    });
    expect(typeof body.amountCentsOverride).toBe("number");
  });

  it("returns zeroes when the response omits the counters", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({})));
    await expect(
      generateCharges(BASE, { conceptId: "c1", cycleId: "cy1", groupId: "", amountCentsOverride: 0, dueDate: "" }),
    ).resolves.toEqual({ created: 0, skipped: 0 });
  });
});

describe("getAccount", () => {
  it("GETs the statement by studentId and unwraps charges/payments/totalBalanceCents", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({ charges: [charge], payments: [payment], totalBalanceCents: 185000 }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const res = await getAccount(BASE, "st1");

    // The statement now also carries the family's credit and, since M18, what is
    // being asked for TODAY. An older gateway sends neither; the credit reads as
    // zero, and the day's figures come back with an EMPTY asOf — which is what
    // lets a screen withhold them instead of rendering $0.00 at a family that
    // owes money.
    expect(res).toEqual({
      charges: [charge],
      payments: [payment],
      totalBalanceCents: 185000,
      unallocatedCents: 0,
      dueTodayCents: 0,
      surchargeCents: 0,
      earlyDiscountCents: 0,
      asOf: "",
    });
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/finance/accounts/st1");
    expect(init.credentials).toBe("include");
  });

  it("defaults the statement to empty lists and a zero balance", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({})));
    await expect(getAccount(BASE, "st1")).resolves.toEqual({
      charges: [],
      payments: [],
      totalBalanceCents: 0,
      unallocatedCents: 0,
      dueTodayCents: 0,
      surchargeCents: 0,
      earlyDiscountCents: 0,
      asOf: "",
    });
  });
});

describe("listCharges", () => {
  it("GETs /finance/charges with the optional filters and unwraps {charges}", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ charges: [charge] }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await listCharges(BASE, { studentId: "st1", cycleId: "cy1" });

    expect(res).toEqual([charge]);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/finance/charges?studentId=st1&cycleId=cy1");
    expect(init.credentials).toBe("include");
    expect(init.method ?? "GET").toBe("GET");
  });

  it("drops empty filters from the query string", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ charges: [] }));
    vi.stubGlobal("fetch", fetchMock);
    await listCharges(BASE, { studentId: "st1" });
    expect(lastCall(fetchMock)[0]).toBe("http://gw/api/v1/finance/charges?studentId=st1");
  });
});

describe("listConcepts", () => {
  it("GETs /finance/concepts and unwraps {concepts}", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ concepts: [concept] }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await listConcepts(BASE);

    expect(res).toEqual([concept]);
    expect(lastCall(fetchMock)[0]).toBe("http://gw/api/v1/finance/concepts");
  });

  it("asks for the inactive ones too when requested", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ concepts: [] }));
    vi.stubGlobal("fetch", fetchMock);
    await listConcepts(BASE, true);
    expect(lastCall(fetchMock)[0]).toBe("http://gw/api/v1/finance/concepts?includeInactive=true");
  });

  it("returns [] when the envelope key is absent", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({})));
    await expect(listConcepts(BASE)).resolves.toEqual([]);
  });
});

describe("getOverdue", () => {
  it("GETs /finance/overdue with the optional cycle filter and unwraps {overdue}", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ overdue: [charge] }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await getOverdue(BASE, "cy1");

    expect(res).toEqual([charge]);
    expect(lastCall(fetchMock)[0]).toBe("http://gw/api/v1/finance/overdue?cycleId=cy1");
  });

  it("omits the cycle filter when none is given", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ overdue: [] }));
    vi.stubGlobal("fetch", fetchMock);
    await getOverdue(BASE);
    expect(lastCall(fetchMock)[0]).toBe("http://gw/api/v1/finance/overdue");
  });

  it("throws on a non-2xx response", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 404 })));
    await expect(getOverdue(BASE)).rejects.toThrow();
  });
});

describe("concepts write path", () => {
  it("createConcept POSTs {name, defaultAmountCents, recurrence} with cents as a number", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(concept, 201));
    vi.stubGlobal("fetch", fetchMock);

    const res = await createConcept(BASE, { name: "Colegiatura", defaultAmountCents: 185000, recurrence: "monthly" });

    expect(res).toEqual(concept);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/finance/concepts");
    expect(init.method).toBe("POST");
    const body = JSON.parse(init.body as string);
    expect(body).toEqual({ name: "Colegiatura", defaultAmountCents: 185000, recurrence: "monthly" });
    expect(typeof body.defaultAmountCents).toBe("number");
  });

  it("updateConcept PUTs the full mutable shape (including active) to the concept's id", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ ...concept, active: false }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await updateConcept(BASE, "c1", {
      name: "Colegiatura",
      defaultAmountCents: 190000,
      recurrence: "monthly",
      active: false,
    });

    expect(res.active).toBe(false);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/finance/concepts/c1");
    expect(init.method).toBe("PUT");
    expect(JSON.parse(init.body as string)).toEqual({
      name: "Colegiatura",
      defaultAmountCents: 190000,
      recurrence: "monthly",
      active: false,
    });
  });
});

describe("charges write path", () => {
  it("listConcepts reads the kind, and an unknown or absent one is the school's own (CC-404)", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ concepts: [
      { id: "k1", name: "Colegiatura", defaultAmountCents: 1, recurrence: "monthly", active: true, kind: "tuition" },
      { id: "k2", name: "Credencial", defaultAmountCents: 1, recurrence: "once", active: true },
    ] })));

    const got = await listConcepts(BASE);

    expect(got.map((c) => c.kind)).toEqual(["tuition", "custom"]);
  });

  it("createCharge POSTs the single-charge shape", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(charge, 201));
    vi.stubGlobal("fetch", fetchMock);

    await createCharge(BASE, {
      studentId: "st1",
      cycleId: "cy1",
      conceptId: "c1",
      amountCentsOverride: 0,
      descriptionOverride: "",
      dueDate: "2026-09-05",
    });

    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/finance/charges");
    expect(JSON.parse(init.body as string)).toEqual({
      studentId: "st1",
      cycleId: "cy1",
      conceptId: "c1",
      amountCentsOverride: 0,
      descriptionOverride: "",
      dueDate: "2026-09-05",
    });
  });

  it("applyDiscount POSTs {discountCents, reason} to the charge's discount route", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ ...charge, discountCents: 5000, remainingCents: 180000 }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await applyDiscount(BASE, "ch1", 5000, "beca parcial");

    expect(res.discountCents).toBe(5000);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/finance/charges/ch1/discount");
    expect(init.method).toBe("POST");
    const body = JSON.parse(init.body as string);
    expect(body).toEqual({ discountCents: 5000, reason: "beca parcial" });
    expect(typeof body.discountCents).toBe("number");
  });

  it("narrows an unrecognized wire enum instead of widening it into the union", async () => {
    // A gateway newer than this build: the value must not reach the UI as-is,
    // because every renderer treats `status`/`method` as an exhaustive union.
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ charges: [{ ...charge, status: "refunded" }] })));
    await expect(listCharges(BASE)).resolves.toMatchObject([{ id: "ch1", status: "pending" }]);

    vi.unstubAllGlobals();
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(jsonResponse({ payments: [{ ...payment, method: "crypto" }], totalBalanceCents: 0 })),
    );
    await expect(getAccount(BASE, "st1")).resolves.toMatchObject({ payments: [{ id: "p1", method: "cash" }] });
  });

  it("a refused write carries its HTTP status, so the screen can tell 409 from 403", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("charge already cancelled", { status: 409 })));

    await expect(cancelCharge(BASE, "ch1", "Duplicado")).rejects.toMatchObject({ status: 409, message: "charge already cancelled" });
  });

  it("cancelCharge POSTs the cancel route with the reason (CC-203)", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ ...charge, status: "cancelled" }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await cancelCharge(BASE, "ch1", "Cargo duplicado");

    expect(res.status).toBe("cancelled");
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/finance/charges/ch1/cancel");
    expect(init.method).toBe("POST");
    expect(JSON.parse(String(init.body))).toEqual({ reason: "Cargo duplicado" });
  });
});

describe("registerPayment · tenders (tarea 8)", () => {
  it("sends the breakdown and reads one payment per tender", async () => {
    const second = { ...payment, id: "p2", method: "credit_note", amountCents: 50000 };
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ payment, payments: [payment, second], charge }, 201));
    vi.stubGlobal("fetch", fetchMock);
    const tenders = [
      { method: "cash" as const, amountCents: 185000 },
      { method: "credit_note" as const, amountCents: 50000, reference: "NC-1" },
    ];

    const res = await registerPayment(BASE, { chargeId: "ch1", amountCents: 235000, method: "cash", reference: "", tenders });

    expect(JSON.parse(lastCall(fetchMock)[1].body as string).tenders).toEqual(tenders);
    expect(res.payments.map((p) => p.method)).toEqual(["cash", "credit_note"]);
  });

  it("reads a response without payments as the single one", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ payment, charge }, 201)));
    const res = await registerPayment(BASE, { chargeId: "ch1", amountCents: 185000, method: "cash", reference: "" });
    expect(res.payments).toEqual([payment]);
  });
});

describe("getIncome · credit notes (tarea 8)", () => {
  it("keeps the period's credit notes apart from the money", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ lines: [], totalCents: 100000, creditNoteCents: 50000 })));
    const income = await getIncome(BASE, "2026-10-01", "2026-10-31");
    expect(income).toEqual({ lines: [], totalCents: 100000, creditNoteCents: 50000 });
  });
});

describe("getExpected · scholarships (T18)", () => {
  const line = { chargeId: "ch1", description: "Colegiatura septiembre", dueDate: "2026-09-10", owedCents: 160000, surchargeCents: 0, discountCents: 0, totalCents: 160000, due: true };

  it("carries what the scholarship takes off each line", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ amountCents: 160000, lines: [{ ...line, scholarshipCents: 160000 }] })));
    const got = await getExpected(BASE, "st1");
    expect(got.lines[0]?.scholarshipCents).toBe(160000);
  });

  it("a gateway older than T18 reads as no scholarship, never undefined", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ amountCents: 160000, lines: [line] })));
    const got = await getExpected(BASE, "st1");
    expect(got.lines[0]?.scholarshipCents).toBe(0);
  });
});

// D1 (T5 review): the server says which lines the balance holds and what it
// comes to paid on time; the search may leave the balance out.
describe("getExpected · the balance it holds", () => {
  const line = { chargeId: "ch1", description: "Inscripción", dueDate: "2026-06-30", owedCents: 50000, surchargeCents: 0, discountCents: 0, scholarshipCents: 0, totalCents: 50000, due: false };

  it("carries inBalance and the on-time balance", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ amountCents: 0, annualBalanceCents: 3200000, onTimeBalanceCents: 3000000, lines: [{ ...line, inBalance: false }] })));
    const got = await getExpected(BASE, "st1");
    expect(got.onTimeBalanceCents).toBe(3000000);
    expect(got.lines[0]?.inBalance).toBe(false);
  });

  it("a gateway that does not say reads every line as in the balance", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ amountCents: 0, annualBalanceCents: 50000, onTimeBalanceCents: 50000, lines: [line] })));
    const got = await getExpected(BASE, "st1");
    expect(got.lines[0]?.inBalance).toBe(true);
    expect(got.onTimeBalanceCents).toBe(50000);
  });

  it("asks without the balance when told to", async () => {
    const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(jsonResponse({ amountCents: 0, lines: [] })));
    vi.stubGlobal("fetch", fetchMock);
    await getExpected(BASE, "st1", "", { withBalance: false });
    expect(String(fetchMock.mock.calls[0]?.[0])).toContain("withBalance=false");
    await getExpected(BASE, "st1");
    expect(String(fetchMock.mock.calls[1]?.[0])).not.toContain("withBalance");
  });
});
