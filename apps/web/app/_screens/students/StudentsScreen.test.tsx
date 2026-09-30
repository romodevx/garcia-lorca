import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import * as configApi from "../../config/api";
import { StudentsScreen } from "./StudentsScreen";

// The screens read labelled groups (ISSUE-002); label them from the listGroups mock.
beforeEach(() => {
  vi.mocked(configApi.listLabelledGroups).mockImplementation(async (...args) =>
    (await vi.mocked(configApi.listGroups)(...args)).map((g) => ({ ...g, label: g.name })));
});


vi.mock("../../students/api", async (importOriginal) => ({
  ...(await importOriginal<typeof studentsApi>()),
  listStudents: vi.fn(),
  createStudent: vi.fn(),
  enroll: vi.fn(),
  enrollIntoGrade: vi.fn(),
}));
vi.mock("../../config/api", () => ({
  listCycles: vi.fn(),
  listGroups: vi.fn(), listLabelledGroups: vi.fn(),
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
    expect(await screen.findByText("Cruz Valentina")).toBeInTheDocument();
    expect(screen.getByText("Sánchez Mateo")).toBeInTheDocument();
    // status pills reflect admission status ("Inscrito" pill is unique; "Trámite"
    // also names a tab, so assert at least one pill exists).
    expect(screen.getByText("Inscrito")).toBeInTheDocument();
    expect(screen.getAllByText("Trámite").length).toBeGreaterThan(0);
  });

  // QA 2026-09-25, ISSUE-005: the floating "Nuevo alumno" sat on top of the
  // pagination's "Siguiente". The page ends with room for it, so the last row
  // and the pager scroll clear of the button.
  it("leaves room under the table for the floating button", async () => {
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByText("Cruz Valentina");

    const fab = screen.getByTitle("Nuevo alumno");
    const room = screen.getByTestId("fab-clearance");
    const footprint = parseInt(fab.style.height, 10) + parseInt(fab.style.bottom, 10);
    expect(parseInt(room.style.height, 10)).toBeGreaterThan(footprint);
  });

  it("names each row's open button after the pupil it opens", async () => {
    const user = userEvent.setup();
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByText("Cruz Valentina");

    // Every row used to offer an identically-named "Ver ficha": a screen reader
    // announced the same thing five times over, and nothing could address one
    // row in particular. The pupil's name is what tells them apart.
    await user.click(screen.getByRole("button", { name: "Ver ficha de Sánchez Mateo" }));

    const card = await screen.findByRole("dialog", { name: "Ficha del alumno" });
    expect(within(card).getByText("Sánchez Mateo")).toBeInTheDocument();
  });

  it("filters by the search box", async () => {
    const user = userEvent.setup();
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByText("Cruz Valentina");

    await user.type(screen.getByLabelText("Buscar"), "mateo");
    expect(screen.getByText("Sánchez Mateo")).toBeInTheDocument();
    expect(screen.queryByText("Cruz Valentina")).not.toBeInTheDocument();
  });

  // Review of T3: the list reads surname first, but the office still types
  // names the way it says them ("mateo sánchez").
  it("finds a pupil typed first name first, too", async () => {
    const user = userEvent.setup();
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByText("Cruz Valentina");

    await user.type(screen.getByLabelText("Buscar"), "mateo sanchez");
    expect(screen.getByText("Sánchez Mateo")).toBeInTheDocument();
    expect(screen.queryByText("Cruz Valentina")).not.toBeInTheDocument();
  });

  it("shows each pupil surname first and sorts by surname, accents aside (T3)", async () => {
    students.listStudents.mockResolvedValue([
      { ...mk("1", "Zoe", "Ñuñez", "enrolled") },
      { ...mk("2", "Beto", "Ávila", "enrolled"), maternalLastName: "Ruiz" },
      { ...mk("3", "Ana", "avila", "enrolled"), maternalLastName: "López" },
      { ...mk("4", "Carla", "Nuñez", "enrolled") },
    ]);
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByText("Ávila Ruiz Beto");
    const opens = screen.getAllByRole("button", { name: /^Ver ficha de / }).map((b) => b.getAttribute("aria-label"));
    expect(opens).toEqual([
      "Ver ficha de avila López Ana",
      "Ver ficha de Ávila Ruiz Beto",
      "Ver ficha de Nuñez Carla",
      "Ver ficha de Ñuñez Zoe",
    ]);
  });

  it("paginates the table: 20 per page, Siguiente reveals the rest, page size changes it", async () => {
    const user = userEvent.setup();
    const many = Array.from({ length: 25 }, (_, i) => mk(String(i + 10), `Nom${i}`, `Ape${String(i).padStart(2, "0")}`, "enrolled"));
    students.listStudents.mockResolvedValue(many);
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByText("Ape00 Nom0");

    // Sorted by surname Ape00..Ape24 → the last one is on page 2, hidden at first.
    expect(screen.queryByText("Ape24 Nom24")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Siguiente" }));
    expect(await screen.findByText("Ape24 Nom24")).toBeInTheDocument();

    // Page size 50 → all 25 on one page.
    await user.click(screen.getByRole("button", { name: "50" }));
    expect(screen.getByText("Ape24 Nom24")).toBeInTheDocument();
  });

  it("opens the quick-add drawer and creates a student in one panel", async () => {
    const user = userEvent.setup();
    students.createStudent.mockResolvedValue(mk("9", "Nuevo", "Alumno", "pending"));
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByText("Cruz Valentina");

    // "Nuevo alumno" is both the header button and the FAB — either opens the drawer.
    const openButtons = screen.getAllByRole("button", { name: /Nuevo alumno/ });
    await user.click(openButtons[0] as HTMLElement);
    const dialog = await screen.findByRole("dialog", { name: "Nuevo alumno" });
    // Por etiqueta y no por placeholder: el texto de reposo es decoración y
    // cambia con el diseño; la etiqueta es lo que nombra al campo.
    await user.type(within(dialog).getByLabelText("CURP"), "HEGA050312MDFRNN08");
    await user.type(within(dialog).getByLabelText("Apellido paterno"), "Alumno");
    await user.type(within(dialog).getByLabelText("Nombre(s)"), "Nuevo");
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
        maternalLastName: "",
        levelId: "lv-p",
        gradeGroup: "",
        guardianName: "TUTOR NUEVO",
      }),
    );
  });
});

