import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import * as configApi from "../../config/api";
import { StudentsError } from "../../students/api";
import { StudentsScreen, placementRefusal } from "./StudentsScreen";

vi.mock("../../students/api", async (importOriginal) => ({
  ...(await importOriginal<typeof studentsApi>()),
  listStudents: vi.fn(),
  listEnrollments: vi.fn(),
  enroll: vi.fn(),
  transfer: vi.fn(),
}));
vi.mock("../../config/api", () => ({
  listCycles: vi.fn(),
  listGroups: vi.fn(),
  listLevels: vi.fn(),
  listGradeLevels: vi.fn(),
}));
vi.mock("./ContactsPanel", () => ({ ContactsPanel: () => null }));

const students = vi.mocked(studentsApi);
const config = vi.mocked(configApi);

const pupil = (levelId: string): studentsApi.Student => ({
  id: "stu-1", curp: "HEGA050312MDFRNN08", firstName: "Ana", lastName: "Cruz",
  level: "Primaria", levelId, gradeGroup: "1° B", guardianName: "Rosa Cruz", admissionStatus: "pending",
});

// Four groups share the name "1° A", one per level: the ambiguity the sheet
// used to put in front of the office.
const GROUPS = [
  { id: "g-pri-a", cycleId: "c1", gradeLevelId: "gl-pri-1", name: "1° A", capacity: 30 },
  { id: "g-pri-b", cycleId: "c1", gradeLevelId: "gl-pri-1", name: "1° B", capacity: 30 },
  { id: "g-kin-a", cycleId: "c1", gradeLevelId: "gl-kin-1", name: "1° A", capacity: 30 },
];
const GRADES = [
  { id: "gl-pri-1", levelId: "lv-pri", name: "1° Primaria", ordinal: 1 },
  { id: "gl-kin-1", levelId: "lv-kin", name: "1° Kínder", ordinal: 1 },
];

