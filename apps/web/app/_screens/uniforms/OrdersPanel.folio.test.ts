import { describe, expect, it } from "vitest";

import type { UniformFulfillment, UniformOrder } from "../../uniforms/api";
import { toRows } from "./OrdersPanel";

const delivery = (seq: number, status: UniformFulfillment["status"]): UniformFulfillment => ({
  id: `f${seq}`, seq, folio: `U-2026-000001-${String.fromCharCode(64 + seq)}`, status, amountCents: 32000,
  deadlineEstimated: false, requiresAttention: false,
});
const order = (source: string, fulfillments: UniformFulfillment[]): UniformOrder => ({
  id: "o1", folio: "U-2026-000001", studentName: "Alumno Prueba", source, label: "closed", totalCents: 32000, lines: [], fulfillments,
});

// ISSUE-T14-019: Pedidos said "U-2026-000001-A" for a counter sale whose
// receipt — and whose Reimprimir — say "U-2026-000001". The letter names one
// delivery of an order handed over in parts; a counter sale is one act.
describe("toRows · folio", () => {
  it("shows a counter sale under the folio its receipt prints", () => {
    const [row] = toRows([order("counter", [delivery(1, "sold_at_counter")])]);

    expect(row?.folioStem).toBe("U-2026-000001");
    expect(row?.folioSuffix).toBe("");
  });

  it("keeps the delivery's letter on an order handed over in parts", () => {
    const rows = toRows([order("family", [delivery(1, "dispatched"), delivery(2, "ready_for_pickup")])]);

    expect(rows.map((r) => r.folioSuffix)).toEqual(["-A", "-B"]);
  });
});
