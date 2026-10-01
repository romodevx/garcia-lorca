import { describe, expect, it } from "vitest";

import type { Payment } from "./api";
import { groupByCharge } from "./allocations";

const pay = (id: string, chargeId: string, cents: number, method: Payment["method"] = "cash"): Payment => ({
  id, chargeId, amountCents: cents, method, reference: "", paidAt: "",
});

describe("groupByCharge (tarea 8)", () => {
  // A deposit paid with two methods splits its allocations per tender, so the
  // same month can come back twice. A person reads it once, with its total.
  it("adds up what each charge received, in the order the charges first appear", () => {
    const grouped = groupByCharge([
      pay("p1", "ch-sep", 50000, "credit_note"),
      pay("p2", "ch-sep", 300000),
      pay("p3", "ch-oct", 320000),
    ]);

    expect(grouped).toEqual([
      { chargeId: "ch-sep", amountCents: 350000 },
      { chargeId: "ch-oct", amountCents: 320000 },
    ]);
  });

  it("is empty for nothing", () => {
    expect(groupByCharge([])).toEqual([]);
  });
});