beforeEach(() => {
  config.listCycles.mockResolvedValue([{ id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" }]);
  config.listGroups.mockResolvedValue(GROUPS);
  config.listLevels.mockResolvedValue([]);
  config.listGradeLevels.mockResolvedValue(GRADES);
});
afterEach(() => vi.clearAllMocks());

async function openSheet(user: ReturnType<typeof userEvent.setup>): Promise<HTMLElement> {
  render(<StudentsScreen onBack={() => {}} />);
  await user.click(await screen.findByRole("button", { name: "Ver ficha de Ana Cruz" }));
  return screen.getByRole("dialog", { name: "Ficha del alumno" });
}

// Regression: task 4. A pupil the quick-add had already placed was offered
// "Inscribir" again; the server answered 409 and the office read a bare
// "No se pudo inscribir". Found reproducing locally on 2026-09-24.
describe("la ficha de un alumno que ya tiene grupo", () => {
  beforeEach(() => {
    students.listStudents.mockResolvedValue([pupil("lv-pri")]);
    students.listEnrollments.mockResolvedValue([
      { id: "enr-1", studentId: "stu-1", cycleId: "c1", groupId: "g-pri-b", termId: "", status: "active", enrolledAt: "2026-08-31" },
    ]);
  });

  it("dice en qué grupo está y no ofrece inscribirlo otra vez", async () => {
    const user = userEvent.setup();
    const sheet = await openSheet(user);

    expect(await within(sheet).findByText(/Grupo actual/)).toBeInTheDocument();
    expect(within(sheet).queryByRole("button", { name: "Inscribir" })).not.toBeInTheDocument();
    expect(within(sheet).getByRole("button", { name: "Cambiar de grupo" })).toBeInTheDocument();
  });

  it("solo ofrece grupos de su nivel, y no el que ya tiene", async () => {
    const user = userEvent.setup();
    const sheet = await openSheet(user);
    const group = await within(sheet).findByRole("group", { name: "Cambiar a" });

    const offered = within(group).getAllByRole("button").map((b) => b.textContent);
    expect(offered).toEqual(["1° A"]); // primaria's 1° A only — not kínder's, not its own 1° B
  });

  it("cambia de grupo con el motivo capturado", async () => {
    const user = userEvent.setup();
    students.transfer.mockResolvedValue({ id: "enr-1", studentId: "stu-1", cycleId: "c1", groupId: "g-pri-a", termId: "", status: "active", enrolledAt: "2026-08-31" });
    const sheet = await openSheet(user);
    await user.click(within(await within(sheet).findByRole("group", { name: "Cambiar a" })).getByRole("button", { name: "1° A" }));
    await user.type(within(sheet).getByLabelText(/^Motivo/), "Cambio de turno");
    await user.click(within(sheet).getByRole("button", { name: "Cambiar de grupo" }));

    await waitFor(() =>
      expect(students.transfer).toHaveBeenCalledWith(expect.any(String), {
        enrollmentId: "enr-1", toGroupId: "g-pri-a", reason: "Cambio de turno",
      }),
    );
  });

  it("un doble clic no manda dos cambios de grupo", async () => {
    const user = userEvent.setup();
    let release: () => void = () => {};
    students.transfer.mockImplementation(() => new Promise((resolve) => {
      release = () => resolve({ id: "enr-1", studentId: "stu-1", cycleId: "c1", groupId: "g-pri-a", termId: "", status: "active", enrolledAt: "2026-08-31" });
    }));
    const sheet = await openSheet(user);
    await user.click(within(await within(sheet).findByRole("group", { name: "Cambiar a" })).getByRole("button", { name: "1° A" }));
    await user.type(within(sheet).getByLabelText(/^Motivo/), "Cambio de turno");
    const go = within(sheet).getByRole("button", { name: "Cambiar de grupo" });
    await user.click(go);
    await user.click(go);
    release();

    await waitFor(() => expect(students.transfer).toHaveBeenCalledTimes(1));
  });

  it("sin motivo marca el campo en lugar de mandar", async () => {
    const user = userEvent.setup();
    const sheet = await openSheet(user);
    await user.click(within(await within(sheet).findByRole("group", { name: "Cambiar a" })).getByRole("button", { name: "1° A" }));
    await user.click(within(sheet).getByRole("button", { name: "Cambiar de grupo" }));

    expect(within(sheet).getByLabelText(/^Motivo/)).toHaveAttribute("aria-invalid", "true");
    expect(students.transfer).not.toHaveBeenCalled();
  });
});

describe("la ficha de un alumno sin grupo", () => {
  beforeEach(() => {
    students.listStudents.mockResolvedValue([pupil("lv-pri")]);
    students.listEnrollments.mockResolvedValue([]);
  });

  it("ofrece inscribir solo en grupos de su nivel", async () => {
    const user = userEvent.setup();
    const sheet = await openSheet(user);
    const group = await within(sheet).findByRole("group", { name: "Inscribir en" });

    expect(within(group).getAllByRole("button").map((b) => b.textContent)).toEqual(["1° A", "1° B"]);
  });

  it("dice lo que respondió el servidor, no un «No se pudo» a secas", async () => {
    const user = userEvent.setup();
    students.enroll.mockRejectedValue(new StudentsError(409, "student is already enrolled this cycle"));
    const sheet = await openSheet(user);
    await user.click(within(await within(sheet).findByRole("group", { name: "Inscribir en" })).getByRole("button", { name: "1° A" }));
    await user.click(within(sheet).getByRole("button", { name: "Inscribir" }));

    expect(await within(sheet).findByRole("alert")).toHaveTextContent(/ya está inscrito/i);
  });
});

describe("lo que la ficha lee", () => {
  beforeEach(() => {
    students.listStudents.mockResolvedValue([pupil("lv-pri")]);
  });

  // An enrolment from last year is still "active" (a cycle's end closes
  // nothing), so the sheet asks for the pupil's enrolment in THIS cycle only.
  it("busca la inscripción del ciclo cuyos grupos muestra", async () => {
    students.listEnrollments.mockResolvedValue([]);
    const user = userEvent.setup();
    await openSheet(user);
    await waitFor(() => expect(students.listEnrollments).toHaveBeenCalledWith(expect.any(String), { studentId: "stu-1", cycleId: "c1" }));
  });

  // Not reading the enrolment is not "no group": offering "Inscribir" then
  // brings back the very 409 this sheet was fixed for.
  it("si no pudo leer su inscripción, lo dice y no ofrece inscribir", async () => {
    students.listEnrollments.mockRejectedValue(new Error("network"));
    const user = userEvent.setup();
    const sheet = await openSheet(user);

    expect(await within(sheet).findByRole("alert")).toHaveTextContent(/No pudimos leer/);
    expect(within(sheet).queryByRole("button", { name: "Inscribir" })).not.toBeInTheDocument();
  });

  it("si no pudo leer los grados, no manda a crear grupos que ya existen", async () => {
    students.listEnrollments.mockResolvedValue([]);
    config.listGradeLevels.mockRejectedValue(new Error("config down"));
    const user = userEvent.setup();
    const sheet = await openSheet(user);

    expect(await within(sheet).findByRole("alert")).toHaveTextContent(/No pudimos leer los grados/);
    expect(within(sheet).queryByText(/créalos en Configuración/)).not.toBeInTheDocument();
  });
});

describe("placementRefusal", () => {
  it("dice «ya inscrito» solo cuando eso fue lo que pasó", () => {
    expect(placementRefusal(new StudentsError(409, "student is already enrolled this cycle"))).toMatch(/Ya está inscrito/);
    expect(placementRefusal(new StudentsError(409, "ese alumno está dado de baja; reactívalo antes"))).toMatch(/dado de baja/);
  });

  it("no enseña inglés del servidor", () => {
    expect(placementRefusal(new StudentsError(400, "cannot transfer across cycles"))).toMatch(/otro ciclo/);
    expect(placementRefusal(new StudentsError(400, "enrollment is not active"))).toMatch(/ya no está activa/);
    expect(placementRefusal(new StudentsError(400, "group not found"))).toMatch(/ya no existe/);
    expect(placementRefusal(new StudentsError(400, "ese grupo es de otro nivel: el alumno está en Primaria"))).toMatch(/otro nivel/);
  });
});
