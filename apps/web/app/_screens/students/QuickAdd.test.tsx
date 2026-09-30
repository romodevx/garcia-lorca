import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import * as configApi from "../../config/api";
import { StudentsScreen, missingFieldMessage, createFailureMessage } from "./StudentsScreen";
import { StudentsError } from "../../students/api";

// The screens read labelled groups (ISSUE-002); label them from the listGroups mock.
beforeEach(() => {
  vi.mocked(configApi.listLabelledGroups).mockImplementation(async (...args) =>
    (await vi.mocked(configApi.listGroups)(...args)).map((g) => ({ ...g, label: g.name })));
});


/**
 * Pide el CAMPO y no su ayuda: el (?) de junto es un botón que se llama
 * "Ayuda sobre <campo>", así que una consulta laxa encuentra los dos.
 */
const CAMPO = { selector: "input, select, textarea" } as const;

vi.mock("../../students/api", async (importOriginal) => ({
  ...(await importOriginal<typeof studentsApi>()),
  listStudents: vi.fn(),
  createStudent: vi.fn(),
  enroll: vi.fn(),
  enrollIntoGrade: vi.fn(),
  addNote: vi.fn(),
}));
vi.mock("../../config/api", () => ({ listCycles: vi.fn(), listGroups: vi.fn(), listLabelledGroups: vi.fn(), listLevels: vi.fn(), listGradeLevels: vi.fn() }));

const students = vi.mocked(studentsApi);
const config = vi.mocked(configApi);

const LEVELS = [
  { id: "lv-k", name: "Kínder", sepCode: "KIN", enrollmentGranularity: "cycle" as const },
  // A level no fixed SEP list ever had, with no SEP key at all.
  { id: "lv-pre", name: "Preescolar", sepCode: "", enrollmentGranularity: "cycle" as const },
  { id: "lv-p", name: "Primaria", sepCode: "PRI", enrollmentGranularity: "cycle" as const },
];
const KINDER_GRADES = [
  { id: "gl-k1", levelId: "lv-k", name: "1° Kínder", ordinal: 1 },
  { id: "gl-k2", levelId: "lv-k", name: "2° Kínder", ordinal: 2 },
  { id: "gl-k3", levelId: "lv-k", name: "3° Kínder", ordinal: 3 },
];

beforeEach(() => {
  students.listStudents.mockResolvedValue([]);
  config.listCycles.mockResolvedValue([]);
  config.listGroups.mockResolvedValue([]);
  config.listLevels.mockResolvedValue(LEVELS);
  config.listGradeLevels.mockResolvedValue(KINDER_GRADES);
  students.enrollIntoGrade.mockResolvedValue({
    enrollment: { id: "e1", studentId: "s1", cycleId: "c1", groupId: "grp-b", termId: "", status: "active", enrolledAt: "2026-08-31" },
    groupName: "1° B",
  });
});
afterEach(() => vi.clearAllMocks());

const FULL = {
  curp: "ROQA930615HDFMRN08",
  firstName: "Andrés",
  lastName: "Romo",
  guardianName: "María Quiroz",
};

describe("what the form is still waiting for", () => {
  // The complaint that started this: the button sat greyed out and never said
  // which of four requirements was unmet. A control that refuses without a
  // reason is worse than one that refuses loudly.
  it("names the field that is missing, one at a time and in reading order", () => {
    expect(missingFieldMessage({ ...FULL, curp: "" })).toMatch(/CURP/);
    expect(missingFieldMessage({ ...FULL, curp: "ROQA930615HDFMRN0" })).toMatch(/18 caracteres.*17/);
    expect(missingFieldMessage({ ...FULL, firstName: "" })).toMatch(/nombre/i);
    expect(missingFieldMessage({ ...FULL, lastName: "" })).toMatch(/apellido paterno/i);
    // T3: the surname is read before the first name, so it is asked for first.
    expect(missingFieldMessage({ ...FULL, firstName: "", lastName: "" })).toMatch(/apellido paterno/i);
    // The one nobody guesses: a tutor of two letters silently failed the check.
    expect(missingFieldMessage({ ...FULL, guardianName: "Jo" })).toMatch(/tutor/i);
    expect(missingFieldMessage(FULL)).toBeUndefined();
  });
});

