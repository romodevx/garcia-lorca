import { describe, expect, it } from "vitest";

import type { Cycle } from "../../config/api";
import type { TuitionPlan } from "../../finance/api";
import { copyPlanToCycle, shiftOneYear } from "./tuition-copy";

const TARGET: Cycle = {
  id: "c2",
  name: "2026-2027",
  status: "draft",
  startsOn: "2026-08-01",
  endsOn: "2027-07-31",
};

const PREVIOUS: TuitionPlan = {
  id: "p1",
  cycleId: "c1",
  levelId: "l1",
  enrollmentCents: 0,
  monthlyCents: 280000,
  months: 10,
  firstBilledMonth: 9,
  dueDay: 10,
  lateFeeCents: 20000,
  active: true,
  policy: {
    dailyMode: false,
    lateFeeDailyCents: 0,
    surchargeCountsAllDays: false,
    earlyPaymentCents: 10000,
    earlyPaymentDays: 5,
  },
  schedule: [],
  monthlyDiscountCents: 0,
  fees: [{ name: "CUOTA DE JULIO", amountCents: 160000, months: [9, 10, 11] }],
  enrollmentDueOn: "2025-06-30",
  enrollmentConcepts: [
    {
      name: "INSCRIPCIÓN",
      tiers: [{ effectiveFrom: "2025-08-31", effectiveTo: "", amountCents: 450000 }],
    },
  ],
};

describe("shiftOneYear", () => {
  it("mueve la misma fecha un año", () => {
    expect(shiftOneYear("2025-08-31")).toBe("2026-08-31");
  });

  it("devuelve vacío para lo vacío", () => {
    expect(shiftOneYear("")).toBe("");
  });

  /**
   * NO se valida contra el rango del ciclo, y esta prueba está aquí para que
   * nadie lo "arregle" otra vez: la fecha límite de inscripción cae fuera del
   * ciclo POR DISEÑO — se cobra en junio para un año que abre en agosto. Una
   * comprobación de rango la borraría en silencio en cada copia.
   */
  it("respeta la fecha que cae fuera del ciclo, porque la inscripción es así", () => {
    expect(shiftOneYear("2025-06-30")).toBe("2026-06-30");
  });
});

describe("copyPlanToCycle", () => {
  it("apunta al ciclo nuevo, no al viejo", () => {
    expect(copyPlanToCycle(PREVIOUS, TARGET).cycleId).toBe("c2");
  });

  /**
   * Los montos se copian TAL CUAL, a propósito. Subirlos es la decisión que la
   * directora vino a tomar; adivinarla sería cobrarle a las familias un número
   * que nadie escribió.
   */
  it("deja los montos intactos", () => {
    const copy = copyPlanToCycle(PREVIOUS, TARGET);

    expect(copy.monthlyCents).toBe(280000);
    expect(copy.enrollmentConcepts[0]?.tiers[0]?.amountCents).toBe(450000);
    expect(copy.fees[0]?.amountCents).toBe(160000);
    expect(copy.lateFeeCents).toBe(20000);
  });

  it("conserva la estructura: mensualidades, día límite, política y cuotas", () => {
    const copy = copyPlanToCycle(PREVIOUS, TARGET);

    expect(copy.months).toBe(10);
    expect(copy.firstBilledMonth).toBe(9);
    expect(copy.dueDay).toBe(10);
    expect(copy.policy.earlyPaymentDays).toBe(5);
    expect(copy.fees[0]?.months).toEqual([9, 10, 11]);
  });

  it("recorre las fechas exactamente un año", () => {
    const copy = copyPlanToCycle(PREVIOUS, TARGET);

    expect(copy.enrollmentConcepts[0]?.tiers[0]?.effectiveFrom).toBe("2026-08-31");
    expect(copy.enrollmentDueOn).toBe("2026-06-30");
  });

  // El identificador del plan viejo no puede viajar: sería pedirle al servidor
  // que sobrescriba el tabulador del año pasado.
  it("no se lleva el identificador del plan anterior", () => {
    expect(copyPlanToCycle(PREVIOUS, TARGET).id).toBe("");
  });

  it("no toca el plan original", () => {
    copyPlanToCycle(PREVIOUS, TARGET);

    expect(PREVIOUS.cycleId).toBe("c1");
    expect(PREVIOUS.enrollmentConcepts[0]?.tiers[0]?.effectiveFrom).toBe("2025-08-31");
  });
});
