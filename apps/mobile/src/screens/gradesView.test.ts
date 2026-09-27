import type { MonthlyGrades } from "../api/grades";
import { defaultMonth, formatGrade, isFailing, monthAverage, shortMonthLabel } from "./gradesView";

const scale = { min: 5, max: 10, decimals: 1, passing: 6 };

function grades(months: string[], subjectGrades: Record<string, number>[]): MonthlyGrades {
  return {
    cycleName: "2026-2027",
    months,
    scale,
    gradingEnabled: true,
    subjects: subjectGrades.map((g, i) => ({ subjectId: `s${i}`, subjectName: `Materia ${i}`, grades: g })),
  };
}

describe("defaultMonth", () => {
  const months = ["2026-08", "2026-09", "2026-10"];

  it("picks the latest month that has any grade", () => {
    const data = grades(months, [{ "2026-08": 9 }, { "2026-09": 8 }]);
    expect(defaultMonth(data, "2026-10")).toBe("2026-09");
  });

  it("falls back to the current month when nothing is captured and it is in the cycle", () => {
    expect(defaultMonth(grades(months, [{}]), "2026-09")).toBe("2026-09");
  });

  it("falls back to the last month of the cycle otherwise", () => {
    expect(defaultMonth(grades(months, [{}]), "2027-03")).toBe("2026-10");
  });

  it("uses the current month for an empty cycle", () => {
    expect(defaultMonth(grades([], []), "2026-09")).toBe("2026-09");
  });
});

describe("formatGrade / isFailing", () => {
  it("formats with the scale decimals", () => {
    expect(formatGrade(9.5, scale)).toBe("9.5");
    expect(formatGrade(8, { ...scale, decimals: 0 })).toBe("8");
  });

  it("fails strictly below passing", () => {
    expect(isFailing(5.9, scale)).toBe(true);
    expect(isFailing(6, scale)).toBe(false);
  });
});

describe("monthAverage", () => {
  it("averages only the captured subjects of the month", () => {
    const data = grades(["2026-09"], [{ "2026-09": 9 }, { "2026-09": 8 }, {}]);
    expect(monthAverage(data.subjects, "2026-09")).toBe(8.5);
  });

  it("is null when nothing is captured", () => {
    expect(monthAverage(grades(["2026-09"], [{}]).subjects, "2026-09")).toBeNull();
  });
});

describe("shortMonthLabel", () => {
  it("abbreviates the month name", () => {
    expect(shortMonthLabel("2026-09")).toBe("Sep");
  });
});
