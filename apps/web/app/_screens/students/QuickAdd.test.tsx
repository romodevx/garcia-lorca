import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import * as configApi from "../../config/api";
import { StudentsScreen, missingFieldMessage, createFailureMessage, isSepLevel } from "./StudentsScreen";
import { StudentsError } from "../../students/api";

vi.mock("../../students/api", async (importOriginal) => ({
  ...(await importOriginal<typeof studentsApi>()),
  listStudents: vi.fn(),
  createStudent: vi.fn(),
  enroll: vi.fn(),
  enrollIntoGrade: vi.fn(),
}));
vi.mock("../../config/api", () => ({ listCycles: vi.fn(), listGroups: vi.fn(), listLevels: vi.fn(), listGradeLevels: vi.fn() }));

const students = vi.mocked(studentsApi);
const config = vi.mocked(configApi);

const LEVELS = [
  { id: "lv-k", name: "Kínder", sepCode: "KIN", enrollmentGranularity: "cycle" as const },
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

// A school names its own levels; a pupil's record carries a SEP level. A level
// outside the supported set has nowhere to put one, and being told that beats a 400 that
// blames the CURP — which is what the panel used to show for it.
describe("which levels can actually hold a pupil", () => {
  it("folds case and accents the way the services do", () => {
    expect(isSepLevel("Kínder")).toBe(true);
    expect(isSepLevel("KINDER")).toBe(true);
    expect(isSepLevel("  primaria ")).toBe(true);
    expect(isSepLevel("Preparatoria")).toBe(true);
    // Maternal became a real level in M16 — the school teaches it and could not
    // enrol those pupils while this returned false.
    expect(isSepLevel("Maternal")).toBe(true);
    expect(isSepLevel("maternal")).toBe(true);
    // A level the system genuinely cannot hold still has to be caught here.
    expect(isSepLevel("Universidad")).toBe(false);
    expect(isSepLevel("Primaria B")).toBe(false);
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
    await user.selectOptions(within(drawer).getByLabelText("Nivel"), "lv-k");
    await user.selectOptions(await within(drawer).findByLabelText("Grado"), "gl-k1");
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
    await user.type(within(drawer).getByLabelText(/CURP/), FULL.curp);
    await user.type(within(drawer).getByLabelText(/Nombre/), FULL.firstName);
    await user.type(within(drawer).getByLabelText(/Apellidos/), FULL.lastName);
    await user.type(within(drawer).getByLabelText(/Tutor/), FULL.guardianName);
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
      level: "kinder", gradeGroup: "", guardianName: FULL.guardianName, admissionStatus: "pending",
    });
    const drawer = await openDrawer(user);
    await fillIdentity(user, drawer);
    await user.selectOptions(within(drawer).getByLabelText("Nivel"), "lv-k");
    await user.selectOptions(await within(drawer).findByLabelText("Grado"), "gl-k2");

    await user.click(within(drawer).getByRole("button", { name: "Crear alumno" }));

    await waitFor(() => expect(students.enrollIntoGrade).toHaveBeenCalledWith(expect.any(String), "s1", "gl-k2"));
    // The level travels as the config level's NAME; the server folds accents, so
    // "Kínder" resolves to the SEP level without a mapping table on this side.
    expect(students.createStudent.mock.calls[0]?.[1]).toMatchObject({ level: "Kínder" });
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
      level: "kinder", gradeGroup: "", guardianName: FULL.guardianName, admissionStatus: "pending",
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

describe("a level that cannot hold a pupil", () => {
  it("says so before sending, and names the levels that can", async () => {
    const user = userEvent.setup();
    // Deliberately a level the system does not carry. Maternal used to stand in
    // for this and no longer can: it is a supported level as of M16.
    config.listLevels.mockResolvedValue([
      { id: "lv-m", name: "Universidad", sepCode: "UNI", enrollmentGranularity: "cycle" as const },
    ]);
    config.listGradeLevels.mockResolvedValue([{ id: "gl-m1", levelId: "lv-m", name: "Primer semestre", ordinal: 1 }]);
    render(<StudentsScreen onBack={() => {}} />);
    await user.click((await screen.findAllByRole("button", { name: "Nuevo alumno" }))[0]!);

    const drawer = screen.getByRole("dialog", { name: "Nuevo alumno" });
    await user.type(within(drawer).getByLabelText(/CURP/), FULL.curp);
    await user.type(within(drawer).getByLabelText(/Nombre/), FULL.firstName);
    await user.type(within(drawer).getByLabelText(/Apellidos/), FULL.lastName);
    await user.type(within(drawer).getByLabelText(/Tutor/), FULL.guardianName);
    await user.selectOptions(within(drawer).getByLabelText("Nivel"), "lv-m");
    await user.selectOptions(await within(drawer).findByLabelText("Grado"), "gl-m1");

    await user.click(within(drawer).getByRole("button", { name: "Crear alumno" }));

    const alert = await within(drawer).findByRole("alert");
    expect(alert).toHaveTextContent(/Universidad/);
    expect(alert).toHaveTextContent(/Maternal/);
    expect(students.createStudent).not.toHaveBeenCalled();
  });
});
