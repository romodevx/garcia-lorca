import { afterEach, describe, expect, it, vi } from "vitest";

import { downloadAttendanceReport, getAttendanceReport, type AttendanceReport } from "./report";

const BASE = "http://gw/";
const Q = { groupId: "g1", from: "2026-09-01", to: "2026-09-30" };

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const report: AttendanceReport = {
  groupId: "g1",
  groupLabel: "1° SECUNDARIA · A",
  from: "2026-09-01",
  to: "2026-09-30",
  listsTaken: 20,
  rows: [{ enrollmentId: "e1", studentId: "s1", name: "GARCÍA ANA", present: 16, absent: 1, late: 3, excused: 0 }],
  totals: { present: 16, absent: 1, late: 3, excused: 0 },
};

describe("getAttendanceReport (T12)", () => {
  it("GETs the group's report over the range with the session", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(report), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(getAttendanceReport(BASE, Q)).resolves.toEqual(report);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("http://gw/api/v1/attendance/reports?groupId=g1&from=2026-09-01&to=2026-09-30");
    expect(init.credentials).toBe("include");
  });

  it("reads a missing rows list as empty", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ ...report, rows: null }), { status: 200 })));
    await expect(getAttendanceReport(BASE, Q)).resolves.toMatchObject({ rows: [] });
  });

  it("throws the gateway's sentence on a refusal", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("El reporte abarca como máximo 366 días.", { status: 400 })));
    await expect(getAttendanceReport(BASE, Q)).rejects.toThrow("El reporte abarca como máximo 366 días.");
  });
});

describe("downloadAttendanceReport (T12)", () => {
  it("fetches the export and names it from Content-Disposition", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response("%PDF", { status: 200, headers: { "Content-Disposition": 'attachment; filename="reporte-asistencia-2026-09-01-2026-09-30.pdf"' } }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const out = await downloadAttendanceReport(BASE, Q, "pdf");
    expect(out.filename).toBe("reporte-asistencia-2026-09-01-2026-09-30.pdf");
    expect((fetchMock.mock.calls[0] as [string])[0]).toBe("http://gw/api/v1/attendance/reports/pdf?groupId=g1&from=2026-09-01&to=2026-09-30");
  });
});
