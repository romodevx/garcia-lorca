import type { AttendanceDay, AttendanceMonth } from "../api/attendance";
import {
  dayLabel,
  findToday,
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

  it("headlines an excused per-class day with how many classes were excused", () => {
    const excused = day({
      status: "excused",
      mode: "per_class",
      classes: [
        { subjectName: "Español", status: "present" },
        { subjectName: "Matemáticas", status: "excused" },
        { subjectName: "Inglés", status: "present" },
      ],
    });
    expect(todayHeadline(excused).text).toBe("Falta justificada en 1 de 3 clases");
  });

  it("headlines a per-class day excused in every class as a plain excused day", () => {
    const excused = day({
      status: "excused",
      mode: "per_class",
      classes: [
        { subjectName: "Español", status: "excused" },
        { subjectName: "Matemáticas", status: "excused" },
      ],
    });
    expect(todayHeadline(excused).text).toBe("Falta justificada");
  });

  it("headlines a per-class day late in one class as late", () => {
    const late = day({
      status: "late",
      mode: "per_class",
      classes: [
        { subjectName: "Español", status: "present" },
        { subjectName: "Matemáticas", status: "late" },
      ],
    });
    expect(todayHeadline(late).text).toBe("Llegó tarde");
  });

  it("headlines an unknown today as a dash, never as a false 'not taken'", () => {
    const headline = todayHeadline(null);
    expect(headline.text).toBe("—");
    expect(headline.spoken).toBe("Sin datos todavía");
  });

  it("says there are no classes on a weekend day without a list", () => {
    // 2026-09-26 is a Saturday, 2026-09-27 a Sunday.
    expect(todayHeadline(day({ date: "2026-09-26", status: "not_taken" })).text).toBe("Sin clases hoy");
    expect(todayHeadline(day({ date: "2026-09-27", status: "not_taken" })).text).toBe("Sin clases hoy");
    // A weekend day that DID get a list still says what happened.
    expect(todayHeadline(day({ date: "2026-09-26", status: "present" })).text).toBe("Asistió");
  });

  describe("findToday", () => {
    const month = (over: Partial<AttendanceMonth>): AttendanceMonth => ({
      month: "2026-09",
      today: "2026-09-28",
      days: [day({ date: "2026-09-28", status: "absent" }), day({ date: "2026-09-25" })],
      summary: { present: 0, absent: 0, late: 0, excused: 0, partial: 0 },
      ...over,
    });

    it("finds today's entry in the current month", () => {
      expect(findToday(month({}))?.status).toBe("absent");
    });

    it("is unknown before the month has loaded", () => {
      expect(findToday(null)).toBeNull();
    });

    it("is unknown when today's entry is missing", () => {
      expect(findToday(month({ days: [day({ date: "2026-09-25" })] }))).toBeNull();
    });

    it("is unknown when the server sent no today", () => {
      expect(findToday(month({ today: "" }))).toBeNull();
    });

    it("is unknown when today falls outside the fetched month (midnight rollover)", () => {
      expect(findToday(month({ today: "2026-10-01", days: [day({ date: "2026-10-01" })] }))).toBeNull();
    });
  });

  it("gives every status a text pill, never color alone", () => {
    expect(statusPill("absent")).toEqual({ label: "Faltó", kind: "bad" });
    expect(statusPill("late")).toEqual({ label: "Retardo", kind: "warn" });
    expect(statusPill("partial").label).toBe("Parcial");
    expect(statusPill("not_taken").label).toBe("Sin lista");
  });
});
