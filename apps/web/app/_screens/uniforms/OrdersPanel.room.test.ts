import { describe, expect, it } from "vitest";

import type { UniformOrder } from "../../uniforms/api";
import { toRows } from "./OrdersPanel";

const order: UniformOrder = {
  id: "o1", folio: "U-2026-000002", studentId: "s1", studentName: "Alumna Prueba", studentLevel: "PRIMARIA", studentGradeGroup: "B",
  source: "counter", label: "closed", totalCents: 25000, lines: [],
  fulfillments: [{ id: "f1", seq: 1, folio: "U-2026-000002-A", status: "sold_at_counter", amountCents: 25000, deadlineEstimated: false, requiresAttention: false }],
};

// ISSUE-T14-204: the Nivel column said "PRIMARIA" — the order stores only the
// level and the letter, while the receipt said "3º PRIMARIA · B".
describe("toRows · Nivel", () => {
  it("names the pupil's classroom with its grade when the cycle knows it", () => {
    const [row] = toRows([order], new Map([["s1", "3º PRIMARIA · B"]]));

    expect(row?.levelLabel).toBe("3º PRIMARIA · B");
  });

  it("falls back to the level the order stored", () => {
    expect(toRows([order])[0]?.levelLabel).toBe("PRIMARIA");
  });
});
