import type { FamilyMovement } from "../../api";
import { splitLedger } from "./ledgerView";

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