describe("StudentsScreen — editar la ficha (T5)", () => {
  it("opens the edit page from the row", async () => {
    const user = userEvent.setup();
    const onEdit = vi.fn();
    render(<StudentsScreen onBack={() => {}} onEdit={onEdit} />);
    await user.click(await screen.findByRole("button", { name: "Editar ficha de Sánchez Mateo" }));
    expect(onEdit).toHaveBeenCalledWith("2");
  });

  it("opens the edit page from the sheet", async () => {
    const user = userEvent.setup();
    const onEdit = vi.fn();
    render(<StudentsScreen onBack={() => {}} onEdit={onEdit} />);
    await user.click(await screen.findByRole("button", { name: "Ver ficha de Cruz Valentina" }));
    const card = await screen.findByRole("dialog", { name: "Ficha del alumno" });
    await user.click(within(card).getByRole("button", { name: "Editar ficha" }));
    expect(onEdit).toHaveBeenCalledWith("1");
  });

  it("offers no edit button when nobody can edit", async () => {
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByText("Cruz Valentina");
    expect(screen.queryByRole("button", { name: "Editar ficha de Cruz Valentina" })).not.toBeInTheDocument();
  });
});

describe("StudentsScreen — por salón", () => {
  // The director counts heads per classroom before deciding to open another
  // group. It follows the filters, because that is the count they mean.
  it("counts pupils per classroom, and follows the filter", async () => {
    const user = userEvent.setup();
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByText("Cruz Valentina");

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

    expect(await screen.findByText("Cruz Valentina")).toBeInTheDocument();
  });
});
