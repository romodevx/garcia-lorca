import { describe, expect, it } from "vitest";

import type { AttendanceSheetRow } from "../../attendance/sheet";
import { addDays, dateWindow, exceptionsOf, isDirty, sessionLine, tally, toggled } from "./sheetModel";

const row = (enrollmentId: string, status: AttendanceSheetRow["status"], note = ""): AttendanceSheetRow => ({
  enrollmentId,
  studentId: `st-${enrollmentId}`,
  name: enrollmentId,
  status,
  note,
});

describe("toggled — the one big tap", () => {
  it("flips present to absent, and anything else back to present", () => {
    expect(toggled("present")).toBe("absent");
    expect(toggled("absent")).toBe("present");
    expect(toggled("late")).toBe("present");
    expect(toggled("excused")).toBe("present");
  });
});

describe("tally", () => {
  it("counts every status", () => {
    expect(tally([row("a", "present"), row("b", "absent"), row("c", "late"), row("d", "present"), row("e", "excused")])).toEqual({
      present: 2,
      absent: 1,
      late: 1,
      excused: 1,
    });
  });
});

describe("exceptionsOf — the save payload", () => {
  it("carries only the non-present rows, with their notes, in list order", () => {
    expect(exceptionsOf([row("a", "present", "ignored"), row("b", "excused", "Cita médica"), row("c", "absent")])).toEqual([
      { enrollmentId: "b", status: "excused", note: "Cita médica" },
      { enrollmentId: "c", status: "absent" },
    ]);
  });
});

describe("isDirty", () => {
  const base = [row("a", "present"), row("b", "absent", "x")];
  it("is false for the same statuses and notes", () => {
    expect(isDirty(base, [row("a", "present"), row("b", "absent", "x")])).toBe(false);
  });
  it("is true when a status or a note changed", () => {
    expect(isDirty([row("a", "late"), base[1]!], base)).toBe(true);
    expect(isDirty([base[0]!, row("b", "absent", "y")], base)).toBe(true);
  });
});

describe("dates", () => {
  it("addDays moves across month and year ends", () => {
    expect(addDays("2026-09-03", -7)).toBe("2026-08-27");
    expect(addDays("2027-01-02", -7)).toBe("2026-12-26");
  });

  it("a teacher gets today…today−window; the director is unlimited", () => {
    expect(dateWindow("2026-09-26", 7, true)).toEqual({ min: "2026-09-19", max: "2026-09-26" });
    expect(dateWindow("2026-09-26", 7, false)).toEqual({});
  });
});

describe("sessionLine", () => {
  it("says who saved the list and at what local time", () => {
    expect(sessionLine({ takenAt: "2026-09-01T08:05:00", takenByName: "Mtra. Rojas" })).toBe("Lista guardada a las 8:05 por Mtra. Rojas");
    expect(sessionLine({ takenAt: "2026-09-01T14:30:00", takenByName: "Dir. Pérez" })).toBe("Lista guardada a las 14:30 por Dir. Pérez");
  });

  it("says the list has not been taken yet", () => {
    expect(sessionLine(null)).toBe("Lista aún no tomada");
  });
});
