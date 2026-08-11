import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import * as configApi from "../../config/api";
import { StudentsScreen } from "./StudentsScreen";

vi.mock("../../students/api", () => ({
  listStudents: vi.fn(),
  createStudent: vi.fn(),
  enroll: vi.fn(),
}));
vi.mock("../../config/api", () => ({
  listCycles: vi.fn(),
  listGroups: vi.fn(),
}));

const students = vi.mocked(studentsApi);
const config = vi.mocked(configApi);

const mk = (id: string, first: string, last: string, status: studentsApi.AdmissionStatus): studentsApi.Student => ({
  id,
  curp: `CURP${id}`,
  firstName: first,
  lastName: last,
  level: "primaria",
  gradeGroup: "1° A",
  guardianName: "Ana Tutor",
  admissionStatus: status,
});

beforeEach(() => {
  students.listStudents.mockResolvedValue([mk("1", "Valentina", "Cruz", "enrolled"), mk("2", "Mateo", "Sánchez", "pending")]);
  config.listCycles.mockResolvedValue([{ id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" }]);
  config.listGroups.mockResolvedValue([]);
});
afterEach(() => vi.clearAllMocks());

describe("StudentsScreen", () => {
  it("renders the header, stat cards and the full student table", async () => {
    render(<StudentsScreen onBack={() => {}} />);

    expect(await screen.findByRole("heading", { name: "Estudiantes" })).toBeInTheDocument();
    // both students listed by name
    expect(await screen.findByText("Valentina Cruz")).toBeInTheDocument();
    expect(screen.getByText("Mateo Sánchez")).toBeInTheDocument();
    // status pills reflect admission status ("Inscrito" pill is unique; "Trámite"
    // also names a tab, so assert at least one pill exists).
    expect(screen.getByText("Inscrito")).toBeInTheDocument();
    expect(screen.getAllByText("Trámite").length).toBeGreaterThan(0);
  });

  it("filters by the search box", async () => {
    const user = userEvent.setup();
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByText("Valentina Cruz");

    await user.type(screen.getByLabelText("Buscar"), "mateo");
    expect(screen.getByText("Mateo Sánchez")).toBeInTheDocument();
    expect(screen.queryByText("Valentina Cruz")).not.toBeInTheDocument();
  });

  it("opens the quick-add drawer and creates a student in one panel", async () => {
    const user = userEvent.setup();
    students.createStudent.mockResolvedValue(mk("9", "Nuevo", "Alumno", "pending"));
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByText("Valentina Cruz");

    // "Nuevo alumno" is both the header button and the FAB — either opens the drawer.
    const openButtons = screen.getAllByRole("button", { name: /Nuevo alumno/ });
    await user.click(openButtons[0] as HTMLElement);
    const dialog = await screen.findByRole("dialog", { name: "Nuevo alumno" });
    await user.type(within(dialog).getByPlaceholderText("XXXX000000XXXXXX00"), "HEGA050312MDFRNN08");
    await user.type(within(dialog).getByPlaceholderText("Ana Sofía"), "Nuevo");
    await user.type(within(dialog).getByPlaceholderText("Ruiz Márquez"), "Alumno");
    await user.type(within(dialog).getByPlaceholderText("Nombre del tutor"), "Tutor Nuevo");
    await user.click(within(dialog).getByRole("button", { name: "Guardar alumno" }));

    await waitFor(() =>
      expect(students.createStudent).toHaveBeenCalledWith(expect.any(String), {
        curp: "HEGA050312MDFRNN08",
        firstName: "Nuevo",
        lastName: "Alumno",
        level: "primaria",
        gradeGroup: "1° A",
        guardianName: "Tutor Nuevo",
      }),
    );
  });
});
