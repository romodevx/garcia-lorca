import { afterEach, describe, expect, it, vi } from "vitest";

import {
  cancelExpense,
  closeCashCut,
  createCategory,
  createExpense,
  downloadReport,
  getExpense,
  getReport,
  getSummary,
  listCategories,
  listExpenses,
  reopenCashCut,
  updateCategory,
  updateExpense,
  uploadReceipt,
} from "./api";

const BASE = "http://gw/";
const ID = "e1000000-0000-0000-0000-000000000001";
const CAT = "c1000000-0000-0000-0000-000000000001";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

function stub(body: unknown, status = 200): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn().mockImplementation(() => Promise.resolve(jsonResponse(body, status)));
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function call(fetchMock: ReturnType<typeof vi.fn>, i = 0): [string, RequestInit] {
  return fetchMock.mock.calls[i] as [string, RequestInit];
}

const expense = {
  id: ID, spentOn: "2026-10-01", categoryId: CAT, categoryName: "Papelería", amountCents: 125050,
  method: "cash", description: "Tóner", status: "active",
};

describe("expenses api (T10)", () => {
  it("lists a range's expenses, cancelled ones when asked", async () => {
    const fetchMock = stub({ expenses: [expense] });
    const got = await listExpenses(BASE, { from: "2026-10-01", to: "2026-10-31", includeCancelled: true });
    expect(call(fetchMock)[0]).toBe("http://gw/api/v1/expenses?from=2026-10-01&to=2026-10-31&includeCancelled=true");
    expect(got[0]?.amountCents).toBe(125050);
    expect(got[0]?.method).toBe("cash");
  });

  it("an unknown method or status degrades instead of breaking the row", async () => {
    stub({ expenses: [{ ...expense, method: "cheque", status: "lost" }] });
    const [got] = await listExpenses(BASE, { from: "2026-10-01", to: "2026-10-31" });
    expect(got?.method).toBe("cash");
    expect(got?.status).toBe("active");
  });

  it("creates in cents, never sending who recorded it", async () => {
    const fetchMock = stub(expense, 201);
    await createExpense(BASE, { spentOn: "2026-10-01", categoryId: CAT, amountCents: 125050, method: "cash", description: "Tóner" });
    const [url, init] = call(fetchMock);
    expect(url).toBe("http://gw/api/v1/expenses");
    expect(init.method).toBe("POST");
    expect(JSON.parse(String(init.body))).toEqual({ spentOn: "2026-10-01", categoryId: CAT, amountCents: 125050, method: "cash", description: "Tóner" });
  });

  it("edits with a reason and cancels with one", async () => {
    const fetchMock = stub(expense);
    await updateExpense(BASE, ID, { spentOn: "2026-10-01", categoryId: CAT, amountCents: 1, method: "card", description: "", reason: "ajuste" });
    expect(call(fetchMock)[0]).toBe(`http://gw/api/v1/expenses/${ID}`);
    expect(call(fetchMock)[1].method).toBe("PUT");
    expect(JSON.parse(String(call(fetchMock)[1].body)).reason).toBe("ajuste");
    await cancelExpense(BASE, ID, "duplicado");
    expect(call(fetchMock, 1)[0]).toBe(`http://gw/api/v1/expenses/${ID}/cancel`);
    expect(JSON.parse(String(call(fetchMock, 1)[1].body))).toEqual({ reason: "duplicado" });
  });

  it("refuses an id that is not an id before any request", async () => {
    const fetchMock = stub(expense);
    await expect(getExpense(BASE, "../x")).rejects.toThrow();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("reads an expense with its history", async () => {
    stub({ expense, events: [{ id: "v1", kind: "edited", before: { amountCents: 1 }, after: { amountCents: 2 }, actorName: "Caja", at: "2026-10-01T15:00:00Z" }] });
    const got = await getExpense(BASE, ID);
    expect(got.events[0]?.kind).toBe("edited");
    expect(got.events[0]?.before.amountCents).toBe(1);
  });

  it("uploads the receipt as the raw body", async () => {
    const fetchMock = stub({ ...expense, receipt: { contentType: "application/pdf", sizeBytes: 3 } });
    const file = new Blob(["%PDF"], { type: "application/pdf" });
    const got = await uploadReceipt(BASE, ID, file);
    const [url, init] = call(fetchMock);
    expect(url).toBe(`http://gw/api/v1/expenses/${ID}/receipt`);
    expect(init.method).toBe("PUT");
    expect(init.body).toBe(file);
    expect(got.receipt?.contentType).toBe("application/pdf");
  });

  it("refuses a receipt over 5 MB without sending it", async () => {
    const fetchMock = stub(expense);
    const big = new Blob([new Uint8Array(5 * 1024 * 1024 + 1)], { type: "application/pdf" });
    await expect(uploadReceipt(BASE, ID, big)).rejects.toThrow(/5 MB/);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("manages categories", async () => {
    const fetchMock = stub({ categories: [{ id: CAT, name: "Luz", active: true }] });
    const got = await listCategories(BASE, true);
    expect(call(fetchMock)[0]).toBe("http://gw/api/v1/expense-categories?includeInactive=true");
    expect(got[0]?.name).toBe("Luz");
    await createCategory(BASE, "Agua");
    expect(JSON.parse(String(call(fetchMock, 1)[1].body))).toEqual({ name: "Agua" });
    await updateCategory(BASE, CAT, { name: "Agua", active: false });
    expect(call(fetchMock, 2)[0]).toBe(`http://gw/api/v1/expense-categories/${CAT}`);
    expect(JSON.parse(String(call(fetchMock, 2)[1].body))).toEqual({ name: "Agua", active: false });
  });

  it("reads the summary and the report, filling what is missing", async () => {
    const fetchMock = stub({ income: { cashCents: 10, cardCents: 0, transferCents: 0, totalCents: 10 } });
    const s = await getSummary(BASE, "2026-10-01", "2026-10-31");
    expect(call(fetchMock)[0]).toBe("http://gw/api/v1/expenses/summary?from=2026-10-01&to=2026-10-31");
    expect(s.income.cashCents).toBe(10);
    expect(s.balance.totalCents).toBe(0);
    expect(s.expensesByCategory).toEqual([]);
    await getReport(BASE, "weekly", "2026-10-15");
    expect(call(fetchMock, 1)[0]).toBe("http://gw/api/v1/expenses/reports?period=weekly&date=2026-10-15");
  });

  it("downloads an export with the session", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(new Blob(["PK"]), { status: 200, headers: { "Content-Disposition": 'attachment; filename="reporte-finanzas-monthly-2026-10-01.xlsx"' } }));
    vi.stubGlobal("fetch", fetchMock);
    const got = await downloadReport(BASE, "monthly", "2026-10-15", "xlsx");
    expect(call(fetchMock)[0]).toBe("http://gw/api/v1/expenses/reports/xlsx?period=monthly&date=2026-10-15");
    expect(got.filename).toBe("reporte-finanzas-monthly-2026-10-01.xlsx");
  });

  it("closes and reopens the cash cut", async () => {
    const fetchMock = stub({ id: "cl", day: "2026-10-01", countedCashCents: 165000 });
    await closeCashCut(BASE, { day: "2026-10-01", countedCashCents: 165000, note: "ok" });
    expect(call(fetchMock)[0]).toBe("http://gw/api/v1/finance/cash-up/close");
    expect(JSON.parse(String(call(fetchMock)[1].body))).toEqual({ day: "2026-10-01", countedCashCents: 165000, note: "ok" });
    await reopenCashCut(BASE, "2026-10-01", "pago corregido");
    expect(call(fetchMock, 1)[0]).toBe("http://gw/api/v1/finance/cash-up/reopen");
  });

  it("surfaces the gateway's sentence on a refusal", async () => {
    stub("", 409);
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("el corte de ese día ya está cerrado", { status: 409 })));
    await expect(closeCashCut(BASE, { day: "2026-10-01", countedCashCents: 0, note: "" })).rejects.toThrow("el corte de ese día ya está cerrado");
  });
});
