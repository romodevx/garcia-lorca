import { describe, expect, it } from "vitest";

import { GradeRequestError } from "./requestError";
import {
  gradeKeySearch,
  isRejectedValue,
  parseGradeInput,
  periodChips,
  refusalScope,
  sheetSearch,
  sheetStats,
  validateGrade,
  type EvaluationPeriodRef,
  type GradeScale,
} from "./periodSheet";

const SCALE: GradeScale = { min: 5, max: 10, decimals: 1, passing: 6 };

describe("parseGradeInput", () => {
  it("reads a whole number, trimming around it", () => {
    expect(parseGradeInput(" 10 ")).toEqual({ kind: "value", value: 10, decimals: 0 });
  });

  it("reads a decimal comma and point alike, so the decimals can be refused", () => {
    expect(parseGradeInput("9,5")).toEqual({ kind: "value", value: 9.5, decimals: 1 });
    expect(parseGradeInput("9.5")).toEqual({ kind: "value", value: 9.5, decimals: 1 });
  });

  it("reads an empty field as a cleared grade", () => {
    expect(parseGradeInput("   ")).toEqual({ kind: "empty" });
  });

  it("rejects what is not a number: plus signs, exponents, hex, inner spaces", () => {
    for (const raw of ["9..5", "abc", "+3", "- 3", "1e1", "0x9", "9 5"]) expect(parseGradeInput(raw)).toEqual({ kind: "invalid" });
  });

  it("reads a negative number as a value, so the range check can name the scale (CC-307)", () => {
    expect(parseGradeInput("-3")).toEqual({ kind: "value", value: -3, decimals: 0 });
  });

  it("counts decimals on the exact value, ignoring trailing zeros", () => {
    expect(parseGradeInput("9.0")).toEqual({ kind: "value", value: 9, decimals: 0 });
  });
});

describe("validateGrade — integers only (T15)", () => {
  it("accepts a whole number inside the scale, and an empty field", () => {
    expect(validateGrade("9", SCALE)).toBeUndefined();
    expect(validateGrade("", SCALE)).toBeUndefined();
  });

  it("refuses any decimal, whatever the level's scale says — never rounds it", () => {
    expect(validateGrade("9.5", SCALE)).toBe("Solo se permiten calificaciones enteras");
    expect(validateGrade("8,7", { ...SCALE, decimals: 2 })).toBe("Solo se permiten calificaciones enteras");
  });

  it("names the range when out of it", () => {
    expect(validateGrade("11", SCALE)).toBe("La calificación debe estar entre 5 y 10");
    expect(validateGrade("4", SCALE)).toBe("La calificación debe estar entre 5 y 10");
  });

  it("names the range for a negative grade, like the server does (CC-307)", () => {
    expect(validateGrade("-3", SCALE)).toBe("La calificación debe estar entre 5 y 10");
  });

  it("explains a non-number, in the server's words", () => {
    expect(validateGrade("x", SCALE)).toBe("La calificación debe ser un número");
  });
});

describe("sheetStats", () => {
  it("counts captured and failing, and averages the exact mean truncated to 2 decimals", () => {
    expect(sheetStats([9, null, 9, 8], SCALE.passing)).toEqual({ captured: 3, total: 4, average: 8.66, failing: 0 });
    expect(sheetStats([5, 10], SCALE.passing)).toEqual({ captured: 2, total: 2, average: 7.5, failing: 1 });
  });

  it("has no average when nothing is captured", () => {
    expect(sheetStats([null, null], 6)).toEqual({ captured: 0, total: 2, average: undefined, failing: 0 });
  });
});

describe("query strings", () => {
  it("leaves the period to the server when none is chosen", () => {
    expect(sheetSearch({ assignmentId: "a1" })).toBe("?assignmentId=a1");
    expect(sheetSearch({ assignmentId: "a1", periodId: "p2" })).toBe("?assignmentId=a1&periodId=p2");
  });

  it("names a cell by assignment, period and enrollment", () => {
    expect(gradeKeySearch({ assignmentId: "a1", periodId: "p1", enrollmentId: "en1" })).toBe("?assignmentId=a1&periodId=p1&enrollmentId=en1");
  });
});

describe("refusalScope", () => {
  it.each([
    ["GRADE_OUT_OF_RANGE", "row"],
    ["GRADE_NOT_INTEGER", "row"],
    ["GRADE_NOT_NUMBER", "row"],
    ["PERIOD_NOT_IN_GRADE", "row"],
    ["ENROLLMENT_NOT_IN_GROUP", "row"],
    ["SOMETHING_NEW", "row"],
    [undefined, "row"],
    ["PERIOD_PREVIOUS_GROUP", "rowLocked"],
    ["LEVEL_NOT_GRADED", "sheet"],
    ["LEVEL_SCALE_NOT_CONFIGURED", "sheet"],
    ["PERIOD_NOT_STARTED", "sheet"],
    ["PERIOD_LOCKED", "sheet"],
  ] as const)("%s belongs to the %s", (code, scope) => {
    expect(refusalScope(code)).toBe(scope);
  });
});

describe("isRejectedValue", () => {
  it("a 400 with words, or a PERIOD_LOCKED whatever its status, is final: resending never helps", () => {
    expect(isRejectedValue(new GradeRequestError(400, "Solo se permiten calificaciones enteras", "GRADE_NOT_INTEGER"))).toBe(true);
    expect(isRejectedValue(new GradeRequestError(409, "El periodo está cerrado", "PERIOD_LOCKED"))).toBe(true);
    expect(isRejectedValue(new GradeRequestError(409, "Conflicto sin código"))).toBe(false);
    expect(isRejectedValue(new Error("network"))).toBe(false);
  });
});

describe("periodChips", () => {
  const P1: EvaluationPeriodRef = { id: "p1", ordinal: 1, name: "1.er trimestre", startsOn: "2026-08-24", endsOn: "2026-11-30" };
  const P2: EvaluationPeriodRef = { id: "p2", ordinal: 2, name: "2.º trimestre", startsOn: "2026-12-07", endsOn: "2027-03-26" };
  const P3: EvaluationPeriodRef = { id: "p3", ordinal: 3, name: "3.er trimestre", startsOn: "2027-04-12", endsOn: "2027-07-09" };

  it("names each period with its short date range", () => {
    const [chip] = periodChips({ periods: [P1], today: "2026-10-01", closedPeriodIds: [], currentPeriodId: "p1" });
    expect(chip).toEqual({ id: "p1", label: "1.er trimestre", range: "24 ago – 30 nov", notStarted: false, closed: false, current: true });
  });

  it("marks the periods that start after today, the closed ones and the current one", () => {
    const chips = periodChips({ periods: [P1, P2, P3], today: "2026-12-07", closedPeriodIds: ["p1"], currentPeriodId: "p2" });
    expect(chips.map((c) => [c.id, c.closed, c.notStarted, c.current])).toEqual([
      ["p1", true, false, false],
      ["p2", false, false, true],
      ["p3", false, true, false],
    ]);
  });
});
