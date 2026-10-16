import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import * as configApi from "../../config/api";
import { StudentsError } from "../../students/api";
import { StudentAltaPage } from "./StudentAltaPage";

/**
 * T9 / ISSUE-009: the alta chooses the GROUP (automatic by default) and the
 * ENTRY DATE (today by default). Valentina goes straight into 1° B; a pupil who
 * entered on 24-Aug is billed from September — the server bills from the date
 * it is given, so the page's job is to give it, and only when it is not today.
 */

vi.mock("../../students/api", async (importOriginal) => ({
  ...(await importOriginal<typeof studentsApi>()),
  listStudents: vi.fn(),
  listEnrollments: vi.fn(),
  createStudent: vi.fn(),
  enroll: vi.fn(),
  enrollIntoGrade: vi.fn(),
  addNote: vi.fn(),
}));
vi.mock("../../config/api", () => ({ listCycles: vi.fn(), listGroups: vi.fn(), listLabelledGroups: vi.fn(), listLevels: vi.fn(), listGradeLevels: vi.fn() }));

/** Every alta test lands on the page itself: /alumnos/nuevo. */
const onCreated = vi.fn();
const onBack = vi.fn();
async function openAlta(props: { canWriteNotes?: boolean } = {}): Promise<HTMLElement> {
  render(<StudentAltaPage onBack={onBack} onCreated={onCreated} {...props} />);
  await screen.findByRole("heading", { level: 1, name: "Nuevo alumno" });
  return document.body;
}

const students = vi.mocked(studentsApi);
const config = vi.mocked(configApi);

const CAMPO = { selector: "input, select, textarea" } as const;
const LEVELS = [{ id: "lv-p", name: "Primaria", sepCode: "PRI", enrollmentGranularity: "cycle" as const }];
const GRADES = [
  { id: "gl-1", levelId: "lv-p", name: "1°", ordinal: 1 },
  { id: "gl-2", levelId: "lv-p", name: "2°", ordinal: 2 },
];
const group = (id: string, gradeLevelId: string, name: string, capacity: number) => ({
  id, cycleId: "c1", gradeLevelId, name, capacity, label: `${gradeLevelId === "gl-1" ? "1°" : "2°"} PRIMARIA · ${name}`,
});
// Out of order on purpose: the picker orders them by name. 1° C is not sized yet.
const GROUPS = [group("grp-1b", "gl-1", "B", 30), group("grp-2a", "gl-2", "A", 28), group("grp-1a", "gl-1", "A", 25), group("grp-1c", "gl-1", "C", 0)];
const CREATED = {
  id: "s1", curp: "ROQA930615HDFMRN08", firstName: "VALENTINA", lastName: "ROMO", maternalLastName: "",
  level: "", levelId: "lv-p", gradeGroup: "", guardianName: "MARÍA QUIROZ", admissionStatus: "pending" as const,
};
const ENROLLMENT = { id: "e1", studentId: "s1", cycleId: "c1", groupId: "grp-1b", termId: "", status: "active" as const, enrolledAt: "" };

