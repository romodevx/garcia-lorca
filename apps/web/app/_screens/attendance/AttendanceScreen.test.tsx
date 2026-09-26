import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../config/api";
import * as studentsApi from "../../students/api";
import { AttendanceScreen } from "./AttendanceScreen";
import { Providers } from "../../test-utils";

vi.mock("../../attendance/api", () => ({ getAttendanceSheet: vi.fn(), saveAttendanceSheet: vi.fn() }));
vi.mock("../../config/api", () => ({
  listCycles: vi.fn(),
  listLevels: vi.fn(),
  listGradeLevels: vi.fn(),
  listLabelledGroups: vi.fn(),
  listSubjects: vi.fn(),
}));
vi.mock("../../teachers/api", () => ({ listTeachingAssignments: vi.fn() }));
vi.mock("../../students/api", () => ({ listStudents: vi.fn() }));

const config = vi.mocked(configApi);
const students = vi.mocked(studentsApi);

beforeEach(() => {
  config.listCycles.mockResolvedValue([{ id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" }]);
  config.listLevels.mockResolvedValue([]);
  config.listGradeLevels.mockResolvedValue([]);
  config.listLabelledGroups.mockResolvedValue([{ id: "g1", cycleId: "c1", gradeLevelId: "gl1", name: "A", capacity: 30, label: "1° A" }]);
  students.listStudents.mockResolvedValue([]);
});
afterEach(() => vi.clearAllMocks());

describe("AttendanceScreen", () => {
  it("renders the header and the group pickers", async () => {
    render(
      <Providers flags={{}}>
        <AttendanceScreen onBack={() => {}} today="2026-09-01" />
      </Providers>,
    );
    expect(await screen.findByRole("heading", { name: "Asistencia" })).toBeInTheDocument();
    expect(screen.getByLabelText("Ciclo")).toBeInTheDocument();
    expect(screen.getByLabelText("Grupo")).toBeInTheDocument();
    expect(await screen.findByRole("option", { name: "1° A" })).toBeInTheDocument();
    expect(screen.getByText("Selecciona un grupo para pasar lista.")).toBeInTheDocument();
  });
});
