import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { useTenders } from "./useTenders";

// T9: correcting a payment opens with the methods it was recorded with.
describe("useTenders with the recorded tenders", () => {
  it("starts from them, in order, and keeps them as typed", () => {
    const { result } = renderHook(() => useTenders(100000, [
      { method: "credit_note", amountCents: 40000, reference: "NC-1" },
      { method: "card", amountCents: 60000, cardType: "credit" },
    ]));
    expect(result.current.rows.map((r) => [r.method, r.amount, r.reference])).toEqual([
      ["credit_note", "400.00", "NC-1"], ["card", "600.00", ""],
    ]);
    expect(result.current.rows[1]?.cardType).toBe("credit");
    expect(result.current.toTenders()).toEqual([
      { method: "credit_note", amountCents: 40000, reference: "NC-1" },
      { method: "card", amountCents: 60000, cardType: "credit" },
    ]);
  });

  it("one recorded method follows the corrected amount, keeping its method", () => {
    const { result, rerender } = renderHook(({ total }) => useTenders(total, [{ method: "transfer", amountCents: 50000 }]), { initialProps: { total: 50000 } });
    rerender({ total: 100000 });
    expect(result.current.rows).toHaveLength(1);
    expect(result.current.rows[0]?.method).toBe("transfer");
    expect(result.current.rows[0]?.amount).toBe("1,000.00");
    act(() => result.current.add());
    expect(result.current.rows).toHaveLength(2);
    expect(new Set(result.current.rows.map((r) => r.key)).size).toBe(2);
  });

  // Review of PR #144 (N6): a card recorded before the type was asked has no
  // type. Correcting it must not quietly call it debit: nothing is chosen, and
  // the correction asks for one (the server refuses a card without it, CC-205).
  it("opens a recorded card with no type unchosen, and asks for one", () => {
    const { result } = renderHook(() => useTenders(60000, [{ method: "card", amountCents: 60000 }]));
    expect(result.current.rows[0]?.cardType).toBe("");

    let ok = true;
    act(() => { ok = result.current.validate(); });
    expect(ok).toBe(false);
    expect(Object.values(result.current.errors)).toContain("Elige si la tarjeta es de débito o de crédito.");

    act(() => result.current.setCardType("t0", "debit"));
    act(() => { ok = result.current.validate(); });
    expect(ok).toBe(true);
    expect(result.current.toTenders()).toEqual([{ method: "card", amountCents: 60000, cardType: "debit" }]);
  });
});
