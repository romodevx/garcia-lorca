import { afterEach, describe, expect, it, vi } from "vitest";
import { FinanceError, registerDeposit } from "./api";
import { getVoucherFile, listPendingVouchers, rejectVoucher, VOUCHER_CODES } from "./vouchers";

const BASE = "http://gw";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

function firstCall(mock: ReturnType<typeof vi.fn>): [string, RequestInit] {
  const call = mock.mock.calls[0] as [string, RequestInit];
  return [call[0], call[1]];
}

const wireVoucher = {
  id: "v1",
  studentId: "s1",
  status: "pending",
  claimedAmountCents: 320000,
  claimedPaidOn: "2026-10-20",
  fileContentType: "image/jpeg",
  fileSizeBytes: 4000,
  uploadedByName: "Rosa Pérez",
  createdAt: "2026-10-20T18:04:00Z",
};

const deposit = {
  id: "dep-1", studentId: "s1", amountCents: 320000, method: "transfer", reference: "",
  paidAt: "2026-10-20T00:00:00Z", calendarVersion: "abc",
};

describe("listPendingVouchers", () => {
  it("asks for the pending queue, oldest first, and keeps what the family claimed", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ vouchers: [wireVoucher] }));
    vi.stubGlobal("fetch", fetchMock);

    const rows = await listPendingVouchers(BASE);

    expect(firstCall(fetchMock)[0]).toBe("http://gw/api/v1/finance/payment-vouchers?status=pending");
    expect(firstCall(fetchMock)[1].credentials).toBe("include");
    expect(rows).toEqual([wireVoucher]);
  });

  it("narrows to one pupil when asked", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ vouchers: [] }));
    vi.stubGlobal("fetch", fetchMock);

    await listPendingVouchers(BASE, "s 1");

    expect(firstCall(fetchMock)[0]).toBe("http://gw/api/v1/finance/payment-vouchers?status=pending&studentId=s+1");
  });

  it("drops claims the family did not make rather than reading them as zero", async () => {
    const { claimedAmountCents: _a, claimedPaidOn: _p, ...bare } = wireVoucher;
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ vouchers: [bare] })));

    const [row] = await listPendingVouchers(BASE);

    expect(row !== undefined && "claimedAmountCents" in row).toBe(false);
    expect(row !== undefined && "claimedPaidOn" in row).toBe(false);
  });
});

describe("getVoucherFile", () => {
  it("reads the file with the session and types it only as one of the three allowed kinds", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response("%PDF", { status: 200, headers: { "Content-Type": "text/html" } }));
    vi.stubGlobal("fetch", fetchMock);

    const blob = await getVoucherFile(BASE, { id: "v1", fileContentType: "application/pdf" });

    expect(firstCall(fetchMock)[0]).toBe("http://gw/api/v1/finance/payment-vouchers/v1/file");
    expect(firstCall(fetchMock)[1].credentials).toBe("include");
    // Whatever the response said, the preview never gets an HTML blob.
    expect(blob.type).toBe("application/pdf");
  });

  it("throws the gateway's sentence when the file is gone", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("comprobante no encontrado", { status: 404 })));

    await expect(getVoucherFile(BASE, { id: "v1", fileContentType: "image/png" })).rejects.toThrow("comprobante no encontrado");
  });
});

describe("rejectVoucher", () => {
  it("posts the reason and returns the voucher as it now stands", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({ voucher: { ...wireVoucher, status: "rejected", rejectionReason: "No se lee" }, replayed: false }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const v = await rejectVoucher(BASE, "v1", "No se lee");

    const [url, init] = firstCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/finance/payment-vouchers/v1/reject");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body as string)).toEqual({ reason: "No se lee" });
    expect(v.status).toBe("rejected");
  });

  it("keeps the machine code of a refusal", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(
      new Response("este comprobante ya se aplicó al pago P-2026-000042", { status: 409, headers: { "X-Error-Code": "VOUCHER_ALREADY_APPLIED" } }),
    ));

    const err = await rejectVoucher(BASE, "v1", "No se lee").catch((e: unknown) => e);

    expect(err).toBeInstanceOf(FinanceError);
    expect((err as FinanceError).errorCode).toBe(VOUCHER_CODES.alreadyApplied);
    expect((err as FinanceError).status).toBe(409);
  });
});

describe("registerDeposit with a voucher", () => {
  it("sends the voucher and no idempotency key: the server derives it", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ deposit, allocations: [], surcharges: [], unallocatedCents: 0, replayed: false }, 201));
    vi.stubGlobal("fetch", fetchMock);

    await registerDeposit(BASE, { studentId: "s1", amountCents: 320000, method: "transfer", reference: "", idempotencyKey: "k-1", voucherId: "v1" });

    const body = JSON.parse(firstCall(fetchMock)[1].body as string) as Record<string, unknown>;
    expect(body.voucherId).toBe("v1");
    expect("idempotencyKey" in body).toBe(false);
    expect("declinedVoucherIds" in body).toBe(false);
  });

  it("sends the vouchers a hand-registered payment is not", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ deposit, allocations: [], surcharges: [], unallocatedCents: 0, replayed: false }, 201));
    vi.stubGlobal("fetch", fetchMock);

    await registerDeposit(BASE, { studentId: "s1", amountCents: 1, method: "cash", reference: "", idempotencyKey: "k-1", declinedVoucherIds: ["v1"] });

    const body = JSON.parse(firstCall(fetchMock)[1].body as string) as Record<string, unknown>;
    expect(body.declinedVoucherIds).toEqual(["v1"]);
    expect(body.idempotencyKey).toBe("k-1");
  });

  it("sends neither field when there is nothing to say, so a gateway with the flag off still answers", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ deposit, allocations: [], surcharges: [], unallocatedCents: 0, replayed: false }, 201));
    vi.stubGlobal("fetch", fetchMock);

    await registerDeposit(BASE, { studentId: "s1", amountCents: 1, method: "cash", reference: "", declinedVoucherIds: [] });

    const body = JSON.parse(firstCall(fetchMock)[1].body as string) as Record<string, unknown>;
    expect("voucherId" in body).toBe(false);
    expect("declinedVoucherIds" in body).toBe(false);
  });

  it("returns the voucher the deposit paid", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({
      deposit, allocations: [], surcharges: [], unallocatedCents: 0, replayed: false,
      linkedVoucher: { ...wireVoucher, status: "applied", depositId: "dep-1" },
    }, 201)));

    const res = await registerDeposit(BASE, { studentId: "s1", amountCents: 1, method: "cash", reference: "" });

    expect(res.linkedVoucher?.id).toBe("v1");
    expect(res.linkedVoucher?.status).toBe("applied");
  });

  it("carries the voucher code of a 409 so the counter can tell it from CC-202", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(
      new Response("elige cuál", { status: 409, headers: { "X-Error-Code": "VOUCHER_CHOICE_REQUIRED" } }),
    ));

    const err = await registerDeposit(BASE, { studentId: "s1", amountCents: 1, method: "cash", reference: "" }).catch((e: unknown) => e);

    expect((err as FinanceError).errorCode).toBe(VOUCHER_CODES.choiceRequired);
  });
});
