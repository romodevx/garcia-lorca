import type { EvaluationPeriod, PeriodSubject } from "../api/grades";
import {
  defaultPeriod,
  deviceDay,
  formatGrade,
  generalAverageOf,
  gradeLabel,
  hasCapturedGrades,
  isFailing,
  subjectAverageOf,
} from "./gradesView";

const scale = { min: 5, max: 10, decimals: 0, passing: 6 };

const periods: EvaluationPeriod[] = [
  { id: "p1", ordinal: 1, name: "1.er trimestre", startsOn: "2026-08-24", endsOn: "2026-11-30" },
  { id: "p2", ordinal: 2, name: "2.º trimestre", startsOn: "2026-12-07", endsOn: "2027-03-15" },
  { id: "p3", ordinal: 3, name: "3.er trimestre", startsOn: "2027-03-16", endsOn: "2027-07-10" },
];

const subject = (grades: Record<string, number>): PeriodSubject => ({ subjectId: "m", subjectName: "M", grades, letters: {} });

describe("defaultPeriod", () => {
  it("picks the period that contains today, both ends inclusive", () => {
    expect(defaultPeriod(periods, "2026-12-07")).toBe("p2");
    expect(defaultPeriod(periods, "2026-11-30")).toBe("p1");
    expect(defaultPeriod(periods, "2027-01-15")).toBe("p2");
  });

  it("between two periods, picks the latest one that already started", () => {
    expect(defaultPeriod(periods, "2026-12-03")).toBe("p1");
  });

  it("after the cycle, picks the last period", () => {
    expect(defaultPeriod(periods, "2027-08-01")).toBe("p3");
  });

  it("before the cycle starts, picks the first period", () => {
    expect(defaultPeriod(periods, "2026-08-01")).toBe("p1");
  });

  it("is null without periods", () => {
    expect(defaultPeriod([], "2026-09-01")).toBeNull();
  });
});

describe("deviceDay", () => {
  it("is the phone's LOCAL date, not the UTC one", () => {
    expect(deviceDay(new Date(2026, 8, 3, 23, 30))).toBe("2026-09-03");
    expect(deviceDay(new Date(2026, 0, 1, 0, 5))).toBe("2026-01-01");
  });
});

describe("formatGrade / isFailing", () => {
  it("writes the stored value exactly, never rounded", () => {
    expect(formatGrade(9)).toBe("9");
    expect(formatGrade(5.5)).toBe("5.5");
    expect(formatGrade(8.25)).toBe("8.25");
  });

  it("fails strictly below passing; without a scale nothing does", () => {
    expect(isFailing(5, scale)).toBe(true);
    expect(isFailing(6, scale)).toBe(false);
    expect(isFailing(3, null)).toBe(false);
  });
});

describe("averages (the shared T15 rule: exact mean, truncated to 2 decimals)", () => {
  it("averages one subject across its captured periods", () => {
    expect(subjectAverageOf(subject({ p1: 9, p2: 9, p3: 8 }))).toBe(8.66);
    expect(subjectAverageOf(subject({ p1: 8, p2: 5 }))).toBe(6.5);
    expect(subjectAverageOf(subject({}))).toBeUndefined();
  });

  it("averages the subjects' exact averages, truncated once", () => {
    const a = subject({ p1: 9, p2: 9, p3: 8 }); // 26/3
    const b = subject({ p1: 9, p2: 8, p3: 8 }); // 25/3
    expect(generalAverageOf([a, b, subject({})])).toBe(8.5);
    expect(generalAverageOf([subject({})])).toBeUndefined();
  });
});

describe("hasCapturedGrades", () => {
  it("is true only when some subject carries a grade", () => {
    expect(hasCapturedGrades([subject({}), subject({ p1: 9 })])).toBe(true);
    expect(hasCapturedGrades([subject({})])).toBe(false);
  });
});

describe("gradeLabel", () => {
  it("reads subject, period and grade, and says reprobada in words", () => {
    expect(gradeLabel("Matemáticas", "1.er trimestre", 9, scale)).toBe("Matemáticas, 1.er trimestre: 9");
    expect(gradeLabel("Historia", "1.er trimestre", 5, scale)).toBe("Historia, 1.er trimestre: 5, reprobada");
    expect(gradeLabel("Inglés", "2.º trimestre", undefined, scale)).toBe("Inglés, 2.º trimestre: sin calificación aún");
  });
});
