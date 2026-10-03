import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as api from "../../schedule/api";
import { ANA, G3A, LUIS, MATE, sampleBoard } from "../../schedule/__fixtures__/board";
import { ScheduleScreen } from "./ScheduleScreen";

vi.mock("../../schedule/api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../../schedule/api")>();
  return {
    ...real,
    getBoard: vi.fn(),
    getOwnBoard: vi.fn(),
    getDay: vi.fn(),
    getOwnDay: vi.fn(),
    createOverride: vi.fn(),
    listAvailableTeachers: vi.fn(),
    listEvents: vi.fn(),
    createAbsence: vi.fn(),
    updateAbsence: vi.fn(),
    cancelAbsence: vi.fn(),
    listAbsences: vi.fn(),
    listUncovered: vi.fn(),
  };
});
const m = vi.mocked(api);

/** Monday 2026-10-05, 08:10 at the school (UTC−6). */
const MONDAY_0810 = new Date("2026-10-05T14:10:00Z");
const BLOCK_MATE = "70000000-0000-4000-8000-000000000001";
const ABSENCE = "90000000-0000-4000-8000-000000000001";

const anaAway = (over: Partial<api.TeacherAbsence> = {}): api.TeacherAbsence => ({
  id: ABSENCE, cycleId: sampleBoard().cycle.id, teacherId: ANA, startsOn: "2026-10-05", endsOn: "2026-10-05",
  allDay: true, startsAt: null, endsAt: null, cancelled: false, kind: "sick", note: "gripe", createdByName: "Dirección", ...over,
});

const uncoveredMate = (absence: api.TeacherAbsence = anaAway()): api.ScheduleClass => ({
  blockId: BLOCK_MATE, groupId: G3A, subjectId: MATE, teacherId: ANA, startsAt: "08:00", endsAt: "08:50", status: "uncovered",
  baseTeacherId: ANA, baseStartsAt: "08:00", baseEndsAt: "08:50", overrideId: null, reason: null, absence,
});

beforeEach(() => {
  m.getBoard.mockResolvedValue(sampleBoard());
  m.getOwnBoard.mockResolvedValue(sampleBoard());
  m.listEvents.mockResolvedValue([]);
  m.getDay.mockResolvedValue({ date: "2026-10-05", weekday: 1, nonWorking: null, classes: [] });
  m.getOwnDay.mockResolvedValue({ date: "2026-10-05", weekday: 1, nonWorking: null, classes: [] });
  m.listAvailableTeachers.mockResolvedValue([]);
  m.listUncovered.mockResolvedValue([]);
  m.listAbsences.mockResolvedValue([]);
});
afterEach(() => vi.clearAllMocks());

function renderManager(canWrite = true): ReturnType<typeof userEvent.setup> {
  const user = userEvent.setup();
  render(<ScheduleScreen onBack={() => undefined} mode="manage" canWrite={canWrite} now={MONDAY_0810} />);
  return user;
}

async function openAbsenceDrawer(user: ReturnType<typeof userEvent.setup>): Promise<HTMLElement> {
  await user.click(await screen.findByRole("button", { name: "Registrar ausencia" }));
  return screen.getByRole("dialog", { name: /Registrar ausencia/ });
}

