import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import * as configApi from "../../config/api";
import { StudentsScreen } from "./StudentsScreen";

// Regression: ISSUE-002 — pupils left without a level by a level deletion
// (ADR 0021) showed a BLANK level cell, a blank level chip and an unlabeled
// "Por salón" chip, and counted as one more level in the "Niveles" card.
// Found by /qa on 2026-09-28
// Report: .gstack/qa-reports/qa-report-localhost-3002-2026-09-28-eliminar-niveles.md

vi.mock("../../students/api", async (importOriginal) => ({
  ...(await importOriginal<typeof studentsApi>()),
  listStudents: vi.fn(),
}));
vi.mock("../../config/api", () => ({
  listCycles: vi.fn(),
  listGroups: vi.fn(),
  listLabelledGroups: vi.fn(),
  listLevels: vi.fn(),
  listGradeLevels: vi.fn(),
}));

const students = vi.mocked(studentsApi);
const config = vi.mocked(configApi);

const pupil = (id: string, first: string, level: string, gradeGroup: string): studentsApi.Student => ({
  id,
  curp: `CURP${id}`,
  firstName: first,
  lastName: "Prueba",
  level,
  levelId: level === "" ? "" : "lv-s",
  gradeGroup,
  guardianName: "Ana Tutor",
  admissionStatus: "pending",
});

beforeEach(() => {
  students.listStudents.mockResolvedValue([
    pupil("1", "Regina", "SECUNDARIA", "A"),
    pupil("2", "Diego", "", ""),
    pupil("3", "Emilia", "", ""),
  ]);
  config.listCycles.mockResolvedValue([{ id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" }]);
  config.listGroups.mockResolvedValue([]);
  config.listLabelledGroups.mockResolvedValue([]);
  config.listLevels.mockResolvedValue([{ id: "lv-s", name: "SECUNDARIA", sepCode: "SEC", enrollmentGranularity: "cycle" }]);
  config.listGradeLevels.mockResolvedValue([]);
});
afterEach(() => vi.clearAllMocks());

describe("StudentsScreen — alumnos sin nivel (nivel eliminado)", () => {
  it("names them 'Sin nivel' / 'Sin grupo' instead of leaving the cells blank", async () => {
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByText("Prueba Diego");

    expect(screen.getByRole("button", { name: "Sin nivel" })).toBeInTheDocument();
    expect(screen.getByTestId("per-group")).toHaveTextContent("Sin grupo");
    expect(screen.getAllByText("Sin nivel").length).toBeGreaterThan(1);
  });

  it("filters them with the 'Sin nivel' chip, so the office can find whom to reassign", async () => {
    const user = userEvent.setup();
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByText("Prueba Diego");

    await user.click(screen.getByRole("button", { name: "Sin nivel" }));

    expect(screen.getByText("Prueba Diego")).toBeInTheDocument();
    expect(screen.getByText("Prueba Emilia")).toBeInTheDocument();
    expect(screen.queryByText("Prueba Regina")).toBeNull();
  });

  it("does not count 'Sin nivel' as a level in the Niveles card", async () => {
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByText("Prueba Diego");

    const card = screen.getByText("Niveles").parentElement as HTMLElement;
    expect(within(card).getByText("1")).toBeInTheDocument();
  });
});