describe("what the server said, in words a person can act on", () => {
  it("tells a duplicate CURP apart from a malformed one", () => {
    expect(createFailureMessage(new StudentsError(409, ""))).toMatch(/ya (existe|está)/i);
    expect(createFailureMessage(new StudentsError(400, ""))).toMatch(/CURP/);
    // Anything else must not be blamed on the CURP, which is what it used to be.
    expect(createFailureMessage(new StudentsError(500, ""))).not.toMatch(/CURP/);
  });
});

describe("the quick-add drawer", () => {
  it("says what is missing instead of leaving a dead button", async () => {
    const user = userEvent.setup();
    render(<StudentsScreen onBack={() => {}} />);
    await user.click((await screen.findAllByRole("button", { name: "Nuevo alumno" }))[0]!);

    const drawer = screen.getByRole("dialog", { name: "Nuevo alumno" });
    await user.type(within(drawer).getByLabelText(/CURP/, CAMPO), FULL.curp);
    await user.type(within(drawer).getByLabelText(/Nombre\(s\)/, CAMPO), FULL.firstName);
    await user.type(within(drawer).getByLabelText(/Apellido paterno/, CAMPO), FULL.lastName);
    // Tutor deliberately left blank.

    await user.click(within(drawer).getByRole("button", { name: "Crear alumno" }));

    // Ya no es UNA alerta arriba: cada campo que falta lleva la suya, así que
    // se busca la del tutor en vez de "la" alerta del cajón.
    expect(within(drawer).getByLabelText("Tutor")).toHaveAttribute("aria-invalid", "true");
    expect(within(drawer).getAllByRole("alert").map((a) => a.textContent).join(" ")).toMatch(/tutor/i);
    expect(students.createStudent).not.toHaveBeenCalled();
  });

  /**
   * Regresión del 23-sep-2026, medida en el navegador antes de existir esta
   * prueba: al guardar vacío salían CINCO errores y sólo se veían CUATRO. El de
   * "Tutor" vivía en y=629 mientras el cuerpo del cajón terminaba en y=576, que
   * es donde empieza el pie pegajoso; la forma no se desplazaba (scrollTop en 0)
   * y el foco se quedaba en el BOTÓN. Se corregían los cuatro visibles, se
   * volvía a guardar, y seguía fallando por una razón invisible.
   */
  it("manda el foco al primer campo que falta, no lo deja en el botón", async () => {
    const user = userEvent.setup();
    render(<StudentsScreen onBack={() => {}} />);
    await user.click((await screen.findAllByRole("button", { name: "Nuevo alumno" }))[0]!);

    const drawer = screen.getByRole("dialog", { name: "Nuevo alumno" });
    const guardar = within(drawer).getByRole("button", { name: "Crear alumno" });
    await user.click(guardar);

    expect(within(drawer).getByLabelText("CURP")).toHaveFocus();
    expect(guardar).not.toHaveFocus();
  });

  it("desplaza ese campo hasta el centro, para que el pie no lo tape", async () => {
    const user = userEvent.setup();
    const scrollIntoView = vi.fn();
    Object.defineProperty(HTMLElement.prototype, "scrollIntoView", {
      value: scrollIntoView,
      writable: true,
      configurable: true,
    });
    render(<StudentsScreen onBack={() => {}} />);
    await user.click((await screen.findAllByRole("button", { name: "Nuevo alumno" }))[0]!);

    const drawer = screen.getByRole("dialog", { name: "Nuevo alumno" });
    await user.click(within(drawer).getByRole("button", { name: "Crear alumno" }));

    expect(scrollIntoView).toHaveBeenCalledWith(expect.objectContaining({ block: "center" }));
  });

  it("creates the pupil once every field is there", async () => {
    const user = userEvent.setup();
    students.createStudent.mockResolvedValue({
      id: "s1", curp: FULL.curp, firstName: FULL.firstName, lastName: FULL.lastName,
      level: "primaria", levelId: "", gradeGroup: "1° A", guardianName: FULL.guardianName,
      admissionStatus: "pending",
    });
    render(<StudentsScreen onBack={() => {}} />);
    await user.click((await screen.findAllByRole("button", { name: "Nuevo alumno" }))[0]!);

    const drawer = screen.getByRole("dialog", { name: "Nuevo alumno" });
    await user.type(within(drawer).getByLabelText(/CURP/, CAMPO), FULL.curp);
    await user.type(within(drawer).getByLabelText(/Nombre\(s\)/, CAMPO), FULL.firstName);
    await user.type(within(drawer).getByLabelText(/Apellido paterno/, CAMPO), FULL.lastName);
    await user.type(within(drawer).getByLabelText(/Tutor/, CAMPO), FULL.guardianName);
    await user.selectOptions(within(drawer).getByLabelText("Nivel"), "lv-k");
    await user.selectOptions(await within(drawer).findByLabelText("Grado"), "gl-k1");
    await user.click(within(drawer).getByRole("button", { name: "Crear alumno" }));

    await waitFor(() => expect(students.createStudent).toHaveBeenCalled());
    expect(students.createStudent.mock.calls[0]?.[1]).toMatchObject({ curp: FULL.curp });
  });

  it("sends the maternal surname on its own (T3)", async () => {
    const user = userEvent.setup();
    students.createStudent.mockResolvedValue({
      id: "s1", curp: FULL.curp, firstName: FULL.firstName, lastName: FULL.lastName, maternalLastName: "Quiroz",
      level: "primaria", levelId: "", gradeGroup: "1° A", guardianName: FULL.guardianName,
      admissionStatus: "pending",
    });
    render(<StudentsScreen onBack={() => {}} />);
    await user.click((await screen.findAllByRole("button", { name: "Nuevo alumno" }))[0]!);

    const drawer = screen.getByRole("dialog", { name: "Nuevo alumno" });
    await user.type(within(drawer).getByLabelText(/CURP/, CAMPO), FULL.curp);
    await user.type(within(drawer).getByLabelText(/Apellido paterno/, CAMPO), FULL.lastName);
    await user.type(within(drawer).getByLabelText(/Apellido materno/, CAMPO), "Quiroz");
    await user.type(within(drawer).getByLabelText(/Nombre\(s\)/, CAMPO), FULL.firstName);
    await user.type(within(drawer).getByLabelText(/Tutor/, CAMPO), FULL.guardianName);
    await user.selectOptions(within(drawer).getByLabelText("Nivel"), "lv-k");
    await user.selectOptions(await within(drawer).findByLabelText("Grado"), "gl-k1");
    await user.click(within(drawer).getByRole("button", { name: "Crear alumno" }));

    await waitFor(() => expect(students.createStudent).toHaveBeenCalled());
    expect(students.createStudent.mock.calls[0]?.[1]).toMatchObject({ lastName: "ROMO", maternalLastName: "QUIROZ", firstName: "ANDRÉS" });
  });

  // The fixed SEP list is gone (task 2): any level the school created in
  // Configuración takes pupils, and it travels by id, not by name.
  it("places a pupil in any level the school defined, Preescolar included", async () => {
    const user = userEvent.setup();
    students.createStudent.mockResolvedValue({
      id: "s1", curp: FULL.curp, firstName: FULL.firstName, lastName: FULL.lastName,
      level: "Preescolar", levelId: "lv-pre", gradeGroup: "", guardianName: FULL.guardianName,
      admissionStatus: "pending",
    });
    render(<StudentsScreen onBack={() => {}} />);
    await user.click((await screen.findAllByRole("button", { name: "Nuevo alumno" }))[0]!);

    const drawer = screen.getByRole("dialog", { name: "Nuevo alumno" });
    await user.type(within(drawer).getByLabelText(/CURP/, CAMPO), FULL.curp);
    await user.type(within(drawer).getByLabelText(/Nombre\(s\)/, CAMPO), FULL.firstName);
    await user.type(within(drawer).getByLabelText(/Apellido paterno/, CAMPO), FULL.lastName);
    await user.type(within(drawer).getByLabelText(/Tutor/, CAMPO), FULL.guardianName);
    await user.selectOptions(within(drawer).getByLabelText("Nivel"), "lv-pre");
    await user.selectOptions(await within(drawer).findByLabelText("Grado"), "gl-k1");
    await user.click(within(drawer).getByRole("button", { name: "Crear alumno" }));

    await waitFor(() => expect(students.createStudent).toHaveBeenCalled());
    expect(students.createStudent.mock.calls[0]?.[1]).toMatchObject({ levelId: "lv-pre" });
    expect(within(drawer).queryByText(/SEP/)).not.toBeInTheDocument();
  });

  it("says a CURP is taken when the server says so", async () => {
    const user = userEvent.setup();
    students.createStudent.mockRejectedValue(new StudentsError(409, ""));
    render(<StudentsScreen onBack={() => {}} />);
    await user.click((await screen.findAllByRole("button", { name: "Nuevo alumno" }))[0]!);

    const drawer = screen.getByRole("dialog", { name: "Nuevo alumno" });
    await user.type(within(drawer).getByLabelText(/CURP/, CAMPO), FULL.curp);
    await user.type(within(drawer).getByLabelText(/Nombre\(s\)/, CAMPO), FULL.firstName);
    await user.type(within(drawer).getByLabelText(/Apellido paterno/, CAMPO), FULL.lastName);
    await user.type(within(drawer).getByLabelText(/Tutor/, CAMPO), FULL.guardianName);
    await user.selectOptions(within(drawer).getByLabelText("Nivel"), "lv-k");
    await user.selectOptions(await within(drawer).findByLabelText("Grado"), "gl-k1");
    await user.click(within(drawer).getByRole("button", { name: "Crear alumno" }));

    expect(await within(drawer).findByRole("alert")).toHaveTextContent(/ya (existe|está)/i);
  });
});

