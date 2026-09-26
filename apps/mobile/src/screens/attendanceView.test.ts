import type { AttendanceDay } from "../api/attendance";
import {
  dayLabel,
  monthLabel,
  monthOf,
  shiftMonth,
  statusPill,
  todayHeadline,
} from "./attendanceView";

const day = (over: Partial<AttendanceDay>): AttendanceDay => ({
  date: "2026-09-28",
  status: "present",
  mode: "daily",
  classes: [],
  ...over,
});

describe("attendance view helpers", () => {
  it("labels a day as short weekday + day + short month", () => {
    expect(dayLabel("2026-09-28")).toBe("Lun 28 sep");
    expect(dayLabel("2026-10-03")).toBe("Sáb 3 oct");
  });

  it("labels a month with its capitalized Spanish name and year", () => {
    expect(monthLabel("2026-09")).toBe("Septiembre 2026");
    expect(monthLabel("2027-01")).toBe("Enero 2027");
  });

  it("shifts months across year boundaries", () => {
    expect(shiftMonth("2026-01", -1)).toBe("2025-12");
    expect(shiftMonth("2026-12", 1)).toBe("2027-01");
    expect(shiftMonth("2026-09", -1)).toBe("2026-08");
  });

  it("takes the month of an ISO day", () => {
    expect(monthOf("2026-09-28")).toBe("2026-09");
  });

  it.each([
    ["present", "Asistió"],
    ["absent", "Faltó"],
    ["late", "Llegó tarde"],
    ["excused", "Falta justificada"],
    ["not_taken", "Lista aún no tomada"],
  ] as const)("headlines today's %s day as %s", (status, text) => {
    expect(todayHeadline(day({ status })).text).toBe(text);
  });

  it("headlines a partial per-class day with the attended count (late still counts as attended)", () => {
    const partial = day({
      status: "partial",
      mode: "per_class",
      classes: [
        { subjectName: "Español", status: "present" },
        { subjectName: "Matemáticas", status: "late" },
        { subjectName: "Inglés", status: "absent" },
        { subjectName: "Arte", status: "present" },
        { subjectName: "Música", status: "present" },
        { subjectName: "Educación física", status: "excused" },
      ],
    });
    expect(todayHeadline(partial).text).toBe("Asistió a 4 de 6 clases");
  });

  it("headlines a missing day (no list yet) as not taken", () => {
    expect(todayHeadline(undefined).text).toBe("Lista aún no tomada");
  });

  it("gives every status a text pill, never color alone", () => {
    expect(statusPill("absent")).toEqual({ label: "Faltó", kind: "bad" });
    expect(statusPill("late")).toEqual({ label: "Retardo", kind: "warn" });
    expect(statusPill("partial").label).toBe("Parcial");
    expect(statusPill("not_taken").label).toBe("Sin lista");
  });
});
