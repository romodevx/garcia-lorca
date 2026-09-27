import type { MonthlyGrades } from "../api/grades";
import { defaultMonth, formatAverage, formatGrade, gradeLabel, isFailing, monthAverage, monthName, shortMonthLabel, subjectAverage } from "./gradesView";

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
  it("writes the stored value, never rounded to the scale", () => {
    expect(formatGrade(9.5)).toBe("9.5");
    expect(formatGrade(8)).toBe("8");
    expect(formatGrade(5.5)).toBe("5.5");
    expect(formatGrade(8.25)).toBe("8.25");
    expect(formatGrade(9.1)).toBe("9.1");
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

  it("rounds half-up to one decimal", () => {
    const data = grades(["2026-09"], [{ "2026-09": 8 }, { "2026-09": 8.5 }]);
    expect(monthAverage(data.subjects, "2026-09")).toBe(8.3);
    expect(formatAverage(8.3)).toBe("8.3");
    expect(formatAverage(8)).toBe("8.0");
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

describe("monthName", () => {
  it("is the lowercase month without the year, as it is read aloud", () => {
    expect(monthName("2026-09")).toBe("septiembre");
  });
});

describe("gradeLabel", () => {
  it("reads subject, month and grade, and says reprobada in words", () => {
    expect(gradeLabel("Matemáticas", "2026-09", 9.5, scale)).toBe("Matemáticas, septiembre: 9.5");
    expect(gradeLabel("Historia", "2026-09", 5, scale)).toBe("Historia, septiembre: 5, reprobada");
    expect(gradeLabel("Inglés", "2026-10", undefined, scale)).toBe("Inglés, octubre: sin calificación aún");
  });
});

describe("subjectAverage", () => {
  it("averages every captured month of one subject, null with none", () => {
    expect(subjectAverage({ subjectId: "m", subjectName: "M", grades: { "2026-08": 8, "2026-09": 5 } })).toBe(6.5);
    expect(subjectAverage({ subjectId: "m", subjectName: "M", grades: {} })).toBeNull();
  });
});