describe("choosing where the pupil goes", () => {
  const openDrawer = async (user: ReturnType<typeof userEvent.setup>) => {
    render(<StudentsScreen onBack={() => {}} />);
    await user.click((await screen.findAllByRole("button", { name: "Nuevo alumno" }))[0]!);
    return screen.getByRole("dialog", { name: "Nuevo alumno" });
  };

  const fillIdentity = async (user: ReturnType<typeof userEvent.setup>, drawer: HTMLElement) => {
    await user.type(within(drawer).getByLabelText(/CURP/, CAMPO), FULL.curp);
    await user.type(within(drawer).getByLabelText(/Nombre\(s\)/, CAMPO), FULL.firstName);
    await user.type(within(drawer).getByLabelText(/Apellido paterno/, CAMPO), FULL.lastName);
    await user.type(within(drawer).getByLabelText(/Tutor/, CAMPO), FULL.guardianName);
  };

  // The grade list is not a fixed 1-2-3: it is whatever the school configured for
  // that level. Kínder has three, primaria six, and a school that adds a fourth
  // kínder grade must see it here without anybody editing this screen.
  it("offers only the grades the chosen level actually has", async () => {
    const user = userEvent.setup();
    const drawer = await openDrawer(user);

    await user.selectOptions(within(drawer).getByLabelText("Nivel"), "lv-k");

    const grade = await within(drawer).findByLabelText("Grado");
    await waitFor(() => expect(within(grade).getAllByRole("option")).toHaveLength(KINDER_GRADES.length + 1));
    expect(config.listGradeLevels).toHaveBeenCalledWith(expect.any(String), "lv-k");
  });

  // The group is NOT asked for. It is a consequence of how full each one is, and
  // asking a person to type it is how "1° A" ended up in records for a group that
  // was full or had never been created.
  it("never asks for the group", async () => {
    const user = userEvent.setup();
    const drawer = await openDrawer(user);

    expect(within(drawer).queryByLabelText("Grado y grupo")).toBeNull();
    expect(within(drawer).getByText(/se asigna solo/i)).toBeInTheDocument();
  });

  it("creates the pupil and places them, then says which group they landed in", async () => {
    const user = userEvent.setup();
    students.createStudent.mockResolvedValue({
      id: "s1", curp: FULL.curp, firstName: FULL.firstName, lastName: FULL.lastName,
      level: "kinder", levelId: "", gradeGroup: "", guardianName: FULL.guardianName, admissionStatus: "pending",
    });
    const drawer = await openDrawer(user);
    await fillIdentity(user, drawer);
    await user.selectOptions(within(drawer).getByLabelText("Nivel"), "lv-k");
    await user.selectOptions(await within(drawer).findByLabelText("Grado"), "gl-k2");

    await user.click(within(drawer).getByRole("button", { name: "Crear alumno" }));

    await waitFor(() => expect(students.enrollIntoGrade).toHaveBeenCalledWith(expect.any(String), "s1", "gl-k2"));
    // The level travels as the config level's ID; the server supplies its name.
    expect(students.createStudent.mock.calls[0]?.[1]).toMatchObject({ levelId: "lv-k" });
    expect(await screen.findByText(/1° B/)).toBeInTheDocument();
  });

  it("will not create a pupil with nowhere to put them", async () => {
    const user = userEvent.setup();
    const drawer = await openDrawer(user);
    await fillIdentity(user, drawer);
    await user.selectOptions(within(drawer).getByLabelText("Nivel"), "lv-k");
    // Grade deliberately not chosen.

    await user.click(within(drawer).getByRole("button", { name: "Crear alumno" }));

    expect(await within(drawer).findByRole("alert")).toHaveTextContent(/grado/i);
    expect(students.createStudent).not.toHaveBeenCalled();
  });

  // A full grade is the school's decision to make. The pupil exists by then, so
  // the message has to say that too — otherwise somebody re-types the CURP and
  // gets "ya existe".
  it("says the grade is full, and that the pupil was still created", async () => {
    const user = userEvent.setup();
    students.createStudent.mockResolvedValue({
      id: "s1", curp: FULL.curp, firstName: FULL.firstName, lastName: FULL.lastName,
      level: "kinder", levelId: "", gradeGroup: "", guardianName: FULL.guardianName, admissionStatus: "pending",
    });
    students.enrollIntoGrade.mockRejectedValue(new StudentsError(409, "no quedan lugares"));
    const drawer = await openDrawer(user);
    await fillIdentity(user, drawer);
    await user.selectOptions(within(drawer).getByLabelText("Nivel"), "lv-k");
    await user.selectOptions(await within(drawer).findByLabelText("Grado"), "gl-k1");

    await user.click(within(drawer).getByRole("button", { name: "Crear alumno" }));

    const alert = await within(drawer).findByRole("alert");
    expect(alert).toHaveTextContent(/lugares|lleno/i);
    expect(alert).toHaveTextContent(/se creó|creado/i);
  });
});

