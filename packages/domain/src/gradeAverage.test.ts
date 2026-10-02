import { describe, expect, it } from "vitest";

import { formatAverage, generalAverage, subjectAverage } from "./gradeAverage";
import vectors from "./gradeAverage.vectors.json";

// The SAME table services/grades/internal/domain/average_test.go runs: the
// web, the parents' app and the report card all average one way.

const toNumbers = (values: readonly string[]): number[] => values.map(Number);

describe("subjectAverage", () => {
  it.each(vectors.subject)("mean of $values is '$want'", ({ values, want }) => {
    expect(formatAverage(subjectAverage(toNumbers(values)))).toBe(want);
  });
});

describe("generalAverage", () => {
  it.each(vectors.general)("mean of the subjects $subjects is '$want'", ({ subjects, want }) => {
    expect(formatAverage(generalAverage(subjects.map(toNumbers)))).toBe(want);
  });
});

describe("formatAverage", () => {
  it("prints a truncated number without trailing zeros", () => {
    expect(formatAverage(8.5)).toBe("8.5");
    expect(formatAverage(9)).toBe("9");
    expect(formatAverage(8.66)).toBe("8.66");
  });

  it("prints nothing for no average", () => {
    expect(formatAverage(undefined)).toBe("");
    expect(formatAverage(null)).toBe("");
  });
});

describe("subjectAverage never rounds up", () => {
  it("8.666… is 8.66, not 8.67", () => {
    expect(subjectAverage([9, 8, 9])).toBe(8.66);
  });
});
