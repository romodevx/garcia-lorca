import { describe, expect, it } from "vitest";

import { DEFAULT_GRADING, fromDraft, gradingErrors, type GradingDraft } from "./gradingDraft";

const OK: GradingDraft = { min: "5", max: "10", passing: "6" };

describe("DEFAULT_GRADING", () => {
  it("grades a new level in whole numbers: 5 to 10, passing at 6, no decimals", () => {
    expect(DEFAULT_GRADING).toEqual({ enabled: true, min: 5, max: 10, decimals: 0, passing: 6 });
  });
});

describe("gradingErrors", () => {
  it("accepts a sound scale, with surrounding spaces", () => {
    expect(gradingErrors(OK)).toEqual({});
    expect(gradingErrors({ ...OK, passing: " 7 " })).toEqual({});
  });

  // ISSUE-T14-005: grades are whole numbers, so the scale's bounds are too.
  it.each(["1e1", "0x6", "6 5", "-1", "abc", "6.555", "6,5", "5.5"])("refuses %j: whole numbers only", (text) => {
    expect(gradingErrors({ ...OK, min: text }).min).toBe("Escribe un número entero, como 5 o 6.");
  });

  it("keeps the minimum at 0 or more and the maximum at 100 or less", () => {
    expect(gradingErrors({ ...OK, min: "0" })).toEqual({});
    expect(gradingErrors({ ...OK, max: "100" })).toEqual({});
    expect(gradingErrors({ ...OK, max: "101" }).max).toBe("La máxima no puede pasar de 100.");
  });

  it("still orders min < passing ≤ max", () => {
    expect(gradingErrors({ ...OK, max: "5" }).max).toBe("La máxima debe ser mayor que la mínima.");
    expect(gradingErrors({ ...OK, passing: "11" }).passing).toBe("El mínimo aprobatorio debe ser mayor que la mínima y no pasar de la máxima.");
  });
});

describe("fromDraft", () => {
  it("reads the typed text strictly, in whole numbers", () => {
    expect(fromDraft({ ...OK, passing: " 7 " }, true)).toEqual({ enabled: true, min: 5, max: 10, decimals: 0, passing: 7 });
    expect(fromDraft({ ...OK, passing: "6,5" }, true).passing).toBeNaN();
  });
});
