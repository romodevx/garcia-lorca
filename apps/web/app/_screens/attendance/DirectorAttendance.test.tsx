import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as attendanceApi from "../../attendance/api";
import type { AttendanceSheet } from "../../attendance/sheet";
import * as configApi from "../../config/api";
import * as teachersApi from "../../teachers/api";
import { DirectorAttendance } from "./DirectorAttendance";

vi.mock("../../attendance/api", () => ({ getAttendanceSheet: vi.fn(), saveAttendanceSheet: vi.fn() }));
vi.mock("../../config/api", () => ({
  listCycles: vi.fn(),
  listLevels: vi.fn(),
  listGradeLevels: vi.fn(),
  listLabelledGroups: vi.fn(),
  listSubjects: vi.fn(),
}));
vi.mock("../../teachers/api", () => ({ listTeachingAssignments: vi.fn() }));

const attendance = vi.mocked(attendanceApi);
const config = vi.mocked(configApi);
const teachers = vi.mocked(teachersApi);
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

async function pickGroup(label: string): Promise<void> {
  const select = await screen.findByLabelText("Grupo");
  await screen.findByRole("option", { name: label });
  await userEvent.setup().selectOptions(select, label);
}

describe("DirectorAttendance", () => {
  it("a daily-level group opens its day list, with no date window", async () => {
    renderScreen();
    await pickGroup("1° PRIMARIA · A");

    await screen.findByRole("button", { name: "Ada Lovelace: Presente" });
    expect(attendance.getAttendanceSheet).toHaveBeenCalledWith(expect.any(String), { groupId: "gPri", date: TODAY });
    expect(screen.getByLabelText("Fecha")).not.toHaveAttribute("min");
    expect(screen.queryByRole("heading", { name: "Elige la asignatura" })).toBeNull();
  });

  it("a per-class group asks for one of the subjects taught in it, then opens that class's list", async () => {
    const user = userEvent.setup();
    renderScreen();
    await pickGroup("1° SECUNDARIA · A");

    expect(await screen.findByRole("heading", { name: "Elige la asignatura" })).toBeInTheDocument();
    await waitFor(() => expect(teachers.listTeachingAssignments).toHaveBeenCalledWith(expect.any(String), { groupId: "gSec" }));
    expect(screen.queryByRole("button", { name: "Artes" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Español" }));

    await waitFor(() =>
      expect(attendance.getAttendanceSheet).toHaveBeenCalledWith(expect.any(String), { groupId: "gSec", date: TODAY, subjectId: "s-esp" }),
    );
    expect(await screen.findByText("Pase de lista · 1° SECUNDARIA · A · Español")).toBeInTheDocument();
  });

  it("a per-class group with no subject assigned says so", async () => {
    teachers.listTeachingAssignments.mockResolvedValue([]);
    renderScreen();
    await pickGroup("1° SECUNDARIA · A");

    expect(await screen.findByText(/no tiene asignaturas asignadas/)).toBeInTheDocument();
    expect(attendance.getAttendanceSheet).not.toHaveBeenCalled();
  });

  it("saves through the director endpoint with the subject", async () => {
    const user = userEvent.setup();
    teachers.listTeachingAssignments.mockResolvedValue([assignment("a1", "s-mat")]);
    renderScreen();
    await pickGroup("1° SECUNDARIA · A");

    await user.click(await screen.findByRole("button", { name: "Ada Lovelace: Presente" }));
    await user.click(screen.getByRole("button", { name: "Guardar lista" }));

    await waitFor(() =>
      expect(attendance.saveAttendanceSheet).toHaveBeenCalledWith(expect.any(String), {
        groupId: "gSec",
        date: TODAY,
        subjectId: "s-mat",
        marks: [{ enrollmentId: "en1", status: "absent" }],
      }),
    );
  });

  it("changing group with unsaved marks asks first", async () => {
    const user = userEvent.setup();
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    renderScreen();
    await pickGroup("1° PRIMARIA · A");

    await user.click(await screen.findByRole("button", { name: "Ada Lovelace: Presente" }));
    await user.selectOptions(screen.getByLabelText("Grupo"), "1° SECUNDARIA · A");

    expect(confirm).toHaveBeenCalled();
    expect(screen.getByLabelText("Grupo")).toHaveValue("gPri");
  });
});
