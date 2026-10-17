import { describe, expect, it } from "vitest";

import type { CashUpPayment, Deposit } from "../../finance/api";
import { cashUpTimeline } from "./cashUpTimeline";

const deposit = (id: string, paidAt: string, folio?: string): Deposit => ({
  id, studentId: "s1", amountCents: 1000, method: "cash", reference: "", paidAt, calendarVersion: "v",
  tenders: [{ method: "cash", amountCents: 1000 }], ...(folio !== undefined ? { folio } : {}),
});
const loose = (id: string, paidAt: string, description: string, reference = ""): CashUpPayment => ({
  id, chargeId: "c1", amountCents: 1000, method: "cash", reference, paidAt, description,
});
const keys = (deposits: Deposit[], payments: CashUpPayment[]): string[] => cashUpTimeline(deposits, payments).map((e) => e.key);

const AT = "2026-10-10T15:00:00Z";

describe("cashUpTimeline", () => {
  it("breaks a tie between counter sales by the U- folio they print, not by id", () => {
    expect(keys([], [loose("a", AT, "Uniformes · U-2026-000009"), loose("z", AT, "Uniformes · U-2026-000002")])).toEqual(["p-z", "p-a"]);
  });

  it("breaks a tie on a deposit with an empty folio by its id", () => {
    expect(keys([deposit("b", AT, ""), deposit("a", AT, "")], [])).toEqual(["d-a", "d-b"]);
  });

  // A back-dated deposit is dated the day the family paid and carries no hour
  // (paidAt at midnight): it opens that day's list, before anything keyed live.
  it("puts a back-dated deposit, dated at midnight, first", () => {
    expect(keys([deposit("live", "2026-10-10T14:35:00Z", "P-2026-000002"), deposit("late", "2026-10-10T00:00:00Z", "P-2026-000009")], []))
      .toEqual(["d-late", "d-live"]);
  });
});