describe("Ausencias — registrar (T19)", () => {
  it("registers a full-day absence and offers to cover what it leaves uncovered", async () => {
    m.createAbsence.mockResolvedValue({ absence: anaAway(), uncoveredCount: 2 });
    const user = renderManager();
    const dialog = await openAbsenceDrawer(user);
    await user.selectOptions(within(dialog).getByLabelText(/^Docente/), ANA);
    await user.click(within(dialog).getByRole("button", { name: "Incapacidad" }));
    expect(within(dialog).getByLabelText(/Todo el día/)).toBeChecked();
    expect(within(dialog).queryByRole("combobox", { name: /^Desde la hora/ })).not.toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Registrar ausencia" }));

    await waitFor(() =>
      expect(m.createAbsence).toHaveBeenCalledWith(expect.any(String), {
        cycleId: sampleBoard().cycle.id, teacherId: ANA, kind: "sick", startsOn: "2026-10-05", endsOn: "2026-10-05", allDay: true,
      }),
    );
    expect(await screen.findByText("Quedan 2 clases sin cubrir en esas fechas")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Cubrir ahora" }));
    expect(screen.getByRole("tab", { name: "Por cubrir" })).toHaveAttribute("aria-selected", "true");
  });

  it("a partial absence asks for its hours (a medical appointment 08:00–10:00)", async () => {
    m.createAbsence.mockResolvedValue({ absence: anaAway({ allDay: false, startsAt: "08:00", endsAt: "10:00" }), uncoveredCount: 0 });
    const user = renderManager();
    const dialog = await openAbsenceDrawer(user);
    await user.selectOptions(within(dialog).getByLabelText(/^Docente/), ANA);
    await user.click(within(dialog).getByRole("button", { name: "Asunto personal" }));
    await user.click(within(dialog).getByLabelText(/Todo el día/));
    await user.click(within(dialog).getByRole("button", { name: "Registrar ausencia" }));
    expect(m.createAbsence).not.toHaveBeenCalled();
    expect(within(dialog).getByRole("combobox", { name: /^Desde la hora/ })).toHaveAttribute("aria-invalid", "true");

    await user.selectOptions(within(dialog).getByRole("combobox", { name: /^Desde la hora/ }), "08:00");
    await user.selectOptions(within(dialog).getByRole("combobox", { name: /^Hasta la hora/ }), "10:00");
    await user.type(within(dialog).getByRole("textbox", { name: /^Nota/ }), "Cita médica");
    await user.click(within(dialog).getByRole("button", { name: "Registrar ausencia" }));
    await waitFor(() =>
      expect(m.createAbsence).toHaveBeenCalledWith(expect.any(String), expect.objectContaining({
        allDay: false, startsAt: "08:00", endsAt: "10:00", kind: "personal", note: "Cita médica",
      })),
    );
    expect(await screen.findByText("No quedan clases sin cubrir en esas fechas")).toBeInTheDocument();
  });

  it("refuses an end date before the start without asking the server", async () => {
    const user = renderManager();
    const dialog = await openAbsenceDrawer(user);
    await user.selectOptions(within(dialog).getByLabelText(/^Docente/), ANA);
    await user.click(within(dialog).getByRole("button", { name: "Vacaciones" }));
    fireEvent.change(within(dialog).getByLabelText(/^Hasta el/), { target: { value: "2026-10-01" } });
    await user.click(within(dialog).getByRole("button", { name: "Registrar ausencia" }));
    expect(m.createAbsence).not.toHaveBeenCalled();
    expect(within(dialog).getByText(/no puede ser antes/)).toBeInTheDocument();
  });

  it("a reader cannot register absences", async () => {
    renderManager(false);
    await screen.findByRole("tab", { name: "Por cubrir" });
    expect(screen.queryByRole("button", { name: "Registrar ausencia" })).not.toBeInTheDocument();
  });
});

describe("Ausencias — clases por cubrir (T19)", () => {
  it("lists today's uncovered classes and assigns a substitute from the suggestions", async () => {
    m.listUncovered.mockResolvedValue([{ date: "2026-10-05", class: uncoveredMate() }]);
    m.listAvailableTeachers.mockResolvedValue([{ teacherId: LUIS, firstName: "Luis", lastName: "Pérez", match: "grade" }]);
    m.createOverride.mockResolvedValue(undefined);
    const user = renderManager();
    await user.click(await screen.findByRole("tab", { name: "Por cubrir" }));
    const list = await screen.findByRole("list", { name: "Clases por cubrir" });
    expect(m.listUncovered).toHaveBeenCalledWith(expect.any(String), sampleBoard().cycle.id, "2026-10-05", "2026-10-05");
    expect(list).toHaveTextContent("Matemáticas");
    expect(list).toHaveTextContent("3.º A");
    expect(list).toHaveTextContent("Ana Ruiz");
    expect(list).toHaveTextContent("Incapacidad");

    await user.click(within(list).getByRole("button", { name: /Asignar suplente/ }));
    const dialog = screen.getByRole("dialog", { name: /Asignar suplente/ });
    await waitFor(() => expect(within(dialog).getByRole("option", { name: "Luis Pérez" })).toBeInTheDocument());
    await user.selectOptions(within(dialog).getByLabelText(/^Suplente/), LUIS);
    await user.click(within(dialog).getByRole("button", { name: "Asignar suplente" }));
    await waitFor(() =>
      expect(m.createOverride).toHaveBeenCalledWith(expect.any(String), {
        blockId: BLOCK_MATE, date: "2026-10-05", kind: "substitute", teacherId: LUIS, reason: "Ausencia de Ana Ruiz",
      }),
    );
  });

  it("switches to this week and filters by teacher", async () => {
    m.listUncovered.mockResolvedValue([
      { date: "2026-10-05", class: uncoveredMate() },
      { date: "2026-10-06", class: { ...uncoveredMate(anaAway({ teacherId: LUIS })), teacherId: LUIS, blockId: "x" } },
    ]);
    const user = renderManager();
    await user.click(await screen.findByRole("tab", { name: "Por cubrir" }));
    await user.click(await screen.findByRole("button", { name: "Esta semana" }));
    await waitFor(() => expect(m.listUncovered).toHaveBeenLastCalledWith(expect.any(String), sampleBoard().cycle.id, "2026-10-05", "2026-10-09"));
    const list = await screen.findByRole("list", { name: "Clases por cubrir" });
    expect(within(list).getAllByRole("listitem")).toHaveLength(2);
    await user.selectOptions(screen.getByLabelText("Docente ausente"), LUIS);
    expect(within(list).getAllByRole("listitem")).toHaveLength(1);
  });

  it("a reader sees only that the teacher is away — never why", async () => {
    m.listUncovered.mockResolvedValue([{ date: "2026-10-05", class: uncoveredMate(anaAway({ kind: undefined, note: undefined })) }]);
    const user = renderManager(false);
    await user.click(await screen.findByRole("tab", { name: "Por cubrir" }));
    const list = await screen.findByRole("list", { name: "Clases por cubrir" });
    expect(list).toHaveTextContent("Ausente");
    expect(list).not.toHaveTextContent("Incapacidad");
    expect(within(list).queryByRole("button", { name: /Asignar suplente/ })).not.toBeInTheDocument();
  });

  it("edits and cancels a registered absence (cancelled, never deleted)", async () => {
    m.listAbsences.mockResolvedValue([anaAway()]);
    m.cancelAbsence.mockResolvedValue(anaAway({ cancelled: true }));
    m.updateAbsence.mockResolvedValue({ absence: anaAway(), uncoveredCount: 3 });
    const user = renderManager();
    await user.click(await screen.findByRole("tab", { name: "Por cubrir" }));
    const registered = await screen.findByRole("list", { name: "Ausencias registradas" });
    expect(registered).toHaveTextContent("Ana Ruiz");
    expect(registered).toHaveTextContent("todo el día");

    await user.click(within(registered).getByRole("button", { name: "Editar la ausencia de Ana Ruiz" }));
    const dialog = screen.getByRole("dialog", { name: /Editar ausencia/ });
    await user.click(within(dialog).getByRole("button", { name: "Otro" }));
    await user.click(within(dialog).getByRole("button", { name: "Guardar cambios" }));
    await waitFor(() => expect(m.updateAbsence).toHaveBeenCalledWith(expect.any(String), ABSENCE, expect.objectContaining({ kind: "other", teacherId: ANA })));
    expect(await screen.findByText("Quedan 3 clases sin cubrir en esas fechas")).toBeInTheDocument();

    await user.click(within(registered).getByRole("button", { name: "Cancelar la ausencia de Ana Ruiz" }));
    await user.click(within(screen.getByRole("dialog", { name: /Cancelar la ausencia/ })).getByRole("button", { name: "Cancelar ausencia" }));
    await waitFor(() => expect(m.cancelAbsence).toHaveBeenCalledWith(expect.any(String), ABSENCE));
  });
});

describe("Ausencias — en la semana y en «ahora» (T19)", () => {
  it("paints the absent teacher's classes «Sin cubrir» on the week, and moves week by week", async () => {
    m.listUncovered.mockResolvedValue([{ date: "2026-10-05", class: uncoveredMate() }]);
    const user = renderManager();
    await user.click(await screen.findByRole("button", { name: "3.º" }));
    await user.click(screen.getByRole("button", { name: "A" }));
    expect(screen.getByText("Semana del 5 al 9 de octubre")).toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole("button", { name: /^Matemáticas, 3.º A, Ana Ruiz, lunes 08:00 a 08:50, sin cubrir/ })).toBeInTheDocument());
    expect(m.listUncovered).toHaveBeenCalledWith(expect.any(String), sampleBoard().cycle.id, "2026-10-05", "2026-10-09");

    await user.click(screen.getByRole("button", { name: "Semana siguiente" }));
    await waitFor(() => expect(m.listUncovered).toHaveBeenLastCalledWith(expect.any(String), sampleBoard().cycle.id, "2026-10-12", "2026-10-16"));
  });

  it("shows an uncovered class in «¿Qué está pasando ahora?» with a way to cover it", async () => {
    m.getDay.mockResolvedValue({ date: "2026-10-05", weekday: 1, nonWorking: null, classes: [uncoveredMate()] });
    const user = renderManager();
    await user.click(await screen.findByRole("tab", { name: "¿Qué está pasando ahora?" }));
    const card = (await screen.findAllByTestId("now-card"))[0] as HTMLElement;
    expect(card).toHaveTextContent("Sin cubrir");
    expect(card).toHaveTextContent("Ana Ruiz · Ausente");
    expect(within(card).getByRole("button", { name: /Asignar suplente/ })).toBeInTheDocument();
  });

  it("says a non-working day has no classes", async () => {
    m.getDay.mockResolvedValue({ date: "2026-10-05", weekday: 1, nonWorking: "Día del maestro", classes: [] });
    const user = renderManager();
    await user.click(await screen.findByRole("tab", { name: "¿Qué está pasando ahora?" }));
    expect(await screen.findByText("Día inhábil: Día del maestro. No hay clases.")).toBeInTheDocument();
  });
});

