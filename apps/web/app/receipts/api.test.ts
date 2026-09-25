import { afterEach, describe, expect, it, vi } from "vitest";

import { getReceipt, receiptPdf, ReceiptError } from "./api";

const BASE = "http://gw";

afterEach(() => vi.restoreAllMocks());

const receipt = {
  kind: "payment", folio: "P-2026-000042", school: { name: "COLEGIO", legalName: "", rfc: "", cct: "", address: "", phone: "", legend: "" },
  paidOn: "2026-10-10", buyer: "Camila", lines: [], surchargeCents: 0, discountCents: 0, creditCents: 0, totalCents: 350000, method: "Efectivo",
};

describe("receipts api (tarea 12)", () => {
  it("reads a payment's receipt with the session cookie", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(receipt), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    const got = await getReceipt(BASE, "payment", "dep-1");

    expect(got.folio).toBe("P-2026-000042");
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("http://gw/api/v1/receipts/payment/dep-1");
    expect(init.credentials).toBe("include");
  });

  it("fetches a sale's PDF as bytes, named after its folio", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response("%PDF-1.4", {
      status: 200,
      headers: { "Content-Disposition": 'attachment; filename="comprobante-U-2026-000007.pdf"' },
    }));
    vi.stubGlobal("fetch", fetchMock);

    const got = await receiptPdf(BASE, "sale", "ord-1");

    expect((fetchMock.mock.calls[0] as [string])[0]).toBe("http://gw/api/v1/receipts/sale/ord-1/pdf");
    expect(got.filename).toBe("comprobante-U-2026-000007.pdf");
    expect(await got.blob.text()).toBe("%PDF-1.4");
  });

  it("refuses an id that is not one before asking anybody", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    await expect(getReceipt(BASE, "payment", "../users")).rejects.toBeInstanceOf(ReceiptError);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("says a missing receipt is missing", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("not found", { status: 404 })));

    await expect(getReceipt(BASE, "sale", "ord-9")).rejects.toMatchObject({ status: 404 });
  });
});