describe("any level the school defined", () => {
  // The fixed SEP list is gone (task 2): a level only this school has —
  // "Universidad" stood in for "a level the system cannot hold" — now takes
  // pupils like any other, and nothing tells the user to rename it.
  it("sends a pupil to a level no fixed list ever had", async () => {
    const user = userEvent.setup();
    students.createStudent.mockResolvedValue({
      id: "s1", curp: FULL.curp, firstName: FULL.firstName, lastName: FULL.lastName,
      level: "Universidad", levelId: "lv-m", gradeGroup: "", guardianName: FULL.guardianName, admissionStatus: "pending",
    });
    config.listLevels.mockResolvedValue([
      { id: "lv-m", name: "Universidad", sepCode: "", enrollmentGranularity: "cycle" as const },
    ]);
    config.listGradeLevels.mockResolvedValue([{ id: "gl-m1", levelId: "lv-m", name: "Primer semestre", ordinal: 1 }]);
    render(<StudentsScreen onBack={() => {}} />);
    await user.click((await screen.findAllByRole("button", { name: "Nuevo alumno" }))[0]!);

    const drawer = screen.getByRole("dialog", { name: "Nuevo alumno" });
    await user.type(within(drawer).getByLabelText(/CURP/, CAMPO), FULL.curp);
    await user.type(within(drawer).getByLabelText(/Nombre\(s\)/, CAMPO), FULL.firstName);
    await user.type(within(drawer).getByLabelText(/Apellido paterno/, CAMPO), FULL.lastName);
    await user.type(within(drawer).getByLabelText(/Tutor/, CAMPO), FULL.guardianName);
    await user.selectOptions(within(drawer).getByLabelText("Nivel"), "lv-m");
    await user.selectOptions(await within(drawer).findByLabelText("Grado"), "gl-m1");

    await user.click(within(drawer).getByRole("button", { name: "Crear alumno" }));

    await waitFor(() => expect(students.createStudent).toHaveBeenCalled());
    expect(students.createStudent.mock.calls[0]?.[1]).toMatchObject({ levelId: "lv-m" });
  });
});