describe("Ausencias — «Cubrir ahora» desde Docentes (review T19)", () => {
  it("opens «Por cubrir» on the week of the absence, even in the past", async () => {
    render(<ScheduleScreen onBack={() => undefined} mode="manage" canWrite now={MONDAY_0810} focus={{ startsOn: "2026-09-29", endsOn: "2026-09-30" }} />);
    expect(await screen.findByRole("tab", { name: "Por cubrir" })).toHaveAttribute("aria-selected", "true");
    await waitFor(() => expect(m.listUncovered).toHaveBeenCalledWith(expect.any(String), sampleBoard().cycle.id, "2026-09-28", "2026-10-02"));
  });
});

describe("Ausencias — una suplencia cuyo suplente falta (review T19)", () => {
  it("covers it again by REPLACING the change, never a dead end", async () => {
    const OLD = "80000000-0000-4000-8000-000000000009";
    m.listUncovered.mockResolvedValue([{ date: "2026-10-05", class: { ...uncoveredMate(anaAway({ teacherId: LUIS })), teacherId: LUIS, overrideId: OLD } }]);
    const EVA = "50000000-0000-4000-8000-000000000003";
    m.getBoard.mockResolvedValue(sampleBoard({ teachers: [...sampleBoard().teachers, { id: EVA, firstName: "Eva", lastName: "Soto" }] }));
    m.listAvailableTeachers.mockResolvedValue([{ teacherId: EVA, firstName: "Eva", lastName: "Soto", match: "subject" }]);
    m.createOverride.mockResolvedValue(undefined);
    const user = renderManager();
    await user.click(await screen.findByRole("tab", { name: "Por cubrir" }));
    const list = await screen.findByRole("list", { name: "Clases por cubrir" });
    await user.click(within(list).getByRole("button", { name: /Asignar suplente/ }));
    const dialog = screen.getByRole("dialog", { name: /Asignar suplente/ });
    expect(dialog).toHaveTextContent("Reemplaza el cambio que ya tenía ese día");
    await waitFor(() => expect(within(dialog).getByRole("option", { name: "Eva Soto" })).toBeInTheDocument());
    await user.selectOptions(within(dialog).getByLabelText(/^Suplente/), EVA);
    await user.click(within(dialog).getByRole("button", { name: "Asignar suplente" }));
    await waitFor(() => expect(m.createOverride).toHaveBeenCalledWith(expect.any(String), expect.objectContaining({ replaceOverrideId: OLD, teacherId: EVA })));
  });
});

describe("Ausencias — validar al salir del campo (review T19)", () => {
  it("says the end date is before the start when leaving the field", async () => {
    const user = renderManager();
    const dialog = await openAbsenceDrawer(user);
    const until = within(dialog).getByLabelText(/^Hasta el/);
    fireEvent.change(until, { target: { value: "2026-10-01" } });
    fireEvent.blur(until);
    expect(within(dialog).getByText(/no puede ser antes/)).toBeInTheDocument();
  });
});
