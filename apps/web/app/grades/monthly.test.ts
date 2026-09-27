import { describe, expect, it } from "vitest";

import { gradeFailure, GradeRequestError, monthChips, parseGradeInput, sheetStats, validateGrade, type GradeScale } from "./monthly";

const SCALE: GradeScale = { min: 5, max: 10, decimals: 1, passing: 6 };

describe("parseGradeInput", () => {
  it("reads a decimal comma and a decimal point alike", () => {
    expect(parseGradeInput("9,5")).toEqual({ kind: "value", value: 9.5, decimals: 1 });
    expect(parseGradeInput(" 9.5 ")).toEqual({ kind: "value", value: 9.5, decimals: 1 });
    expect(parseGradeInput("10")).toEqual({ kind: "value", value: 10, decimals: 0 });
  });

  it("reads an empty field as a cleared grade", () => {
    expect(parseGradeInput("   ")).toEqual({ kind: "empty" });
  });

  it("rejects what is not a number", () => {
    expect(parseGradeInput("9..5")).toEqual({ kind: "invalid" });
    expect(parseGradeInput("abc")).toEqual({ kind: "invalid" });
    expect(parseGradeInput("-3")).toEqual({ kind: "invalid" });
  });

  it("rejects exponents, hex and spaces inside the number, but trims around it", () => {
    expect(parseGradeInput("1e1")).toEqual({ kind: "invalid" });
    expect(parseGradeInput("0x9")).toEqual({ kind: "invalid" });
    expect(parseGradeInput("9 5")).toEqual({ kind: "invalid" });
    expect(parseGradeInput("9, 5")).toEqual({ kind: "invalid" });
    expect(parseGradeInput("\t9,5 ")).toEqual({ kind: "value", value: 9.5, decimals: 1 });
  });

  it("counts decimals on the exact value, ignoring trailing zeros", () => {
    expect(parseGradeInput("10.0")).toEqual({ kind: "value", value: 10, decimals: 0 });
    expect(parseGradeInput("9,0")).toEqual({ kind: "value", value: 9, decimals: 0 });
    expect(parseGradeInput("9.50")).toEqual({ kind: "value", value: 9.5, decimals: 1 });
  });
});

describe("validateGrade", () => {
  it("accepts a value inside the scale", () => {
    expect(validateGrade("9,5", SCALE)).toBeUndefined();
    expect(validateGrade("", SCALE)).toBeUndefined();
  });

  it("names the range when out of it", () => {
    expect(validateGrade("11", SCALE)).toBe("La calificación debe estar entre 5 y 10");
    expect(validateGrade("4", SCALE)).toBe("La calificación debe estar entre 5 y 10");
  });

  it("names the allowed decimals", () => {
    expect(validateGrade("9.55", SCALE)).toBe("Solo se permite 1 decimal");
    expect(validateGrade("9.5", { ...SCALE, decimals: 0 })).toBe("Solo se permiten calificaciones enteras");
    expect(validateGrade("9.555", { ...SCALE, decimals: 2 })).toBe("Solo se permiten 2 decimales");
  });

  it("explains a non-number, in the server's words", () => {
    expect(validateGrade("x", SCALE)).toBe("La calificación debe ser un número");
    expect(validateGrade("1e1", SCALE)).toBe("La calificación debe ser un número");
  });

  it("accepts trailing zeros the scale allows", () => {
    expect(validateGrade("10.0", { ...SCALE, decimals: 0 })).toBeUndefined();
    expect(validateGrade("9,0", { ...SCALE, decimals: 0 })).toBeUndefined();
    expect(validateGrade("9.50", SCALE)).toBeUndefined();
  });
});

describe("sheetStats", () => {
  it("counts captured, averages them and counts those below passing", () => {
    const stats = sheetStats([9, null, 5.5, 8], SCALE.passing);
    expect(stats).toEqual({ captured: 3, total: 4, average: 7.5, failing: 1 });
  });

  it("rounds the average half-up to one decimal", () => {
    expect(sheetStats([8.4, 8.5], 6).average).toBe(8.5);
    expect(sheetStats([8, 8.5], 6).average).toBe(8.3);
  });

  it("has no average when nothing is captured", () => {
    expect(sheetStats([null, null], 6)).toEqual({ captured: 0, total: 2, average: undefined, failing: 0 });
  });
});

describe("monthChips", () => {
  it("names months short, in Spanish", () => {
    expect(monthChips(["2026-09", "2026-10"]).map((c) => c.label)).toEqual(["Sep", "Oct"]);
  });

  it("adds the year only when a short name repeats", () => {
    expect(monthChips(["2025-09", "2026-06", "2026-09"]).map((c) => c.label)).toEqual(["Sep 2025", "Jun", "Sep 2026"]);
  });

  it("gives every chip its full name", () => {
    expect(monthChips(["2026-01"])[0]).toEqual({ month: "2026-01", label: "Ene", fullName: "enero de 2026" });
  });
});

describe("gradeFailure", () => {
  it("keeps the server's own words and status", async () => {
    const err = await gradeFailure(new Response("Solo se permite 1 decimal\n", { status: 400 }));
    expect(err).toBeInstanceOf(GradeRequestError);
    expect(err.status).toBe(400);
    expect(err.message).toBe("Solo se permite 1 decimal");
  });

  it("falls back to the status when the body is empty", async () => {
    const err = await gradeFailure(new Response("", { status: 503 }));
    expect(err.message).toBe("HTTP 503");
    expect(err.detail).toBe("");
  });
});
