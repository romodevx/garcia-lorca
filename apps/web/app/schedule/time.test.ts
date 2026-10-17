import { describe, expect, it } from "vitest";

import {
  SCHOOL_DAY_EARLIEST,
  SCHOOL_DAY_LATEST,
  timeError,
  addDays,
  defaultEnd,
  gridBounds,
  layoutColumns,
  mondayOf,
  nextDateOnWeekday,
  schoolWeekOf,
  nowInSchoolZone,
  rowOf,
  runningAt,
  snapToSlot,
  spanishDate,
  subjectColor,
  timeOptions,
  toHHMM,
  toMinutes,
  weekdayOf,
  weekLabel,
} from "./time";

describe("toMinutes / toHHMM", () => {
  it.each([
    ["00:00", 0],
    ["07:05", 425],
    ["08:50", 530],
    ["24:00", 1440],
  ])("%s is %i minutes and back", (hhmm, minutes) => {
    expect(toMinutes(hhmm)).toBe(minutes);
    expect(toHHMM(minutes)).toBe(hhmm);
  });

  it.each(["", "8:00", "25:00", "08:60", "ab:cd"])("%j is not a time", (bad) => {
    expect(Number.isNaN(toMinutes(bad))).toBe(true);
  });
});

describe("rowOf", () => {
  it.each([
    [420, 420, 1],
    [425, 420, 2],
    [480, 420, 13],
  ])("minute %i on a grid from %i is row %i (5-minute rows)", (minute, start, row) => {
    expect(rowOf(minute, start)).toBe(row);
  });
});

describe("snapToSlot", () => {
  it.each([
    [480, 480],
    [484, 480],
    [487, 485],
  ])("%i snaps down to %i", (m, want) => {
    expect(snapToSlot(m)).toBe(want);
  });
});

describe("gridBounds", () => {
  it("defaults to 07:00–15:00 with no level grid", () => {
    expect(gridBounds(undefined, [])).toEqual({ start: 420, end: 900 });
  });

  it("uses the level's grid", () => {
    expect(gridBounds({ gridStart: "07:30", gridEnd: "14:00" }, [])).toEqual({ start: 450, end: 840 });
  });

  it("widens itself to whole hours around any class outside it", () => {
    const blocks = [{ startsAt: "06:40", endsAt: "07:30" }, { startsAt: "15:00", endsAt: "15:20" }];
    expect(gridBounds({ gridStart: "07:00", gridEnd: "15:00" }, blocks)).toEqual({ start: 360, end: 960 });
  });
});

describe("defaultEnd", () => {
  it.each([
    [480, 900, 530],
    [870, 900, 900],
  ])("a class from %i on a grid ending %i ends at %i", (start, gridEnd, want) => {
    expect(defaultEnd(start, gridEnd)).toBe(want);
  });
});

describe("timeOptions", () => {
  it("lists 5-minute steps, both ends included", () => {
    expect(timeOptions(480, 500)).toEqual(["08:00", "08:05", "08:10", "08:15", "08:20"]);
  });
});

describe("layoutColumns (calendar overlap columns)", () => {
  const ev = (id: string, s: number, e: number): { id: string; start: number; end: number } => ({ id, start: s, end: e });

  it("a lone class takes the whole column", () => {
    expect(layoutColumns([ev("a", 480, 530)]).get("a")).toEqual({ column: 0, columns: 1 });
  });

  it("back-to-back classes do not overlap (end is exclusive)", () => {
    const got = layoutColumns([ev("a", 480, 530), ev("b", 530, 580)]);
    expect(got.get("a")).toEqual({ column: 0, columns: 1 });
    expect(got.get("b")).toEqual({ column: 0, columns: 1 });
  });

  it("two overlapping classes sit side by side", () => {
    const got = layoutColumns([ev("a", 480, 530), ev("b", 500, 560)]);
    expect(got.get("a")).toEqual({ column: 0, columns: 2 });
    expect(got.get("b")).toEqual({ column: 1, columns: 2 });
  });

  it("a freed column is reused inside the same cluster", () => {
    // a 8:00–9:00, b 8:10–8:30, c 8:40–9:10: b ends before c, so c reuses b's column.
    const got = layoutColumns([ev("a", 480, 540), ev("b", 490, 510), ev("c", 520, 550)]);
    expect(got.get("a")).toEqual({ column: 0, columns: 2 });
    expect(got.get("b")).toEqual({ column: 1, columns: 2 });
    expect(got.get("c")).toEqual({ column: 1, columns: 2 });
  });

  it("separate clusters size their columns independently", () => {
    const got = layoutColumns([ev("a", 480, 530), ev("b", 490, 520), ev("c", 600, 650)]);
    expect(got.get("c")).toEqual({ column: 0, columns: 1 });
  });

  it("three at once take three columns", () => {
    const got = layoutColumns([ev("a", 480, 530), ev("b", 480, 530), ev("c", 480, 530)]);
    expect([...got.values()].map((v) => v.column).sort()).toEqual([0, 1, 2]);
    expect(got.get("a")?.columns).toBe(3);
  });
});

