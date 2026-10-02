import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as attendanceApi from "../../attendance/api";
import * as reportApi from "../../attendance/report";
import type { AttendanceSheet } from "../../attendance/sheet";
import * as configApi from "../../config/api";
import * as teachersApi from "../../teachers/api";
import { DirectorAttendance } from "./DirectorAttendance";

vi.mock("../../attendance/api", () => ({ getAttendanceSheet: vi.fn(), saveAttendanceSheet: vi.fn(), scanAttendanceCard: vi.fn() }));
vi.mock("../../config/api", () => ({
  listCycles: vi.fn(),
  listLevels: vi.fn(),
  listGradeLevels: vi.fn(),
  listLabelledGroups: vi.fn(),
  listSubjects: vi.fn(),
}));
vi.mock("../../attendance/report", () => ({ getAttendanceReport: vi.fn(), downloadAttendanceReport: vi.fn() }));
vi.mock("../../teachers/api", () => ({ listTeachingAssignments: vi.fn() }));

const attendance = vi.mocked(attendanceApi);
const config = vi.mocked(configApi);
const teachers = vi.mocked(teachersApi);
const reports = vi.mocked(reportApi);
const TODAY = "2026-09-01";

const sheet: AttendanceSheet = {
  mode: "daily",
  date: TODAY,
  editable: true,
  windowDays: 7,
  session: null,
  rows: [{ enrollmentId: "en1", studentId: "st1", name: "Ada Lovelace", status: "present", note: "" }],
};

const assignment = (id: string, subjectId: string) => ({ id, cycleId: "c1", groupId: "gSec", subjectId, teacherId: "t1" });

beforeEach(() => {
  config.listCycles.mockResolvedValue([{ id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" }]);
  config.listLevels.mockResolvedValue([
    { id: "lPri", name: "Primaria", sepCode: "", enrollmentGranularity: "cycle", attendanceMode: "daily" },
    { id: "lSec", name: "Secundaria", sepCode: "", enrollmentGranularity: "cycle", attendanceMode: "per_class" },
  ]);
  config.listGradeLevels.mockResolvedValue([
    { id: "glPri1", levelId: "lPri", name: "1° PRIMARIA", ordinal: 1 },
    { id: "glSec1", levelId: "lSec", name: "1° SECUNDARIA", ordinal: 1 },
  ]);
  config.listLabelledGroups.mockResolvedValue([
    { id: "gPri", cycleId: "c1", gradeLevelId: "glPri1", name: "A", capacity: 30, label: "1° PRIMARIA · A" },
    { id: "gSec", cycleId: "c1", gradeLevelId: "glSec1", name: "A", capacity: 30, label: "1° SECUNDARIA · A" },
  ]);
  config.listSubjects.mockResolvedValue([
    { id: "s-mat", gradeLevelId: "glSec1", name: "Matemáticas" },
    { id: "s-esp", gradeLevelId: "glSec1", name: "Español" },
    { id: "s-art", gradeLevelId: "glSec1", name: "Artes" },
  ]);
  teachers.listTeachingAssignments.mockResolvedValue([assignment("a1", "s-mat"), assignment("a2", "s-esp")]);
  attendance.getAttendanceSheet.mockResolvedValue(sheet);
  attendance.saveAttendanceSheet.mockResolvedValue(undefined);
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

const renderScreen = () => render(<DirectorAttendance onBack={() => {}} today={TODAY} />);

describe("DirectorAttendance — Reporte (T12)", () => {
  it("switches from the pass list to the report and reads the group chosen level → grade → group", async () => {
    const user = userEvent.setup();
    reports.getAttendanceReport.mockResolvedValue({
      groupId: "gSec", groupLabel: "1° SECUNDARIA · A", from: "2026-09-01", to: TODAY, listsTaken: 1,
      rows: [{ enrollmentId: "en1", studentId: "st1", name: "LOVELACE ADA", active: true, present: 0, absent: 0, late: 1, excused: 0 }],
      totals: { present: 0, absent: 0, late: 1, excused: 0 },
    });
    renderScreen();

    await user.click(await screen.findByRole("radio", { name: "Reporte" }));
    await user.click(await within(screen.getByRole("group", { name: "Nivel" })).findByRole("button", { name: "Secundaria" }));
    await user.click(within(screen.getByRole("group", { name: "Grado" })).getByRole("button", { name: "1° SECUNDARIA" }));
    await user.click(within(screen.getByRole("group", { name: "Grupo" })).getByRole("button", { name: "A" }));

    await waitFor(() => expect(reports.getAttendanceReport).toHaveBeenCalledWith(expect.any(String), { groupId: "gSec", from: "2026-09-01", to: TODAY }));
    expect(await screen.findByRole("table", { name: "Asistencia por alumno" })).toBeInTheDocument();
    expect(screen.queryByLabelText("Grupo", { selector: "select" })).toBeNull();

    await user.click(screen.getByRole("radio", { name: "Pase de lista" }));
    expect(await screen.findByLabelText("Grupo", { selector: "select" })).toBeInTheDocument();
  });
});
