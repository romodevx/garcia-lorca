import { describe, expect, it } from "vitest";

import {
  draftsOf,
  nextDraft,
  periodProblems,
  rowProblems,
  toInputs,
  type PeriodDraft,
} from "./evaluationPeriods";

const cycle = { startsOn: "2026-08-24", endsOn: "2027-07-09" };

function draft(key: string, startsOn: string, endsOn: string, name = "T"): PeriodDraft {
  return { key, name, startsOn, endsOn };
}

describe("rowProblems — one row, checked when its field is left", () => {
  it("accepts a period inside the cycle, a single day included", () => {
    expect(rowProblems(draft("a", "2026-09-01", "2026-09-01"), cycle)).toEqual([]);
  });

  it.each([
    ["no name", draft("a", "2026-09-01", "2026-09-30", "  "), "name", "Escribe el nombre del periodo."],
    ["no start", draft("a", "", "2026-09-30"), "startsOn", "Elige la fecha de inicio."],
    ["no end", draft("a", "2026-09-01", ""), "endsOn", "Elige la fecha de fin."],
    ["ends before it starts", draft("a", "2026-10-01", "2026-09-30"), "endsOn", "El fin es antes que el inicio."],
    ["starts before the cycle", draft("a", "2026-08-23", "2026-09-30"), "startsOn", "Queda fuera del ciclo (24/08/2026 a 09/07/2027)."],
    ["ends after the cycle", draft("a", "2027-06-01", "2027-07-10"), "endsOn", "Queda fuera del ciclo (24/08/2026 a 09/07/2027)."],
  ] as const)("%s", (_, row, field, message) => {
    expect(rowProblems(row, cycle)).toContainEqual({ key: "a", field, message });
  });
});

describe("periodProblems — the whole set, checked on save", () => {
  it("names the period another one overlaps, on the later one's start", () => {
    const set = [draft("b", "2026-11-30", "2027-03-31", "2.º"), draft("a", "2026-08-24", "2026-11-30", "1.º")];
    expect(periodProblems(set, cycle)).toEqual([{ key: "b", field: "startsOn", message: "Se encima con «1.º» (termina el 30/11/2026)." }]);
  });

  it("adjacent periods do not overlap", () => {
    const set = [draft("a", "2026-08-24", "2026-11-30"), draft("b", "2026-12-01", "2027-03-31")];
    expect(periodProblems(set, cycle)).toEqual([]);
  });

  it("reports row problems first and skips overlap checks on rows without dates", () => {
    const set = [draft("a", "2026-08-24", "2026-11-30"), draft("b", "", "2026-10-01")];
    expect(periodProblems(set, cycle)).toEqual([{ key: "b", field: "startsOn", message: "Elige la fecha de inicio." }]);
  });

  it("refuses more than twelve periods", () => {
    const set = Array.from({ length: 13 }, (_, i) => draft(String(i), `2026-09-${String(i + 1).padStart(2, "0")}`, `2026-09-${String(i + 1).padStart(2, "0")}`));
    expect(periodProblems(set, cycle)).toContainEqual({ key: "12", field: "name", message: "Un grado puede tener hasta 12 periodos." });
  });
});

describe("nextDraft — the defaults of a new row", () => {
  it("starts the first period on the cycle's first day, three months long", () => {
    expect(nextDraft([], cycle, "n1")).toEqual({ key: "n1", name: "PERIODO 1", startsOn: "2026-08-24", endsOn: "2026-11-23" });
  });

  it("starts the next period the day after the last one ends, clamped to the cycle", () => {
    const set = [draft("a", "2026-08-24", "2026-11-30"), draft("b", "2026-12-01", "2027-05-31")];
    expect(nextDraft(set, cycle, "n3")).toEqual({ key: "n3", name: "PERIODO 3", startsOn: "2027-06-01", endsOn: "2027-07-09" });
  });

  it("leaves the dates empty when the cycle is already covered", () => {
    expect(nextDraft([draft("a", "2026-08-24", "2027-07-09")], cycle, "n2")).toEqual({ key: "n2", name: "PERIODO 2", startsOn: "", endsOn: "" });
  });
});

describe("drafts ↔ the wire", () => {
  it("draftsOf keeps ordinal order and keys by id", () => {
    const periods = [
      { id: "p2", cycleId: "c", gradeLevelId: "g", ordinal: 2, name: "B", startsOn: "2026-12-01", endsOn: "2027-03-31", termId: null },
      { id: "p1", cycleId: "c", gradeLevelId: "g", ordinal: 1, name: "A", startsOn: "2026-08-24", endsOn: "2026-11-30", termId: "t" },
    ];
    expect(draftsOf(periods)).toEqual([draft("p1", "2026-08-24", "2026-11-30", "A"), draft("p2", "2026-12-01", "2027-03-31", "B")]);
  });

  it("toInputs numbers the periods in date order and trims names", () => {
    const set = [draft("b", "2026-12-01", "2027-03-31", " SEGUNDO "), draft("a", "2026-08-24", "2026-11-30", "PRIMERO")];
    expect(toInputs(set)).toEqual([
      { ordinal: 1, name: "PRIMERO", startsOn: "2026-08-24", endsOn: "2026-11-30" },
      { ordinal: 2, name: "SEGUNDO", startsOn: "2026-12-01", endsOn: "2027-03-31" },
    ]);
  });
});
