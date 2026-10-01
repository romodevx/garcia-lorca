import { describe, expect, it } from "vitest";

import {
  AMOUNT_ERROR,
  MAX_TENDERS,
  SUM_ERROR,
  coveredCents,
  nextMethod,
  remainingLabel,
  tenderErrors,
  tenderLabel,
  tenderSummary,
  toTenders,
  type TenderRow,
} from "./tenders";

const row = (key: string, method: TenderRow["method"], amount: string, extra: Partial<TenderRow> = {}): TenderRow => ({
  key, method, cardType: "debit", amount, reference: "", ...extra,
});

describe("tenders · the arithmetic of a split payment", () => {
  it("covers what the readable amounts add up to", () => {
    expect(coveredCents([row("a", "credit_note", "500"), row("b", "cash", "1,000.00")])).toBe(150000);
    // An unreadable amount covers nothing — it is an error, not a zero.
    expect(coveredCents([row("a", "cash", "abc"), row("b", "cash", "10")])).toBe(1000);
  });

  it("says what is left, and by how much it goes over", () => {
    expect(remainingLabel(150000, 100000)).toBe("$500.00");
    expect(remainingLabel(150000, 150000)).toBe("$0.00");
    expect(remainingLabel(150000, 160000)).toBe("excede por $100.00");
  });

  it("offers at most four", () => {
    expect(MAX_TENDERS).toBe(4);
  });

  it("suggests the next method nobody has used yet, cash first", () => {
    expect(nextMethod([row("a", "credit_note", "500")])).toBe("cash");
    expect(nextMethod([row("a", "cash", "500")])).toBe("card");
    expect(nextMethod([row("a", "cash", "1"), row("b", "card", "1"), row("c", "transfer", "1")])).toBe("credit_note");
  });
});

describe("tenderErrors · what blocks the submit", () => {
  it("accepts a credit note that covers the whole amount", () => {
    expect(tenderErrors([row("a", "credit_note", "1500")], 150000)).toEqual({});
  });

  it("accepts a credit note completed with cash", () => {
    expect(tenderErrors([row("a", "credit_note", "500"), row("b", "cash", "1000")], 150000)).toEqual({});
  });

  it("marks every amount that is not above zero", () => {
    expect(tenderErrors([row("a", "credit_note", "0"), row("b", "cash", "")], 150000)).toEqual({ a: AMOUNT_ERROR, b: AMOUNT_ERROR });
  });

  it("marks the last amount when the sum does not match the amount to pay", () => {
    expect(tenderErrors([row("a", "credit_note", "500"), row("b", "cash", "900")], 150000)).toEqual({ b: SUM_ERROR });
    expect(SUM_ERROR).toBe("La suma de las formas de pago debe ser igual al monto a pagar.");
  });
});

describe("toTenders · what travels", () => {
  it("sends cents, a trimmed reference only when there is one, and the card type only on a card", () => {
    expect(toTenders([
      row("a", "credit_note", "500", { reference: "  NC-12 " }),
      row("b", "card", "1,000.00", { cardType: "credit" }),
      row("c", "cash", "10", { cardType: "credit" }),
    ])).toEqual([
      { method: "credit_note", amountCents: 50000, reference: "NC-12" },
      { method: "card", amountCents: 100000, cardType: "credit" },
      { method: "cash", amountCents: 1000 },
    ]);
  });
});

describe("labels", () => {
  it("names each method as the office says it", () => {
    expect(tenderLabel("cash")).toBe("Efectivo");
    expect(tenderLabel("card")).toBe("Tarjeta");
    expect(tenderLabel("card", "debit")).toBe("Tarjeta de débito");
    expect(tenderLabel("card", "credit")).toBe("Tarjeta de crédito");
    expect(tenderLabel("transfer")).toBe("Transferencia");
    expect(tenderLabel("credit_note")).toBe("Nota de crédito");
    expect(tenderLabel("mixed")).toBe("Combinado");
  });

  it("summarises a split payment method by method", () => {
    expect(tenderSummary([{ method: "credit_note", amountCents: 50000 }, { method: "cash", amountCents: 100000 }]))
      .toBe("Nota de crédito $500.00 + Efectivo $1,000.00");
    // One tender is just its method: the amount is already on the row.
    expect(tenderSummary([{ method: "transfer", amountCents: 100000 }])).toBe("Transferencia");
  });
});
