import type { FamilyMovement } from "../../api";
import { groupDeposits, splitLedger } from "./ledgerView";

const line = (date: string, description: string): FamilyMovement => ({
  kind: "charge",
  date,
  description,
  amountCents: 100,
  balanceCents: 100,
  accrued: false,
  notYetDue: false,
  voided: false,
});

const names = (lines: FamilyMovement[]): string[] => lines.map((m) => m.description);

describe("splitLedger", () => {
  it("splits YYYY-MM-DD days at the school's day: today counts as happened", () => {
    const view = splitLedger([line("2026-09-30", "sep"), line("2026-10-17", "hoy"), line("2026-10-18", "mañana")], "2026-10-17");

    expect(names(view.happened)).toEqual(["hoy", "sep"]);
    expect(names(view.upcoming)).toEqual(["mañana"]);
  });

  it("compares by calendar day when a line ever carries a time", () => {
    const view = splitLedger([line("2026-10-17T15:00:00Z", "hoy con hora"), line("2026-10-18T00:00:00-06:00", "mañana")], "2026-10-17");

    expect(names(view.happened)).toEqual(["hoy con hora"]);
    expect(names(view.upcoming)).toEqual(["mañana"]);
  });

  it("reads everything newest first when the day is unknown", () => {
    const view = splitLedger([line("2026-09-30", "sep"), line("2027-06-10", "jun")], "");

    expect(names(view.happened)).toEqual(["jun", "sep"]);
    expect(view.upcoming).toEqual([]);
  });
});

describe("groupDeposits (ISSUE-CC-315)", () => {
  const money = (kind: FamilyMovement["kind"], method: string, amountCents: number, depositId: string): FamilyMovement => ({
    ...line("2026-10-12", `${kind}-${method}`),
    kind,
    method,
    amountCents,
    depositId,
  });

  it("folds a deposit's tenders into one row where its first line was, keeping the rest", () => {
    const rows = groupDeposits([
      line("2026-10-01", "cargo"),
      money("credit_note", "credit_note", 50000, "d1"),
      money("payment", "cash", 20000, "d1"),
      money("payment", "card", 10000, "d2"),
    ]);

    expect(rows.map((r) => r.movement.description)).toEqual(["cargo", "payment-cash", "payment-card"]);
    // A credit note completed with cash still reads as the payment, for the total.
    expect(rows[1]?.movement.amountCents).toBe(70000);
    expect(rows[1]?.tenders).toEqual([
      { method: "credit_note", amountCents: 50000 },
      { method: "cash", amountCents: 20000 },
    ]);
    expect(rows[2]?.tenders).toEqual([]);
  });
});