describe("la captura del alta de alumno", () => {
  async function abrirAlta(user: ReturnType<typeof userEvent.setup>): Promise<HTMLElement> {
    render(<StudentsScreen onBack={() => {}} />);
    // Son dos: el del encabezado y el flotante.
    await user.click((await screen.findAllByRole("button", { name: "Nuevo alumno" }))[0]!);
    return screen.getByRole("dialog", { name: "Nuevo alumno" });
  }

  it("el nombre, los apellidos, el CURP y el tutor se capturan en mayúsculas", async () => {
    const user = userEvent.setup();
    const alta = await abrirAlta(user);

    await user.type(within(alta).getByLabelText("Nombre(s)"), "ana sofía");
    await user.type(within(alta).getByLabelText("Apellido paterno"), "ruiz");
    await user.type(within(alta).getByLabelText("Apellido materno"), "márquez");
    await user.type(within(alta).getByLabelText("Tutor"), "josé ruiz");

    expect(within(alta).getByLabelText("Nombre(s)")).toHaveValue("ANA SOFÍA");
    expect(within(alta).getByLabelText("Apellido paterno")).toHaveValue("RUIZ");
    expect(within(alta).getByLabelText("Apellido materno")).toHaveValue("MÁRQUEZ");
    expect(within(alta).getByLabelText("Tutor")).toHaveValue("JOSÉ RUIZ");
  });

  it("pide los apellidos ANTES que el nombre (T3)", async () => {
    const user = userEvent.setup();
    const alta = await abrirAlta(user);
    const paterno = within(alta).getByLabelText("Apellido paterno");
    const materno = within(alta).getByLabelText("Apellido materno");
    const nombre = within(alta).getByLabelText("Nombre(s)");
    const follows = (a: Element, b: Element): boolean => (a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;
    expect(follows(paterno, materno)).toBe(true);
    expect(follows(materno, nombre)).toBe(true);
    // The maternal surname is optional: not every pupil has one on file.
    await user.click(within(alta).getByRole("button", { name: "Crear alumno" }));
    expect(materno).not.toHaveAttribute("aria-invalid", "true");
  });

  it("guardar en blanco marca CADA campo que falta, no uno solo", async () => {
    // La validación ya daba un mensaje por campo, pero uno a la vez y en una
    // sola frase arriba: había que guardar cinco veces para descubrir los cinco.
    const user = userEvent.setup();
    const alta = await abrirAlta(user);

    await user.click(within(alta).getByRole("button", { name: "Crear alumno" }));

    expect(within(alta).getByLabelText("CURP")).toHaveAttribute("aria-invalid", "true");
    expect(within(alta).getByLabelText("Nombre(s)")).toHaveAttribute("aria-invalid", "true");
    expect(within(alta).getByLabelText("Apellido paterno")).toHaveAttribute("aria-invalid", "true");
    expect(within(alta).getByLabelText("Tutor")).toHaveAttribute("aria-invalid", "true");
    expect(students.createStudent).not.toHaveBeenCalled();
  });

  it("el rojo de un campo se limpia al escribir en él, y sólo en él", async () => {
    const user = userEvent.setup();
    const alta = await abrirAlta(user);
    await user.click(within(alta).getByRole("button", { name: "Crear alumno" }));

    await user.type(within(alta).getByLabelText("Nombre(s)"), "A");

    expect(within(alta).getByLabelText("Nombre(s)")).not.toHaveAttribute("aria-invalid", "true");
    expect(within(alta).getByLabelText("CURP")).toHaveAttribute("aria-invalid", "true");
  });

  it("conserva el mensaje que explica POR QUÉ falla, no sólo que falta", async () => {
    // Un CURP de 17 caracteres no está "vacío": el mensaje tiene que decir que
    // faltó uno, que es lo que esta pantalla ya sabía decir.
    const user = userEvent.setup();
    const alta = await abrirAlta(user);

    await user.type(within(alta).getByLabelText("CURP"), "ROQA930615HDFMRN0");
    await user.click(within(alta).getByRole("button", { name: "Crear alumno" }));

    expect(within(alta).getByText(/18 caracteres.*17/)).toBeInTheDocument();
  });
});

describe("QuickAdd · una nota al dar de alta (tarea 6)", () => {
  const created = {
    id: "s1", curp: FULL.curp, firstName: FULL.firstName, lastName: FULL.lastName,
    level: "kinder", levelId: "", gradeGroup: "", guardianName: FULL.guardianName, admissionStatus: "pending" as const,
  };

  const openWith = async (user: ReturnType<typeof userEvent.setup>, canWriteNotes: boolean) => {
    render(<StudentsScreen onBack={() => {}} canWriteNotes={canWriteNotes} />);
    await user.click((await screen.findAllByRole("button", { name: "Nuevo alumno" }))[0]!);
    const drawer = screen.getByRole("dialog", { name: "Nuevo alumno" });
    await user.type(within(drawer).getByLabelText(/CURP/, CAMPO), FULL.curp);
    await user.type(within(drawer).getByLabelText(/Nombre\(s\)/, CAMPO), FULL.firstName);
    await user.type(within(drawer).getByLabelText(/Apellido paterno/, CAMPO), FULL.lastName);
    await user.type(within(drawer).getByLabelText(/Tutor/, CAMPO), FULL.guardianName);
    await user.selectOptions(within(drawer).getByLabelText("Nivel"), "lv-k");
    await user.selectOptions(await within(drawer).findByLabelText("Grado"), "gl-k2");
    return drawer;
  };

  it("saves the note on the new pupil", async () => {
    const user = userEvent.setup();
    students.createStudent.mockResolvedValue(created);
    students.addNote.mockResolvedValue({ id: "n-1", studentId: "s1", body: "Alergia al polvo", authorUserId: "u", authorName: "", createdAt: "", voided: false });
    const drawer = await openWith(user, true);
    await user.type(within(drawer).getByLabelText(/Nota/, CAMPO), "Alergia al polvo");
    await user.click(within(drawer).getByRole("button", { name: "Crear alumno" }));

    await waitFor(() => expect(students.addNote).toHaveBeenCalledWith(expect.any(String), "s1", "Alergia al polvo"));
    await waitFor(() => expect(students.enrollIntoGrade).toHaveBeenCalled());
  });

  it("sends no note when the field was left empty", async () => {
    const user = userEvent.setup();
    students.createStudent.mockResolvedValue(created);
    const drawer = await openWith(user, true);
    await user.click(within(drawer).getByRole("button", { name: "Crear alumno" }));
    await waitFor(() => expect(students.enrollIntoGrade).toHaveBeenCalled());
    expect(students.addNote).not.toHaveBeenCalled();
  });

  it("offers no note to whoever may not write one", async () => {
    const user = userEvent.setup();
    const drawer = await openWith(user, false);
    expect(within(drawer).queryByLabelText(/Nota/, CAMPO)).not.toBeInTheDocument();
  });

  // The pupil exists by then: a note that failed must not read as "nothing saved".
  it("says the pupil was created when only the note failed", async () => {
    const user = userEvent.setup();
    students.createStudent.mockResolvedValue(created);
    students.addNote.mockRejectedValue(new StudentsError(500, "boom"));
    const drawer = await openWith(user, true);
    await user.type(within(drawer).getByLabelText(/Nota/, CAMPO), "Alergia al polvo");
    await user.click(within(drawer).getByRole("button", { name: "Crear alumno" }));

    expect(await screen.findByText(/la nota no se guardó/i)).toBeInTheDocument();
    expect(students.enrollIntoGrade).toHaveBeenCalled();
  });

  // Both failed: the drawer stays open on the placement, and a retry cannot
  // write the note (the CURP is taken). The note must be named there too.
  it("names the lost note when the placement also failed", async () => {
    const user = userEvent.setup();
    students.createStudent.mockResolvedValue(created);
    students.addNote.mockRejectedValue(new StudentsError(500, "boom"));
    students.enrollIntoGrade.mockRejectedValue(new StudentsError(409, "the grade is full"));
    const drawer = await openWith(user, true);
    await user.type(within(drawer).getByLabelText(/Nota/, CAMPO), "Alergia al polvo");
    await user.click(within(drawer).getByRole("button", { name: "Crear alumno" }));

    expect(await within(drawer).findByRole("alert")).toHaveTextContent(/la nota no se guardó/i);
  });
});
