import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as attApi from "../../attendance/api";
import * as configApi from "../../config/api";
import * as studentsApi from "../../students/api";
import * as teachingApi from "../../teaching/api";
import { AttendanceScreen } from "./AttendanceScreen";
import { Providers } from "../../test-utils";

// Both paths are mocked so we can assert WHICH client set each role exercises.
// The director path boots the admin attendance + config + students clients; the
// teacher path is self-scoped to the teaching client alone.
vi.mock("../../attendance/api", () => ({ getAttendanceSheet: vi.fn(), saveAttendanceSheet: vi.fn(), scanAttendanceCard: vi.fn() }));
vi.mock("../../config/api", () => ({
  listCycles: vi.fn(),
  listLevels: vi.fn(),
  listGradeLevels: vi.fn(),
  listLabelledGroups: vi.fn(),
  listSubjects: vi.fn(),
}));
vi.mock("../../teachers/api", () => ({ listTeachingAssignments: vi.fn() }));
vi.mock("../../students/api", () => ({ listStudents: vi.fn() }));
vi.mock("../../teaching/api", () => ({
  listAttendanceClasses: vi.fn(),
  getTeachingAttendanceSheet: vi.fn(),
  saveTeachingAttendance: vi.fn(),
  scanTeachingCard: vi.fn(),
}));

const att = vi.mocked(attApi);
const config = vi.mocked(configApi);
const students = vi.mocked(studentsApi);
const teaching = vi.mocked(teachingApi);

beforeEach(() => {
  config.listCycles.mockResolvedValue([]);
  config.listLevels.mockResolvedValue([]);
  config.listGradeLevels.mockResolvedValue([]);
  config.listLabelledGroups.mockResolvedValue([]);
  students.listStudents.mockResolvedValue([]);
  teaching.listAttendanceClasses.mockResolvedValue([]);
});

afterEach(() => vi.clearAllMocks());

describe("AttendanceScreen — role-adaptive branch", () => {
  it("renders the TEACHER surface for an attendance_own:read-only principal, with NO students-list/admin-attendance/config call", async () => {
    render(
      <Providers flags={{}}>
        <AttendanceScreen onBack={() => {}} today="2026-09-01" permissions={["attendance_own:read"]} />
      </Providers>,
    );

    // The teacher subtitle distinguishes the self-scoped surface.
    expect(await screen.findByText(/solo tus grupos/)).toBeInTheDocument();
    await waitFor(() => expect(teaching.listAttendanceClasses).toHaveBeenCalled());

    // A teacher must never hit the director-only clients (they'd 403).
    expect(students.listStudents).not.toHaveBeenCalled();
    expect(att.getAttendanceSheet).not.toHaveBeenCalled();
    expect(config.listCycles).not.toHaveBeenCalled();
  });

  it("renders the DIRECTOR surface for an attendance:read principal (director wins over attendance_own:read)", async () => {
    render(
      <Providers flags={{}}>
        <AttendanceScreen onBack={() => {}} today="2026-09-01" permissions={["attendance:read", "attendance_own:read"]} />
      </Providers>,
    );

    // The director subtitle distinguishes the full, school-wide surface.
    expect(await screen.findByText(/marca solo las excepciones/)).toBeInTheDocument();
    await waitFor(() => expect(config.listCycles).toHaveBeenCalled());

    // The director path never touches the teacher client.
    expect(teaching.listAttendanceClasses).not.toHaveBeenCalled();
  });
});

describe("AttendanceScreen — Escanear QR follows module_student_card", () => {
  const TODAY = "2026-09-01";
  const sheet = {
    mode: "daily" as const,
    date: TODAY,
    editable: true,
    windowDays: 7,
    session: null,
    rows: [{ enrollmentId: "en1", studentId: "st1", name: "Ada Lovelace", status: "present" as const, note: "" }],
  };

  beforeEach(() => {
    window.history.replaceState(null, "", "/asistencia");
    teaching.listAttendanceClasses.mockResolvedValue([{ groupId: "g1", groupLabel: "1° A", levelName: "Primaria", mode: "daily" }]);
    teaching.getTeachingAttendanceSheet.mockResolvedValue(sheet);
  });

  const renderTeacher = (flags: Record<string, boolean>) =>
    render(
      <Providers flags={flags}>
        <AttendanceScreen onBack={() => {}} today={TODAY} permissions={["attendance_own:read"]} />
      </Providers>,
    );

  it("flag ON: the teacher's list offers Escanear QR", async () => {
    renderTeacher({ module_student_card: true });
    await screen.findByRole("button", { name: "Ada Lovelace: Presente" });
    expect(screen.getByRole("button", { name: "Escanear QR" })).toBeInTheDocument();
  });

  it("flag OFF: no Escanear QR", async () => {
    renderTeacher({});
    await screen.findByRole("button", { name: "Ada Lovelace: Presente" });
    expect(screen.queryByRole("button", { name: "Escanear QR" })).not.toBeInTheDocument();
  });
});
