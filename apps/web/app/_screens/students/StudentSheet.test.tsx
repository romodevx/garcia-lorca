import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import * as configApi from "../../config/api";
import { StudentsError } from "../../students/api";
import { StudentsScreen, placementRefusal } from "./StudentsScreen";

// The screens read labelled groups (ISSUE-002); label them from the listGroups mock.
beforeEach(() => {
  vi.mocked(configApi.listLabelledGroups).mockImplementation(async (...args) =>
    (await vi.mocked(configApi.listGroups)(...args)).map((g) => ({ ...g, label: g.name })));
});


vi.mock("../../students/api", async (importOriginal) => ({
  ...(await importOriginal<typeof studentsApi>()),
  listStudents: vi.fn(),
  listEnrollments: vi.fn(),
  enroll: vi.fn(),
  transfer: vi.fn(),
  previewWithdrawal: vi.fn(),
  withdraw: vi.fn(),
  revokeCard: vi.fn(),
  reissueCard: vi.fn(),
}));
vi.mock("../../config/api", () => ({
  listCycles: vi.fn(),
  listGroups: vi.fn(), listLabelledGroups: vi.fn(),
  listLevels: vi.fn(),
  listGradeLevels: vi.fn(),
}));
vi.mock("./ContactsPanel", () => ({ ContactsPanel: () => null }));

const students = vi.mocked(studentsApi);
const config = vi.mocked(configApi);

