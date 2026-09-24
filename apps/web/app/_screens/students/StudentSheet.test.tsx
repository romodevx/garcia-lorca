import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import * as configApi from "../../config/api";
import { StudentsError } from "../../students/api";
import { StudentsScreen } from "./StudentsScreen";

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
