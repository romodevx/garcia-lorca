import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import * as configApi from "../../config/api";
import { StudentAltaPage } from "./StudentAltaPage";

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

beforeEach(() => {
  students.listStudents.mockResolvedValue([]);
  config.listCycles.mockResolvedValue([]);
  config.listGroups.mockResolvedValue([]);
  config.listLabelledGroups.mockResolvedValue([]);
  config.listLevels.mockResolvedValue([{ id: "lv-k", name: "Kínder", sepCode: "KIN", enrollmentGranularity: "cycle" }]);
  config.listGradeLevels.mockResolvedValue([{ id: "gl-k1", levelId: "lv-k", name: "1° Kínder", ordinal: 1 }]);
  students.createStudent.mockResolvedValue({
    id: "s1", curp: "ROQA930615HDFMRN08", firstName: "ANA", lastName: "ROMO",
    level: "Kínder", levelId: "lv-k", gradeGroup: "", guardianName: "MARÍA", admissionStatus: "pending",
  });
  students.enrollIntoGrade.mockResolvedValue({
    enrollment: { id: "e1", studentId: "s1", cycleId: "c1", groupId: "g1", termId: "", status: "active", enrolledAt: "2026-08-31" },
    groupName: "1° A",
  });
});
afterEach(() => vi.clearAllMocks());

async function openAndFill(): Promise<{ drawer: HTMLElement; user: ReturnType<typeof userEvent.setup> }> {
  const user = userEvent.setup();
  const drawer = await openAlta();
  await user.type(within(drawer).getByLabelText(/CURP/, CAMPO), "ROQA930615HDFMRN08");
  await user.type(within(drawer).getByLabelText(/Apellido paterno/, CAMPO), "Romo");
  await user.type(within(drawer).getByLabelText(/Nombre\(s\)/, CAMPO), "Ana");
  await user.type(within(drawer).getByLabelText(/Tutor/, CAMPO), "María Quiroz");
  await user.selectOptions(within(drawer).getByLabelText("Nivel"), "lv-k");
  await user.selectOptions(await within(drawer).findByLabelText("Grado"), "gl-k1");
  return { drawer, user };
}

// CC-314: a CURP whose check digit is invented, or whose date is not the
// birth date captured, is WARNED about under the field — and still saved:
// the legacy import brings imperfect CURPs, so it is never a block.
describe("quick add: CURP warnings (CC-314)", () => {
  it("warns about an invented check digit and still creates the pupil", async () => {
    const { drawer, user } = await openAndFill();
    const curp = within(drawer).getByLabelText(/CURP/, CAMPO);
    await user.clear(curp);
    await user.type(curp, "PUCA150101HMNRCLA0");

    expect(await within(drawer).findByText(/dígito verificador/)).toBeInTheDocument();
    await user.click(within(drawer).getByRole("button", { name: "Crear alumno" }));
    await waitFor(() => expect(students.createStudent).toHaveBeenCalled());
    expect(students.createStudent.mock.calls[0]?.[1]).toMatchObject({ curp: "PUCA150101HMNRCLA0" });
  });

  it("warns when the birth date is not the one the CURP carries", async () => {
    const { drawer, user } = await openAndFill();
    const curp = within(drawer).getByLabelText(/CURP/, CAMPO);
    await user.clear(curp);
    await user.type(curp, "PUCA150101HMNRCLA5");
    expect(within(drawer).queryByText(/El CURP dice que nació/)).toBeNull();

    await user.click(within(drawer).getByRole("button", { name: /más datos/i }));
    fireEvent.change(within(drawer).getByLabelText(/Fecha de nacimiento/, CAMPO), { target: { value: "2014-10-17" } });
    expect(await within(drawer).findByText(/El CURP dice que nació el 01\/01\/2015/)).toBeInTheDocument();
  });
});

// T11: the birth date is what the morning greeting reads. The drawer already
// holds eight fields, so it waits under "Más datos", collapsed: the 80% case
// does not need it to create the pupil (ui-forms-and-modals.md).
describe("quick add: birth date (T11)", () => {
  it("keeps the birth date collapsed until asked for", async () => {
    const { drawer, user } = await openAndFill();
    expect(within(drawer).queryByLabelText(/Fecha de nacimiento/, CAMPO)).toBeNull();

    const more = within(drawer).getByRole("button", { name: /más datos/i });
    expect(more).toHaveAttribute("aria-expanded", "false");
    await user.click(more);
    expect(more).toHaveAttribute("aria-expanded", "true");
    expect(within(drawer).getByLabelText(/Fecha de nacimiento/, CAMPO)).toBeInTheDocument();
  });

  it("sends the birth date with the new pupil", async () => {
    const { drawer, user } = await openAndFill();
    await user.click(within(drawer).getByRole("button", { name: /más datos/i }));
    fireEvent.change(within(drawer).getByLabelText(/Fecha de nacimiento/, CAMPO), { target: { value: "2016-02-29" } });
    await user.click(within(drawer).getByRole("button", { name: "Crear alumno" }));

    await waitFor(() => expect(students.createStudent).toHaveBeenCalled());
    expect(students.createStudent.mock.calls[0]?.[1]).toMatchObject({ birthDate: "2016-02-29" });
  });

  it("creates the pupil without one, as before", async () => {
    const { drawer, user } = await openAndFill();
    await user.click(within(drawer).getByRole("button", { name: "Crear alumno" }));

    await waitFor(() => expect(students.createStudent).toHaveBeenCalled());
    expect(students.createStudent.mock.calls[0]?.[1]?.birthDate ?? "").toBe("");
  });

  // Review: a typed date after today (the calendar's max only binds the
  // picker) is refused on save, in its own field, with the section opened even
  // if it had been collapsed again.
  it("refuses a birth date after today, opening the section to show it", async () => {
    const { drawer, user } = await openAndFill();
    const more = within(drawer).getByRole("button", { name: /más datos/i });
    await user.click(more);
    fireEvent.change(within(drawer).getByLabelText(/Fecha de nacimiento/, CAMPO), { target: { value: "2999-01-01" } });
    await user.click(more);
    await user.click(within(drawer).getByRole("button", { name: "Crear alumno" }));

    expect(await within(drawer).findByLabelText(/Fecha de nacimiento/, CAMPO)).toHaveAccessibleDescription(/no puede ser posterior a hoy/i);
    expect(more).toHaveAttribute("aria-expanded", "true");
    expect(students.createStudent).not.toHaveBeenCalled();
  });
});