const pupil = (levelId: string): studentsApi.Student => ({
  id: "stu-1", curp: "HEGA050312MDFRNN08", firstName: "Ana", lastName: "Cruz",
  level: "Primaria", levelId, gradeGroup: "1° B", guardianName: "Rosa Cruz", admissionStatus: "pending", enrollmentNumber: "2026-00042",
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

async function openSheet(user: ReturnType<typeof userEvent.setup>, props: { canRevokeCard?: boolean; canWithdraw?: boolean } = {}): Promise<HTMLElement> {
  render(<StudentsScreen onBack={() => {}} {...props} />);
  await user.click(await screen.findByRole("button", { name: "Ver ficha de Cruz Ana" }));
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
    // Task 13: the matrícula, as the card shows it.
    expect(within(sheet).getByText("Matrícula")).toBeInTheDocument();
    expect(within(sheet).getByText("2026-00042")).toBeInTheDocument();
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

  it("muestra la vista previa y deja la ficha en solo lectura al dar de baja", async () => {
    const user = userEvent.setup();
    students.previewWithdrawal.mockResolvedValue({ toCancel: [{ id: "fee-1", description: "Colegiatura", dueDate: "2026-11-01", amountCents: 25000 }], toCancelCents: 25000, remainingDebtCents: 0 });
    students.withdraw.mockResolvedValue({ id: "enr-1", studentId: "stu-1", cycleId: "c1", groupId: "g-pri-b", termId: "", status: "withdrawn", enrolledAt: "2026-08-31", withdrawnOn: "2026-10-15", withdrawalReason: "Cambio de escuela" });
    const sheet = await openSheet(user, { canWithdraw: true });
    await user.click(within(sheet).getByRole("button", { name: "Dar de baja" }));
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    expect(within(sheet).getAllByRole("button", { name: "Dar de baja" })).toHaveLength(1);
    expect(await within(sheet).findByText(/Se cancelarán 1 cargos/)).toBeInTheDocument();
    await user.type(within(sheet).getByLabelText("Motivo"), "Cambio de escuela");
    const submit = sheet.querySelector<HTMLButtonElement>('form[aria-labelledby="withdraw-title"] button[type="submit"]');
    expect(submit).not.toBeNull();
    await user.click(submit!);
    await waitFor(() => expect(students.withdraw).toHaveBeenCalledWith(expect.any(String), "enr-1", { withdrawnOn: expect.any(String), reason: "Cambio de escuela" }));
    const baja = await within(sheet).findByText(/Baja el 15 oct 2026 · Cambio de escuela/);
    expect(baja).toBeInTheDocument();
    await waitFor(() => expect(baja).toHaveFocus());
    expect(within(sheet).queryByRole("button", { name: "Cambiar de grupo" })).not.toBeInTheDocument();
  });

  it("returns focus to Dar de baja after closing the panel with unsaved text", async () => {
    const user = userEvent.setup();
    const sheet = await openSheet(user, { canWithdraw: true });
    const trigger = within(sheet).getByRole("button", { name: "Dar de baja" });
    await user.click(trigger);
    await user.type(within(sheet).getByLabelText("Motivo"), "Se muda");
    await user.keyboard("{Escape}");

    expect(await within(sheet).findByText(/¿Descartar el motivo escrito\?/)).toBeInTheDocument();
    await user.click(within(sheet).getByRole("button", { name: "Seguir editando" }));
    expect(within(sheet).getByLabelText("Motivo")).toHaveValue("Se muda");
    await user.keyboard("{Escape}");
    await user.click(await within(sheet).findByRole("button", { name: "Descartar cambios" }));
    const restoredTrigger = await within(sheet).findByRole("button", { name: "Dar de baja" });
    await waitFor(() => expect(restoredTrigger).toHaveFocus());
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

  it("muestra fecha y motivo de una inscripción ya dada de baja", async () => {
    students.listStudents.mockResolvedValue([{ ...pupil("lv-pri"), admissionStatus: "withdrawn" }]);
    students.listEnrollments.mockResolvedValue([{
      id: "enr-withdrawn", studentId: "stu-1", cycleId: "c1", groupId: "g-pri-b", termId: "",
      status: "withdrawn", enrolledAt: "2026-08-31", withdrawnOn: "2026-10-15", withdrawalReason: "motivo",
    }]);
    const user = userEvent.setup();
    const sheet = await openSheet(user);

    expect(await within(sheet).findByText("Baja el 15 oct 2026 · motivo")).toBeInTheDocument();
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

// Task 13: revoking a card is irreversible (the old code dies), so it asks
// once more on the same button before doing it.
describe("la credencial en la ficha", () => {
  beforeEach(() => {
    students.listStudents.mockResolvedValue([pupil("lv-pri")]);
    students.listEnrollments.mockResolvedValue([]);
    students.revokeCard.mockResolvedValue(undefined);
    students.reissueCard.mockResolvedValue(undefined);
  });

  it("revoca la credencial tras confirmar en el mismo botón", async () => {
    const user = userEvent.setup();
    const sheet = await openSheet(user, { canRevokeCard: true });

    await user.click(within(sheet).getByRole("button", { name: "Revocar credencial" }));
    expect(students.revokeCard).not.toHaveBeenCalled();
    await user.click(within(sheet).getByRole("button", { name: /Confirmar: revocar/ }));

    await waitFor(() => expect(students.revokeCard).toHaveBeenCalledWith(expect.any(String), "stu-1"));
    expect(await within(sheet).findByText(/queda suspendida/)).toBeInTheDocument();
  });

  // Owner's decision: a revoked card stays suspended until the office issues a
  // new one on purpose (a stolen phone would otherwise get a valid code).
  it("emite una credencial nueva a propósito", async () => {
    const user = userEvent.setup();
    const sheet = await openSheet(user, { canRevokeCard: true });

    await user.click(within(sheet).getByRole("button", { name: "Emitir nueva credencial" }));
    await waitFor(() => expect(students.reissueCard).toHaveBeenCalledWith(expect.any(String), "stu-1"));
    expect(await within(sheet).findByText(/nueva credencial/i, { selector: "p" })).toBeInTheDocument();
  });

  it("dice que la credencial sigue vigente si no hay nada que reemitir", async () => {
    students.reissueCard.mockRejectedValue(new studentsApi.StudentsError(409, "the pupil's card is valid; revoke it first"));
    const user = userEvent.setup();
    const sheet = await openSheet(user, { canRevokeCard: true });

    await user.click(within(sheet).getByRole("button", { name: "Emitir nueva credencial" }));
    expect(await within(sheet).findByRole("alert")).toHaveTextContent("La credencial sigue vigente");
  });

  it("no la ofrece sin el permiso", async () => {
    const user = userEvent.setup();
    const sheet = await openSheet(user);
    expect(within(sheet).queryByRole("button", { name: "Revocar credencial" })).toBeNull();
  });
});

describe("verificar credencial (tarea 13)", () => {
  beforeEach(() => {
    students.listStudents.mockResolvedValue([pupil("lv-pri")]);
    students.listEnrollments.mockResolvedValue([]);
  });

  it("abre la verificación desde Alumnos, sólo con el módulo encendido", async () => {
    const user = userEvent.setup();
    const { unmount } = render(<StudentsScreen onBack={() => {}} />);
    await screen.findByRole("button", { name: "Ver ficha de Cruz Ana" });
    expect(screen.queryByRole("button", { name: "Verificar credencial" })).toBeNull();
    unmount();

    render(<StudentsScreen onBack={() => {}} canVerifyCard />);
    await user.click(await screen.findByRole("button", { name: "Verificar credencial" }));
    expect(screen.getByRole("dialog", { name: "Verificar credencial" })).toBeInTheDocument();
  });
});
