import { describe, expect, it } from "vitest";

import { depositRequest, newAttemptKey } from "./depositRequest";

describe("depositRequest", () => {
  it("carries the first tender for an old gateway, every tender, and the attempt's key", () => {
    const req = depositRequest({
      studentId: "s1", amountCents: 100000, attemptKey: "k-1", paidOn: "",
      tenders: [{ method: "card", amountCents: 60000, reference: "V-1", cardType: "debit" }, { method: "cash", amountCents: 40000 }],
    });

    expect(req).toEqual({
      studentId: "s1", amountCents: 100000, method: "card", reference: "V-1", cardType: "debit",
      tenders: [{ method: "card", amountCents: 60000, reference: "V-1", cardType: "debit" }, { method: "cash", amountCents: 40000 }],
      idempotencyKey: "k-1",
    });
  });

  it("sends the day, the chosen charges and the early-payment grant only when given", () => {
    const req = depositRequest({
      studentId: "s1", amountCents: 500, attemptKey: "k", paidOn: "2026-10-01",
      tenders: [{ method: "cash", amountCents: 500 }], chargeIds: ["ch1"], grantEarlyPayment: true,
    });

    expect(req).toMatchObject({ paidAt: "2026-10-01", chargeIds: ["ch1"], grantEarlyPayment: true, method: "cash", reference: "" });
  });
});

describe("depositRequest with vouchers (ADR 0040)", () => {
  it("pays BY a voucher without the attempt's key: the server derives it from the voucher", () => {
    const req = depositRequest({
      studentId: "s1", amountCents: 500, attemptKey: "k", paidOn: "", tenders: [{ method: "transfer", amountCents: 500 }], voucherId: "v1",
    });

    expect(req.voucherId).toBe("v1");
    expect("idempotencyKey" in req).toBe(false);
    expect("declinedVoucherIds" in req).toBe(false);
  });

  it("carries the vouchers a payment by hand is not, with its own key", () => {
    const req = depositRequest({
      studentId: "s1", amountCents: 500, attemptKey: "k", paidOn: "", tenders: [{ method: "cash", amountCents: 500 }], declinedVoucherIds: ["v1", "v2"],
    });

    expect(req).toMatchObject({ idempotencyKey: "k", declinedVoucherIds: ["v1", "v2"] });
    expect("voucherId" in req).toBe(false);
  });
});

describe("newAttemptKey", () => {
  it("is never empty and never repeats", () => {
    const a = newAttemptKey();
    expect(a).not.toBe("");
    expect(newAttemptKey()).not.toBe(a);
  });
});
