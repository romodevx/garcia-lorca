import { describe, expect, it } from "vitest";

import type { UniformFulfillment, UniformOrder } from "../../uniforms/api";
import { toRows } from "./OrdersPanel";

const sale = (f: Partial<UniformFulfillment>): UniformOrder => ({
  id: "o1", folio: "U-2026-000002", studentName: "Alumno Prueba", source: "counter", label: "closed", totalCents: 25000, lines: [],
  fulfillments: [{ id: "f1", seq: 1, folio: "U-2026-000002", status: "sold_at_counter", amountCents: 25000, deadlineEstimated: false, requiresAttention: false, ...f }],
});

// C2-015: a counter sale can be returned; once returned it says so, still
// reprints (the ticket reads CANCELADO) and cannot be returned again.
describe("toRows · devoluciones", () => {
  it("offers the return on a counter sale", () => {
    const [row] = toRows([sale({})]);

    expect(row?.returnOrderId).toBe("o1");
    expect(row?.receiptOrderId).toBe("o1");
  });

  it("reads a returned sale as returned, reprintable, not returnable again", () => {
    const [row] = toRows([sale({ status: "returned", return: { reason: "Talla", refundMode: "void", refundCents: 25000, refundStatus: "done", damagedUnits: 0 } })]);

    expect(row?.statusLabel).toBe("Devuelta");
    expect(row?.receiptOrderId).toBe("o1");
    expect(row?.returnOrderId).toBeUndefined();
  });

  it("says when the refund is still to be recorded", () => {
    const [row] = toRows([sale({ status: "returned", requiresAttention: true, return: { reason: "Talla", refundMode: "expense", refundMethod: "cash", refundCents: 25000, refundStatus: "pending", damagedUnits: 0 } })]);

    expect(row?.statusLabel).toBe("Devuelta · reembolso pendiente");
    expect(row?.returnOrderId).toBe("o1");
  });
});
