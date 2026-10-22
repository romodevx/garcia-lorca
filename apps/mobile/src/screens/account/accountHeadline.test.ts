import { accountHeadline } from "./accountHeadline";

const totals = (balanceCents: number) => ({ chargedCents: 0, discountCents: 0, paidCents: 0, balanceCents, notYetDueCents: 0 });

describe("accountHeadline", () => {
  it("leads with what is due today when the school's day is known", () => {
    expect(accountHeadline({ totals: totals(2800000), dueTodayCents: 350000, asOf: "2026-10-21" })).toEqual({
      label: "Para estar al corriente",
      amountCents: 350000,
      settled: false,
      hasToday: true,
      balanceCents: 2800000,
    });
  });

  it("is settled when nothing is due today, whatever the cycle still owes", () => {
    const h = accountHeadline({ totals: totals(2800000), dueTodayCents: 0, asOf: "2026-10-21" });
    expect(h.settled).toBe(true);
    expect(h.amountCents).toBe(0);
  });

  // bug_005: an unknown day must not read as a fabricated "$0.00 · Al corriente".
  it("falls back to the balance when the day is unknown", () => {
    expect(accountHeadline({ totals: totals(2800000), dueTodayCents: 0, asOf: "" })).toMatchObject({
      label: "Saldo por pagar",
      amountCents: 2800000,
      settled: false,
      hasToday: false,
    });
  });

  it("names a negative balance as credit, as a positive amount, when the day is unknown", () => {
    expect(accountHeadline({ totals: totals(-50000), dueTodayCents: 0, asOf: "" })).toMatchObject({
      label: "Saldo a favor",
      amountCents: 50000,
      settled: true,
    });
  });

  it("calls a zero balance the current balance when the day is unknown", () => {
    expect(accountHeadline({ totals: totals(0), dueTodayCents: 0, asOf: "" }).label).toBe("Saldo actual");
  });
});
