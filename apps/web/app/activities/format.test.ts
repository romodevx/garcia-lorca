import { describe, expect, it } from "vitest";

import { optionLabel, optionsSummary, PERIODICITY_LABEL, scheduleSummary } from "./format";

describe("scheduleSummary", () => {
  it("joins the days that share hours", () => {
    expect(scheduleSummary([
      { weekday: 4, start: "15:00", end: "16:30" },
      { weekday: 2, start: "15:00", end: "16:30" },
    ])).toBe("Mar y Jue 15:00–16:30");
  });

  it("lists days with different hours apart, in week order", () => {
    expect(scheduleSummary([
      { weekday: 6, start: "09:00", end: "11:00" },
      { weekday: 1, start: "15:00", end: "16:00" },
      { weekday: 3, start: "15:00", end: "16:00" },
      { weekday: 5, start: "15:00", end: "16:00" },
    ])).toBe("Lun, Mié y Vie 15:00–16:00 · Sáb 09:00–11:00");
  });

  it("says so when there is none", () => {
    expect(scheduleSummary([])).toBe("Sin horario");
  });
});

describe("periodicity and options", () => {
  it("names each periodicity in Spanish", () => {
    expect(PERIODICITY_LABEL).toEqual({ monthly: "Mensual", weekly: "Semanal", per_cycle: "Una vez por ciclo" });
  });

  it("labels an option by days a week or as the flat fee", () => {
    expect(optionLabel({ daysPerWeek: 1, amountCents: 60000 })).toBe("1 día/semana — $600.00");
    expect(optionLabel({ daysPerWeek: 3, amountCents: 80000 })).toBe("3 días/semana — $800.00");
    expect(optionLabel({ daysPerWeek: null, amountCents: 90000 })).toBe("Cuota fija — $900.00");
  });

  it("summarises the options, fewest days first and the flat fee last", () => {
    expect(optionsSummary([
      { daysPerWeek: null, amountCents: 90000 },
      { daysPerWeek: 2, amountCents: 70000 },
      { daysPerWeek: 1, amountCents: 60000 },
    ])).toBe("1 día $600.00 · 2 días $700.00 · Fija $900.00");
  });
});
