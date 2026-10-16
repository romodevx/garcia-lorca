import { describe, expect, it } from "vitest";

import { closePreview, dateRangeLabel, dayLabel, publishedMessage } from "./format";

describe("dayLabel", () => {
  it("reads a date on the school's calendar", () => {
    expect(dayLabel("2026-12-01T18:00:00Z")).toMatch(/1 dic.* 2026/);
  });

  it.each([null, undefined, ""])("says — when there is no date (%s)", (iso) => {
    expect(dayLabel(iso)).toBe("—");
  });
});

describe("dateRangeLabel", () => {
  it("names both days of the period, as calendar days", () => {
    expect(dateRangeLabel("2026-08-24", "2026-11-30")).toBe("24 ago 2026 – 30 nov 2026");
  });

  it("is undefined when the period has no dates", () => {
    expect(dateRangeLabel(null, null)).toBeUndefined();
    expect(dateRangeLabel("2026-08-24", null)).toBeUndefined();
  });
});

describe("closePreview — what closing will publish, said before closing (T10)", () => {
  it.each([
    [24, 1, "Se publicarán 24 boletas. 1 alumno sin calificaciones no recibirá boleta."],
    [30, 3, "Se publicarán 30 boletas. 3 alumnos sin calificaciones no recibirán boleta."],
    [1, 0, "Se publicará 1 boleta."],
    [0, 2, "No se publicará ninguna boleta. 2 alumnos sin calificaciones no recibirán boleta."],
  ])("%i boletas, %i without grades", (reportCards, skipped, text) => {
    expect(closePreview(reportCards, skipped)).toBe(text);
  });
});

describe("publishedMessage — what closing did", () => {
  const result = (reportCards: number, skippedWithoutGrades: number) => ({ status: "closed" as const, version: 1, reportCards, skippedWithoutGrades });

  it.each([
    [124, 0, "Se publicaron 124 boletas"],
    [1, 0, "Se publicó 1 boleta"],
    [1, 1, "Se publicó 1 boleta. 1 alumno sin calificaciones no recibió boleta."],
    [24, 3, "Se publicaron 24 boletas. 3 alumnos sin calificaciones no recibieron boleta."],
    [0, 2, "No se publicó ninguna boleta. 2 alumnos sin calificaciones no recibieron boleta."],
  ])("%i boletas, %i without grades", (reportCards, skipped, text) => {
    expect(publishedMessage(result(reportCards, skipped))).toBe(text);
  });
});
