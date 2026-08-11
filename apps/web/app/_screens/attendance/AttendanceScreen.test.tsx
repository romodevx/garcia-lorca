import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as attApi from "../../attendance/api";
import * as configApi from "../../config/api";
import * as studentsApi from "../../students/api";
import { AttendanceScreen } from "./AttendanceScreen";
import { Providers } from "../../test-utils";

vi.mock("../../attendance/api", () => ({ getDailyRoster: vi.fn(), saveDailyAttendance: vi.fn() }));
vi.mock("../../config/api", () => ({ listCycles: vi.fn(), listGroups: vi.fn() }));
vi.mock("../../students/api", () => ({ listStudents: vi.fn() }));

const att = vi.mocked(attApi);
const config = vi.mocked(configApi);
const students = vi.mocked(studentsApi);

beforeEach(() => {
  config.listCycles.mockResolvedValue([{ id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" }]);
  config.listGroups.mockResolvedValue([{ id: "g1", cycleId: "c1", gradeLevelId: "gl1", name: "1° A", capacity: 30 }]);
  students.listStudents.mockResolvedValue([]);
  att.getDailyRoster.mockResolvedValue([]);
});
afterEach(() => vi.clearAllMocks());

describe("AttendanceScreen", () => {
  it("renders the header, the day pickers and the stat cards", async () => {
    render(
      <Providers flags={{}}>
        <AttendanceScreen onBack={() => {}} today="2026-09-01" />
      </Providers>,
    );
    expect(await screen.findByRole("heading", { name: "Asistencia" })).toBeInTheDocument();
    expect(screen.getByLabelText("Ciclo")).toBeInTheDocument();
    expect(screen.getByLabelText("Grupo")).toBeInTheDocument();
    expect(screen.getByLabelText("Fecha")).toBeInTheDocument();
    expect(screen.getByText("Presentes")).toBeInTheDocument();
    expect(screen.getByText("Ausentes")).toBeInTheDocument();
  });
});
