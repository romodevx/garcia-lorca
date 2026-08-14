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
vi.mock("../../attendance/api", () => ({ getDailyRoster: vi.fn(), saveDailyAttendance: vi.fn() }));
vi.mock("../../config/api", () => ({ listCycles: vi.fn(), listGroups: vi.fn() }));
vi.mock("../../students/api", () => ({ listStudents: vi.fn() }));
vi.mock("../../teaching/api", () => ({
  listGroups: vi.fn(),
  getAttendanceRoster: vi.fn(),
  saveAttendance: vi.fn(),
}));

const att = vi.mocked(attApi);
const config = vi.mocked(configApi);
const students = vi.mocked(studentsApi);
const teaching = vi.mocked(teachingApi);

beforeEach(() => {
  config.listCycles.mockResolvedValue([]);
  config.listGroups.mockResolvedValue([]);
  students.listStudents.mockResolvedValue([]);
  att.getDailyRoster.mockResolvedValue([]);
  teaching.listGroups.mockResolvedValue([]);
  teaching.getAttendanceRoster.mockResolvedValue([]);
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
    await waitFor(() => expect(teaching.listGroups).toHaveBeenCalled());

    // A teacher must never hit the director-only clients (they'd 403).
    expect(students.listStudents).not.toHaveBeenCalled();
    expect(att.getDailyRoster).not.toHaveBeenCalled();
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
    expect(teaching.listGroups).not.toHaveBeenCalled();
  });
});
