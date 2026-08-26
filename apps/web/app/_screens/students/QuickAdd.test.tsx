import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import * as configApi from "../../config/api";
import { StudentsScreen, missingFieldMessage, createFailureMessage } from "./StudentsScreen";
import { StudentsError } from "../../students/api";

vi.mock("../../students/api", async (importOriginal) => ({
  ...(await importOriginal<typeof studentsApi>()),
  listStudents: vi.fn(),
  createStudent: vi.fn(),
  enroll: vi.fn(),
}));
vi.mock("../../config/api", () => ({ listCycles: vi.fn(), listGroups: vi.fn() }));

const students = vi.mocked(studentsApi);
const config = vi.mocked(configApi);

beforeEach(() => {
  students.listStudents.mockResolvedValue([]);
  config.listCycles.mockResolvedValue([]);
  config.listGroups.mockResolvedValue([]);
});
afterEach(() => vi.clearAllMocks());

const FULL = {
  curp: "ROQA930615HDFMRN08",
  firstName: "Andrés",
  lastName: "Romo Quiroz",
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
    expect(missingFieldMessage({ ...FULL, lastName: "" })).toMatch(/apellidos/i);
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
    await user.type(within(drawer).getByLabelText(/CURP/), FULL.curp);
    await user.type(within(drawer).getByLabelText(/Nombre/), FULL.firstName);
    await user.type(within(drawer).getByLabelText(/Apellidos/), FULL.lastName);
    // Tutor deliberately left blank.

    await user.click(within(drawer).getByRole("button", { name: "Crear alumno" }));

    expect(await within(drawer).findByRole("alert")).toHaveTextContent(/tutor/i);
    expect(students.createStudent).not.toHaveBeenCalled();
  });

  it("creates the pupil once every field is there", async () => {
    const user = userEvent.setup();
    students.createStudent.mockResolvedValue({
      id: "s1", curp: FULL.curp, firstName: FULL.firstName, lastName: FULL.lastName,
      level: "primaria", gradeGroup: "1° A", guardianName: FULL.guardianName,
      admissionStatus: "pending",
    });
    render(<StudentsScreen onBack={() => {}} />);
    await user.click((await screen.findAllByRole("button", { name: "Nuevo alumno" }))[0]!);

    const drawer = screen.getByRole("dialog", { name: "Nuevo alumno" });
    await user.type(within(drawer).getByLabelText(/CURP/), FULL.curp);
    await user.type(within(drawer).getByLabelText(/Nombre/), FULL.firstName);
    await user.type(within(drawer).getByLabelText(/Apellidos/), FULL.lastName);
    await user.type(within(drawer).getByLabelText(/Tutor/), FULL.guardianName);
    await user.click(within(drawer).getByRole("button", { name: "Crear alumno" }));

    await waitFor(() => expect(students.createStudent).toHaveBeenCalled());
    expect(students.createStudent.mock.calls[0]?.[1]).toMatchObject({ curp: FULL.curp });
  });

  it("says a CURP is taken when the server says so", async () => {
    const user = userEvent.setup();
    students.createStudent.mockRejectedValue(new StudentsError(409, ""));
    render(<StudentsScreen onBack={() => {}} />);
    await user.click((await screen.findAllByRole("button", { name: "Nuevo alumno" }))[0]!);

    const drawer = screen.getByRole("dialog", { name: "Nuevo alumno" });
    await user.type(within(drawer).getByLabelText(/CURP/), FULL.curp);
    await user.type(within(drawer).getByLabelText(/Nombre/), FULL.firstName);
    await user.type(within(drawer).getByLabelText(/Apellidos/), FULL.lastName);
    await user.type(within(drawer).getByLabelText(/Tutor/), FULL.guardianName);
    await user.click(within(drawer).getByRole("button", { name: "Crear alumno" }));

    expect(await within(drawer).findByRole("alert")).toHaveTextContent(/ya (existe|está)/i);
  });
});
