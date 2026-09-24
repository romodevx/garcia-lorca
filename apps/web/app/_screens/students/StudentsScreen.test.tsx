import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import * as configApi from "../../config/api";
import { StudentsScreen } from "./StudentsScreen";

vi.mock("../../students/api", async (importOriginal) => ({
  ...(await importOriginal<typeof studentsApi>()),
  listStudents: vi.fn(),
  createStudent: vi.fn(),
  enroll: vi.fn(),
  enrollIntoGrade: vi.fn(),
}));
vi.mock("../../config/api", () => ({
  listCycles: vi.fn(),
  listGroups: vi.fn(),
  listLevels: vi.fn(),
  listGradeLevels: vi.fn(),
}));

const students = vi.mocked(studentsApi);
const config = vi.mocked(configApi);

const mk = (id: string, first: string, last: string, status: studentsApi.AdmissionStatus): studentsApi.Student => ({
  id,
  curp: `CURP${id}`,
  firstName: first,
  lastName: last,
  level: "primaria",
  levelId: "",
  gradeGroup: "1° A",
  guardianName: "Ana Tutor",
  admissionStatus: status,
});

beforeEach(() => {
  students.listStudents.mockResolvedValue([mk("1", "Valentina", "Cruz", "enrolled"), mk("2", "Mateo", "Sánchez", "pending")]);
  config.listCycles.mockResolvedValue([{ id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" }]);
  config.listGroups.mockResolvedValue([]);
  config.listLevels.mockResolvedValue([
    { id: "lv-p", name: "Primaria", sepCode: "PRI", enrollmentGranularity: "cycle" },
  ]);
  config.listGradeLevels.mockResolvedValue([{ id: "gl-1", levelId: "lv-p", name: "1° Primaria", ordinal: 1 }]);
  students.enrollIntoGrade.mockResolvedValue({
    enrollment: { id: "e1", studentId: "9", cycleId: "c1", groupId: "grp-a", termId: "", status: "active", enrolledAt: "2026-08-31" },
    groupName: "1° A",
  });
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

  it("names each row's open button after the pupil it opens", async () => {
    const user = userEvent.setup();
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByText("Valentina Cruz");

    // Every row used to offer an identically-named "Ver ficha": a screen reader
    // announced the same thing five times over, and nothing could address one
    // row in particular. The pupil's name is what tells them apart.
    await user.click(screen.getByRole("button", { name: "Ver ficha de Mateo Sánchez" }));

    const card = await screen.findByRole("dialog", { name: "Ficha del alumno" });
    expect(within(card).getByText("Mateo Sánchez")).toBeInTheDocument();
  });

  it("filters by the search box", async () => {
    const user = userEvent.setup();
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByText("Valentina Cruz");

    await user.type(screen.getByLabelText("Buscar"), "mateo");
    expect(screen.getByText("Mateo Sánchez")).toBeInTheDocument();
    expect(screen.queryByText("Valentina Cruz")).not.toBeInTheDocument();
  });

  it("paginates the table: 20 per page, Siguiente reveals the rest, page size changes it", async () => {
    const user = userEvent.setup();
    const many = Array.from({ length: 25 }, (_, i) => mk(String(i + 10), `Nom${i}`, `Ape${String(i).padStart(2, "0")}`, "enrolled"));
    students.listStudents.mockResolvedValue(many);
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByText("Nom0 Ape00");

    // Sorted by surname Ape00..Ape24 → the last one is on page 2, hidden at first.
    expect(screen.queryByText("Nom24 Ape24")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Siguiente" }));
    expect(await screen.findByText("Nom24 Ape24")).toBeInTheDocument();

    // Page size 50 → all 25 on one page.
    await user.click(screen.getByRole("button", { name: "50" }));
    expect(screen.getByText("Nom24 Ape24")).toBeInTheDocument();
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
    // Por etiqueta y no por placeholder: el texto de reposo es decoración y
    // cambia con el diseño; la etiqueta es lo que nombra al campo.
    await user.type(within(dialog).getByLabelText("CURP"), "HEGA050312MDFRNN08");
    await user.type(within(dialog).getByLabelText("Nombre"), "Nuevo");
    await user.type(within(dialog).getByLabelText("Apellidos"), "Alumno");
    await user.type(within(dialog).getByLabelText("Tutor"), "Tutor Nuevo");
    await user.selectOptions(within(dialog).getByLabelText("Nivel"), "lv-p");
    await user.selectOptions(await within(dialog).findByLabelText("Grado"), "gl-1");
    await user.click(within(dialog).getByRole("button", { name: "Crear alumno" }));

    await waitFor(() =>
      // The level travels as the config level's ID, and gradeGroup is empty:
      // the server writes it when capacity chooses the group, so the record can
      // never claim a group the pupil is not in.
      expect(students.createStudent).toHaveBeenCalledWith(expect.any(String), {
        curp: "HEGA050312MDFRNN08",
        firstName: "NUEVO",
        lastName: "ALUMNO",
        levelId: "lv-p",
        gradeGroup: "",
        guardianName: "TUTOR NUEVO",
      }),
    );
  });
});

describe("StudentsScreen — por salón", () => {
  // The director counts heads per classroom before deciding to open another
  // group. It follows the filters, because that is the count they mean.
  it("counts pupils per classroom, and follows the filter", async () => {
    const user = userEvent.setup();
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByText("Valentina Cruz");

    const perGroup = await screen.findByTestId("per-group");
    expect(perGroup).toHaveTextContent("1° A");

    await user.type(screen.getByLabelText("Buscar"), "Valentina");

    expect(await screen.findByTestId("per-group")).toHaveTextContent("1");
  });
});

describe("StudentsScreen — la lista que no cargó", () => {
  // The same lie as the counter had: a roll that could not be read showed as
  // "Sin alumnos en esta vista", so the office was told there are no pupils
  // when the truth was that nobody could ask. It is the message that sent
  // somebody looking for a registration problem that did not exist.
  it("says it could not read the roll, instead of reporting no pupils", async () => {
    students.listStudents.mockRejectedValue(new Error("students is down"));

    render(<StudentsScreen onBack={() => {}} />);

    expect(await screen.findByRole("alert")).toHaveTextContent(/No pudimos cargar/i);
    expect(screen.queryByText(/Sin alumnos en esta vista/)).not.toBeInTheDocument();
  });

  it("can retry", async () => {
    const user = userEvent.setup();
    students.listStudents.mockRejectedValueOnce(new Error("students is down"));
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByRole("alert");

    students.listStudents.mockResolvedValue([mk("1", "Valentina", "Cruz", "enrolled")]);
    await user.click(screen.getByRole("button", { name: /Reintentar/ }));

    expect(await screen.findByText("Valentina Cruz")).toBeInTheDocument();
  });
});
