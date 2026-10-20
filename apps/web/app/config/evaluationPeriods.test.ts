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

function draft(key: string, startsOn: string, endsOn: string, name = "T", id = ""): PeriodDraft {
  return { key, id, name, startsOn, endsOn };
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
  const SEMESTERS = [
    { name: "1.ER SEMESTRE", startsOn: "2026-08-24", endsOn: "2027-01-22" },
    { name: "2.º SEMESTRE", startsOn: "2027-01-25", endsOn: "2027-07-09" },
  ];

  it("proposes the level's own periods, in order, when the level has them", () => {
    const first = nextDraft([], cycle, "n1", SEMESTERS);
    const second = nextDraft([first], cycle, "n2", SEMESTERS);
    expect([first, second]).toEqual([
      { key: "n1", id: "", name: "1.ER SEMESTRE", startsOn: "2026-08-24", endsOn: "2027-01-22" },
      { key: "n2", id: "", name: "2.º SEMESTRE", startsOn: "2027-01-25", endsOn: "2027-07-09" },
    ]);
  });

  it("proposes the level's next period that starts after the rows already there", () => {
    expect(nextDraft([draft("a", "2026-08-24", "2026-12-31")], cycle, "n2", SEMESTERS)).toEqual({
      key: "n2", id: "", name: "2.º SEMESTRE", startsOn: "2027-01-25", endsOn: "2027-07-09",
    });
  });

  it("past the level's periods, never names a row after a trimester", () => {
    const set = [draft("a", "2026-08-24", "2026-12-31")];
    expect(nextDraft(set, cycle, "n2", SEMESTERS.slice(0, 1)).name).toBe("2.º periodo");
  });

  it("names the first three rows after the trimesters and splits the cycle in contiguous thirds, the last ending with the cycle", () => {
    const first = nextDraft([], cycle, "n1");
    const second = nextDraft([first], cycle, "n2");
    const third = nextDraft([first, second], cycle, "n3");
    expect([first, second, third]).toEqual([
      { key: "n1", id: "", name: "1.er trimestre", startsOn: "2026-08-24", endsOn: "2026-12-08" },
      { key: "n2", id: "", name: "2.º trimestre", startsOn: "2026-12-09", endsOn: "2027-03-24" },
      { key: "n3", id: "", name: "3.er trimestre", startsOn: "2027-03-25", endsOn: "2027-07-09" },
    ]);
  });

  it("starts the next period the day after the last one ends; the third always ends on the cycle's last day", () => {
    const set = [draft("a", "2026-08-24", "2026-11-30"), draft("b", "2026-12-01", "2027-03-31")];
    expect(nextDraft(set, cycle, "n3")).toEqual({ key: "n3", id: "", name: "3.er trimestre", startsOn: "2027-04-01", endsOn: "2027-07-09" });
  });

  it("falls back to three months when an earlier row already runs past its third", () => {
    expect(nextDraft([draft("a", "2026-08-24", "2027-03-31")], cycle, "n2")).toEqual({
      key: "n2", id: "", name: "2.º trimestre", startsOn: "2027-04-01", endsOn: "2027-06-30",
    });
  });

  it("gives a fourth row on a generic ordinal name, three months long, clamped to the cycle", () => {
    const set = [draft("a", "2026-08-24", "2026-10-31"), draft("b", "2026-11-01", "2026-12-31"), draft("c", "2027-01-01", "2027-05-31")];
    expect(nextDraft(set, cycle, "n4")).toEqual({ key: "n4", id: "", name: "4.º periodo", startsOn: "2027-06-01", endsOn: "2027-07-09" });
  });

  it("leaves the dates empty when the cycle is already covered", () => {
    expect(nextDraft([draft("a", "2026-08-24", "2027-07-09")], cycle, "n2")).toEqual({ key: "n2", id: "", name: "2.º trimestre", startsOn: "", endsOn: "" });
  });

  it("never names a row PERIODO N", () => {
    expect(nextDraft([], cycle, "n1").name).not.toMatch(/PERIODO/);
  });
});

describe("drafts ↔ the wire", () => {
  it("draftsOf keeps ordinal order and keys by id", () => {
    const periods = [
      { id: "p2", cycleId: "c", gradeLevelId: "g", ordinal: 2, name: "B", startsOn: "2026-12-01", endsOn: "2027-03-31", termId: null },
      { id: "p1", cycleId: "c", gradeLevelId: "g", ordinal: 1, name: "A", startsOn: "2026-08-24", endsOn: "2026-11-30", termId: "t" },
    ];
    expect(draftsOf(periods)).toEqual([draft("p1", "2026-08-24", "2026-11-30", "A", "p1"), draft("p2", "2026-12-01", "2027-03-31", "B", "p2")]);
  });

  it("toInputs numbers the periods in date order, trims names and sends each saved row's id", () => {
    const set = [draft("b", "2026-12-01", "2027-03-31", " SEGUNDO ", "p2"), draft("a", "2026-08-24", "2026-11-30", "PRIMERO")];
    expect(toInputs(set)).toEqual([
      { ordinal: 1, name: "PRIMERO", startsOn: "2026-08-24", endsOn: "2026-11-30" },
      { id: "p2", ordinal: 2, name: "SEGUNDO", startsOn: "2026-12-01", endsOn: "2027-03-31" },
    ]);
  });
});