// 12:00 in Morelia on 16-Oct-2026. Only Date is faked: userEvent keeps its timers.
const NOON_IN_MORELIA = new Date("2026-10-16T18:00:00Z");

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true, toFake: ["Date"] });
  vi.setSystemTime(NOON_IN_MORELIA);
  students.listStudents.mockResolvedValue([]);
  students.listEnrollments.mockResolvedValue([]);
  students.createStudent.mockResolvedValue(CREATED);
  students.enrollIntoGrade.mockResolvedValue({ enrollment: ENROLLMENT, groupName: "1° A" });
  students.enroll.mockResolvedValue(ENROLLMENT);
  config.listCycles.mockResolvedValue([{ id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-24", endsOn: "2027-07-09" }]);
  config.listLevels.mockResolvedValue(LEVELS);
  config.listGradeLevels.mockResolvedValue(GRADES);
  config.listLabelledGroups.mockResolvedValue(GROUPS);
});
afterEach(() => {
  vi.useRealTimers();
  vi.clearAllMocks();
});

type User = ReturnType<typeof userEvent.setup>;

async function openFilledDrawer(user: User, gradeId = "gl-1"): Promise<HTMLElement> {
  const drawer = await openAlta();
  await waitFor(() => expect(config.listLabelledGroups).toHaveBeenCalledWith(expect.any(String), "c1"));
  await user.type(within(drawer).getByLabelText(/CURP/, CAMPO), CREATED.curp);
  await user.type(within(drawer).getByLabelText(/Apellido paterno/, CAMPO), "Romo");
  await user.type(within(drawer).getByLabelText(/Nombre\(s\)/, CAMPO), "Valentina");
  await user.type(within(drawer).getByLabelText(/Tutor/, CAMPO), "María Quiroz");
  await user.selectOptions(within(drawer).getByLabelText("Nivel", CAMPO), "lv-p");
  const grade = within(drawer).getByLabelText("Grado", CAMPO);
  await waitFor(() => expect(within(grade).getAllByRole("option")).toHaveLength(GRADES.length + 1));
  await user.selectOptions(grade, gradeId);
  return drawer;
}

const groupSelect = (drawer: HTMLElement): HTMLSelectElement => within(drawer).getByLabelText("Grupo", CAMPO) as HTMLSelectElement;
const entryDate = (drawer: HTMLElement): HTMLInputElement => within(drawer).getByLabelText("Fecha de ingreso", CAMPO) as HTMLInputElement;
const create = async (user: User, drawer: HTMLElement): Promise<void> => {
  await user.click(within(drawer).getByRole("button", { name: "Crear alumno" }));
};

describe("the alta's group and entry date", () => {
  // A page has room: nothing hides behind a disclosure any more.
  it("shows Grupo and Fecha de ingreso right away, on Automático and today", async () => {
    const user = userEvent.setup();
    const drawer = await openFilledDrawer(user);

    expect(within(drawer).queryByRole("button", { name: /grupo o fecha de ingreso/i })).toBeNull();
    expect(groupSelect(drawer).value).toBe("");
    expect(entryDate(drawer).value).toBe("2026-10-16");
  });

  // "25 lugares" read as 25 FREE seats; it is the group's size. 0 = not sized yet.
  it("offers Automático first, then the grade's own groups by name with their cupo", async () => {
    const user = userEvent.setup();
    const drawer = await openFilledDrawer(user);

    const options = within(groupSelect(drawer)).getAllByRole("option").map((o) => o.textContent);
    expect(options).toEqual(["Automático (el primero con lugar)", "A · cupo 25", "B · cupo 30", "C · sin cupo definido"]);
    expect(groupSelect(drawer).value).toBe("");
  });

  it("prefills the entry date with the SCHOOL's today, never the UTC day", async () => {
    // 20:00 in Morelia on the 16th is already the 17th in UTC.
    vi.setSystemTime(new Date("2026-10-17T02:00:00Z"));
    const user = userEvent.setup();
    const drawer = await openFilledDrawer(user);

    expect(entryDate(drawer).value).toBe("2026-10-16");
    expect(entryDate(drawer)).toHaveAttribute("max", "2026-10-16");
    expect(entryDate(drawer)).toHaveAttribute("min", "2026-08-24");
  });

  it("by default lets capacity choose, and sends no entry date for today", async () => {
    const user = userEvent.setup();
    const drawer = await openFilledDrawer(user);
    await create(user, drawer);

    await waitFor(() => expect(students.enrollIntoGrade).toHaveBeenCalledWith(expect.any(String), "s1", "gl-1", ""));
    expect(students.enroll).not.toHaveBeenCalled();
  });

  it("registers Valentina straight into 1° B", async () => {
    const user = userEvent.setup();
    const drawer = await openFilledDrawer(user);
    await user.selectOptions(groupSelect(drawer), "grp-1b");
    await create(user, drawer);

    await waitFor(() => expect(students.enroll).toHaveBeenCalledWith(expect.any(String), { studentId: "s1", groupId: "grp-1b", enrolledOn: "" }));
    expect(students.enrollIntoGrade).not.toHaveBeenCalled();
    await waitFor(() => expect(onCreated).toHaveBeenCalledWith(expect.stringMatching(/quedó en 1° PRIMARIA · B/)));
  });

  it("sends a past entry date and says billing starts from that month", async () => {
    const user = userEvent.setup();
    const drawer = await openFilledDrawer(user);
    expect(within(drawer).queryByText("Se cobrarán las colegiaturas desde ese mes.")).toBeNull();

    fireEvent.change(entryDate(drawer), { target: { value: "2026-08-24" } });
    expect(within(drawer).getByText("Se cobrarán las colegiaturas desde ese mes.")).toBeInTheDocument();
    await create(user, drawer);

    await waitFor(() => expect(students.enrollIntoGrade).toHaveBeenCalledWith(expect.any(String), "s1", "gl-1", "2026-08-24"));
  });

  it("refuses a future entry date before asking the server", async () => {
    const user = userEvent.setup();
    const drawer = await openFilledDrawer(user);
    fireEvent.change(entryDate(drawer), { target: { value: "2026-10-20" } });
    await create(user, drawer);

    expect(entryDate(drawer)).toHaveAttribute("aria-invalid", "true");
    expect(students.createStudent).not.toHaveBeenCalled();
  });

  it("goes back to Automático when the grade changes", async () => {
    const user = userEvent.setup();
    const drawer = await openFilledDrawer(user);
    await user.selectOptions(groupSelect(drawer), "grp-1b");

    await user.selectOptions(within(drawer).getByLabelText("Grado", CAMPO), "gl-2");

    expect(groupSelect(drawer).value).toBe("");
    expect(within(groupSelect(drawer)).getAllByRole("option").map((o) => o.textContent)).toEqual(["Automático (el primero con lugar)", "A · cupo 28"]);
  });

  it.each([
    [400, "La fecha de ingreso debe estar entre el inicio del ciclo y hoy"],
    [403, "Para una fecha de ingreso anterior a hoy se necesita permiso de finanzas"],
  ])("shows the server's own words for a %i, and says the pupil was created", async (status, words) => {
    students.enrollIntoGrade.mockRejectedValue(new StudentsError(status, words));
    const user = userEvent.setup();
    const drawer = await openFilledDrawer(user);
    fireEvent.change(entryDate(drawer), { target: { value: "2026-08-24" } });
    await create(user, drawer);

    const alert = await within(drawer).findByRole("alert");
    expect(alert).toHaveTextContent(words);
    expect(alert).toHaveTextContent(/sí se creó/);
  });

  // The pupil exists after the first try; a second "Crear alumno" must only
  // place them, or it meets "ya existe ese CURP" and the fix is lost.
  it("after fixing the date, retries only the placement — never a second pupil", async () => {
    students.enrollIntoGrade.mockRejectedValueOnce(new StudentsError(403, "Para una fecha de ingreso anterior a hoy se necesita permiso de finanzas"));
    const user = userEvent.setup();
    const drawer = await openFilledDrawer(user);
    fireEvent.change(entryDate(drawer), { target: { value: "2026-08-24" } });
    await create(user, drawer);
    await within(drawer).findByRole("alert");

    fireEvent.change(entryDate(drawer), { target: { value: "2026-10-16" } });
    await create(user, drawer);

    await waitFor(() => expect(students.enrollIntoGrade).toHaveBeenLastCalledWith(expect.any(String), "s1", "gl-1", ""));
    expect(students.createStudent).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(onCreated).toHaveBeenCalledWith(expect.stringMatching(/quedó en 1° A/)));
  });

  // Tech-lead blocker: the retry only places, so an edit to who the pupil IS
  // would be silently dropped. Those fields lock, and the page says why.
  it("after a placement failure, locks who the pupil is and says only placement is left", async () => {
    students.enrollIntoGrade.mockRejectedValueOnce(new StudentsError(409, "no quedan lugares"));
    const user = userEvent.setup();
    const drawer = await openFilledDrawer(user);
    await create(user, drawer);
    await within(drawer).findByRole("alert");

    expect(within(drawer).getByText("El alumno ya se creó; corrige sólo grupo, grado o fecha de ingreso.")).toBeInTheDocument();
    for (const label of ["CURP", "Apellido paterno", "Apellido materno", "Nombre(s)", "Tutor"]) {
      expect(within(drawer).getByLabelText(label, CAMPO)).toHaveAttribute("readonly");
    }
    expect(within(drawer).getByLabelText("Nivel", CAMPO)).toBeDisabled();
    await user.type(within(drawer).getByLabelText("Nombre(s)", CAMPO), "X");
    expect(within(drawer).getByLabelText("Nombre(s)", CAMPO)).toHaveValue("VALENTINA");
    // Grade and group stay editable: they are what the retry sends.
    expect(groupSelect(drawer)).not.toBeDisabled();
    expect(within(drawer).getByLabelText("Grado", CAMPO)).not.toBeDisabled();

    await user.selectOptions(within(drawer).getByLabelText("Grado", CAMPO), "gl-2");
    await create(user, drawer);
    await waitFor(() => expect(students.enrollIntoGrade).toHaveBeenLastCalledWith(expect.any(String), "s1", "gl-2", ""));
    expect(students.createStudent).toHaveBeenCalledTimes(1);
  });
});