describe("subjectColor", () => {
  it("is deterministic per name", () => {
    expect(subjectColor("Matemáticas")).toBe(subjectColor("Matemáticas"));
  });

  it("spreads different subjects over the palette", () => {
    const names = ["Matemáticas", "Español", "Inglés", "Historia", "Geografía", "Ciencias", "Música", "Educación Física"];
    expect(new Set(names.map(subjectColor)).size).toBeGreaterThan(2);
  });
});

describe("nowInSchoolZone (America/Mexico_City, never the browser's zone)", () => {
  it.each([
    // 2026-10-05 is a Monday. Mexico City is UTC−6 all year (no DST since 2022).
    ["2026-10-05T14:07:00Z", { date: "2026-10-05", time: "08:07", weekday: 1 }],
    // 01:30 UTC on Tuesday is still Monday evening at the school.
    ["2026-10-06T01:30:00Z", { date: "2026-10-05", time: "19:30", weekday: 1 }],
    ["2026-10-04T18:00:00Z", { date: "2026-10-04", time: "12:00", weekday: 7 }],
  ])("%s", (iso, want) => {
    expect(nowInSchoolZone(new Date(iso))).toEqual(want);
  });
});

describe("weekdayOf / nextDateOnWeekday", () => {
  it.each([
    ["2026-10-05", 1],
    ["2026-10-09", 5],
    ["2026-10-11", 7],
  ])("%s is weekday %i", (iso, wd) => {
    expect(weekdayOf(iso)).toBe(wd);
  });

  it.each([
    ["2026-10-05", 1, "2026-10-05"], // today is that weekday
    ["2026-10-05", 3, "2026-10-07"],
    ["2026-10-08", 1, "2026-10-12"], // past it this week: next week
    ["2026-10-31", 2, "2026-11-03"], // across a month
  ])("from %s the next weekday %i is %s", (today, wd, want) => {
    expect(nextDateOnWeekday(today, wd)).toBe(want);
  });
});

describe("runningAt", () => {
  const cls = (id: string, s: string, e: string): { blockId: string; startsAt: string; endsAt: string } => ({ blockId: id, startsAt: s, endsAt: e });

  it("keeps the classes under way at that minute, the end exclusive", () => {
    const got = runningAt([cls("a", "08:00", "08:50"), cls("b", "08:50", "09:40"), cls("c", "07:00", "08:00")], "08:00");
    expect(got.map((c) => c.blockId)).toEqual(["a"]);
    expect(runningAt([cls("a", "08:00", "08:50"), cls("b", "08:50", "09:40")], "08:50").map((c) => c.blockId)).toEqual(["b"]);
  });
});

describe("week helpers (T19)", () => {
  it("finds the Monday of a date's week and adds days across months", () => {
    expect(mondayOf("2026-10-07")).toBe("2026-10-05");
    expect(mondayOf("2026-10-05")).toBe("2026-10-05");
    expect(mondayOf("2026-10-11")).toBe("2026-10-05");
    expect(addDays("2026-09-30", 2)).toBe("2026-10-02");
    expect(addDays("2026-10-05", -7)).toBe("2026-09-28");
  });

  it("names a school week in Spanish", () => {
    expect(weekLabel("2026-10-05")).toBe("Semana del 5 al 9 de octubre");
    expect(weekLabel("2026-09-28")).toBe("Semana del 28 de septiembre al 2 de octubre");
  });

  it("writes a date the way people read it", () => {
    expect(spanishDate("2026-10-05")).toBe("lunes 5 de octubre");
  });
});

describe("schoolWeekOf (review T19)", () => {
  it("is the date's week on a school day and NEXT week on a weekend", () => {
    expect(schoolWeekOf("2026-10-07")).toBe("2026-10-05");
    expect(schoolWeekOf("2026-10-10")).toBe("2026-10-12");
    expect(schoolWeekOf("2026-10-11")).toBe("2026-10-12");
  });
});

describe("ISSUE-028 · a typed time, judged against the school day", () => {
  it("the school day spans 07:00 to 20:00", () => {
    expect(toHHMM(SCHOOL_DAY_EARLIEST)).toBe("07:00");
    expect(toHHMM(SCHOOL_DAY_LATEST)).toBe("20:00");
  });

  const range = { min: SCHOOL_DAY_EARLIEST, max: SCHOOL_DAY_LATEST };
  it.each([
    ["07:00", undefined],
    ["20:00", undefined],
    ["08:35", undefined],
    ["06:55", "Escribe una hora entre 07:00 y 20:00."],
    ["20:05", "Escribe una hora entre 07:00 y 20:00."],
    ["08:07", "La hora va en pasos de 5 minutos: 08:05 o 08:10."],
    ["", undefined],
  ])("%s → %s", (value, want) => {
    expect(timeError(value, range)).toBe(want);
  });

  it("a time already saved outside the day is kept as it is, never refused", () => {
    expect(timeError("06:30", { ...range, kept: "06:30" })).toBeUndefined();
    expect(timeError("06:35", { ...range, kept: "06:30" })).toBe("Escribe una hora entre 07:00 y 20:00.");
  });
});
