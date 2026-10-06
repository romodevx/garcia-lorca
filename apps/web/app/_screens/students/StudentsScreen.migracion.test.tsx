import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import * as configApi from "../../config/api";
import { StudentsScreen } from "./StudentsScreen";

vi.mock("../../students/api", async (importOriginal) => ({
  ...(await importOriginal<typeof studentsApi>()),
  listStudents: vi.fn(),
  listEnrollments: vi.fn(),
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

beforeEach(() => {
  window.history.replaceState(null, "", "/alumnos");
  students.listEnrollments.mockResolvedValue([]);
  students.listStudents.mockResolvedValue([]);
  config.listCycles.mockResolvedValue([]);
  config.listGroups.mockResolvedValue([]);
  config.listLabelledGroups.mockResolvedValue([]);
  config.listLevels.mockResolvedValue([]);
  config.listGradeLevels.mockResolvedValue([]);
});
afterEach(() => vi.clearAllMocks());

describe("StudentsScreen — the way into Migrar alumnos (ADR 0033)", () => {
  it("offers «¿Vienes de otro sistema?» when the school has no pupils yet", async () => {
    const onOpenImport = vi.fn();
    const user = userEvent.setup();
    render(<StudentsScreen onBack={() => {}} onOpenImport={onOpenImport} />);

    await user.click(await screen.findByRole("button", { name: "¿Vienes de otro sistema?" }));

    expect(onOpenImport).toHaveBeenCalled();
  });

  it("finds a pupil by the BBVA reference the migration brought", async () => {
    const pupil = (id: string, first: string, bankReference: string): studentsApi.Student => ({
      id, curp: `CURP${id}`, firstName: first, lastName: "Prueba", level: "", levelId: "", gradeGroup: "", guardianName: "", admissionStatus: "enrolled", bankReference,
    });
    students.listStudents.mockResolvedValue([pupil("1", "Regina", "231076"), pupil("2", "Diego", "998877")]);
    const user = userEvent.setup();
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByText("Prueba Diego");

    await user.type(screen.getByLabelText("Buscar"), "231076");

    await waitFor(() => expect(screen.queryByText("Prueba Diego")).toBeNull());
    expect(screen.getByText("Prueba Regina")).toBeInTheDocument();
  });

  it("offers nothing without the permission and flag (no callback given)", async () => {
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByText("Sin alumnos en esta vista");
    expect(screen.queryByRole("button", { name: "¿Vienes de otro sistema?" })).not.toBeInTheDocument();
  });
});
