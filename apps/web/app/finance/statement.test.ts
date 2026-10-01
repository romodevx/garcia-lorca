import { afterEach, describe, expect, it, vi } from "vitest";

import { getStatement, StatementError, statementPdf } from "./statement";

const BASE = "http://gw";

afterEach(() => vi.restoreAllMocks());

const wire = {
  movements: [
    { kind: "charge", date: "2026-09-10", description: "Colegiatura septiembre", amountCents: 320000, balanceCents: 320000, chargeId: "c-1", accrued: false, notYetDue: false, voided: false },
    { kind: "payment", date: "2026-09-12", description: "Pago", amountCents: 100000, balanceCents: 220000, depositId: "d-1", method: "card", cardType: "debit", folio: "P-2026-000042", accrued: false, notYetDue: false, voided: true, voidReason: "dos veces" },
  ],
  totals: { chargedCents: 320000, discountCents: 0, paidCents: 100000, balanceCents: 220000, notYetDueCents: 0 },
  dueTodayCents: 220000, surchargeCents: 0, earlyDiscountCents: 0, unallocatedCents: 0, asOf: "2026-10-01",
};

describe("statement api (T7)", () => {
  it("reads the office's ledger with the session cookie", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(wire), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    const got = await getStatement(BASE, "st-1");

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("http://gw/api/v1/finance/accounts/st-1/statement");
    expect(init.credentials).toBe("include");
    expect(got.movements).toHaveLength(2);
    expect(got.movements[1]).toMatchObject({ kind: "payment", voided: true, voidReason: "dos veces", folio: "P-2026-000042" });
    expect(got.totals.balanceCents).toBe(220000);
    expect(got.asOf).toBe("2026-10-01");
  });

  it("fills what an older gateway leaves out instead of crashing on it", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({}), { status: 200 })));

    const got = await getStatement(BASE, "st-1");

    expect(got.movements).toEqual([]);
    expect(got.totals).toEqual({ chargedCents: 0, discountCents: 0, paidCents: 0, balanceCents: 0, notYetDueCents: 0 });
  });

  it("fetches the PDF as bytes with the gateway's file name", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response("%PDF-1.4", {
      status: 200,
      headers: { "Content-Disposition": 'attachment; filename="estado-de-cuenta-2026-10-01.pdf"' },
    }));
    vi.stubGlobal("fetch", fetchMock);

    const got = await statementPdf(BASE, "st-1");

    expect((fetchMock.mock.calls[0] as [string])[0]).toBe("http://gw/api/v1/finance/accounts/st-1/statement/pdf");
    expect(got.filename).toBe("estado-de-cuenta-2026-10-01.pdf");
  });

  it("refuses an id that is not one before asking anybody", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    await expect(getStatement(BASE, "../users")).rejects.toBeInstanceOf(StatementError);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("says why a read failed", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("forbidden", { status: 403 })));

    await expect(getStatement(BASE, "st-1")).rejects.toMatchObject({ status: 403 });
  });
});

describe("downloadStatementPdf", () => {
  it("hands the PDF to the shared download helper with the gateway's name", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("%PDF-1.4", {
      status: 200,
      headers: { "Content-Disposition": 'attachment; filename="estado-de-cuenta-2026-10-01.pdf"' },
    })));
    const blobs = await import("../_lib/downloadBlob");
    const spy = vi.spyOn(blobs, "downloadBlob").mockImplementation(() => {});
    const { downloadStatementPdf } = await import("./statement");

    await downloadStatementPdf(BASE, "st-1");

    expect(spy).toHaveBeenCalledWith(expect.any(Blob), "estado-de-cuenta-2026-10-01.pdf");
  });
});