describe("Crear y agregar otro", () => {
  // Repetitive entry: the context (Nivel, Grado, Grupo, Fecha) stays, who the
  // pupil is clears, and the cursor goes back to the first field.
  it("keeps the placement, clears the pupil, refocuses CURP and says who was created", async () => {
    const user = userEvent.setup();
    const drawer = await openFilledDrawer(user);
    await user.selectOptions(groupSelect(drawer), "grp-1b");
    fireEvent.change(entryDate(drawer), { target: { value: "2026-09-01" } });

    await user.click(within(drawer).getByRole("button", { name: "Crear y agregar otro" }));

    await waitFor(() => expect(within(drawer).getByLabelText("CURP", CAMPO)).toHaveValue(""));
    expect(within(drawer).getByLabelText("CURP", CAMPO)).toHaveFocus();
    expect(within(drawer).getByLabelText("Nombre(s)", CAMPO)).toHaveValue("");
    expect(within(drawer).getByLabelText("Tutor", CAMPO)).toHaveValue("");
    expect((within(drawer).getByLabelText("Nivel", CAMPO) as HTMLSelectElement).value).toBe("lv-p");
    expect((within(drawer).getByLabelText("Grado", CAMPO) as HTMLSelectElement).value).toBe("gl-1");
    expect(groupSelect(drawer).value).toBe("grp-1b");
    expect(entryDate(drawer).value).toBe("2026-09-01");
    expect(within(drawer).getByRole("status")).toHaveTextContent(/Alumno creado · .*quedó en 1° PRIMARIA · B/);
    expect(onCreated).not.toHaveBeenCalled();
  });

  it("a second pupil after a partial failure is a NEW pupil, not a retry", async () => {
    students.enrollIntoGrade.mockRejectedValueOnce(new StudentsError(409, "no quedan lugares"));
    const user = userEvent.setup();
    const drawer = await openFilledDrawer(user);
    await create(user, drawer);
    await within(drawer).findByRole("alert");
    await user.click(within(drawer).getByRole("button", { name: "Crear y agregar otro" }));
    await waitFor(() => expect(within(drawer).getByLabelText("CURP", CAMPO)).toHaveValue(""));

    expect(within(drawer).getByLabelText("CURP", CAMPO)).not.toHaveAttribute("readonly");
    expect(within(drawer).queryByText(/El alumno ya se creó/)).toBeNull();
  });
});
