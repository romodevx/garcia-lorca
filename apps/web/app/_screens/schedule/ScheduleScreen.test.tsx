import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as api from "../../schedule/api";
import { ANA, G1SA, G3A, LUIS, MATE, PRIMARIA, SECUNDARIA, sampleBoard } from "../../schedule/__fixtures__/board";
import { ScheduleEntry } from "./ScheduleEntry";
import { ScheduleScreen } from "./ScheduleScreen";

vi.mock("../../schedule/api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../../schedule/api")>();
  return {
    ...real,
    getBoard: vi.fn(),
    getOwnBoard: vi.fn(),
    getDay: vi.fn(),
    getOwnDay: vi.fn(),
    createBlock: vi.fn(),
    updateBlock: vi.fn(),
    deleteBlock: vi.fn(),
    createOverride: vi.fn(),
    deleteOverride: vi.fn(),
    listAvailableTeachers: vi.fn(),
    putSettings: vi.fn(),
    listEvents: vi.fn(),
  };
});
const m = vi.mocked(api);

/** Monday 2026-10-05, 08:10 at the school (UTC−6). */
const MONDAY_0810 = new Date("2026-10-05T14:10:00Z");
const BLOCK_MATE = "70000000-0000-4000-8000-000000000001";

beforeEach(() => {
  m.getBoard.mockResolvedValue(sampleBoard());
  m.getOwnBoard.mockResolvedValue(sampleBoard());
  m.listEvents.mockResolvedValue([]);
  m.getDay.mockResolvedValue({ date: "2026-10-05", weekday: 1, nonWorking: null, classes: [] });
  m.getOwnDay.mockResolvedValue({ date: "2026-10-05", weekday: 1, nonWorking: null, classes: [] });
  m.listAvailableTeachers.mockResolvedValue([]);
});
afterEach(() => vi.clearAllMocks());

function renderManager(canWrite = true): ReturnType<typeof userEvent.setup> {
  const user = userEvent.setup();
  render(<ScheduleScreen onBack={() => undefined} mode="manage" canWrite={canWrite} now={MONDAY_0810} />);
  return user;
}

/** jsdom has no hour/minute segments: a typed time arrives as one change, then the blur. */
async function typeTime(user: ReturnType<typeof userEvent.setup>, input: HTMLElement, value: string): Promise<void> {
  await user.click(input);
  fireEvent.change(input, { target: { value } });
  await user.tab();
}

const blocks = (): HTMLElement[] => screen.queryAllByTestId("schedule-block");

async function choosePrimaria(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await user.click(await screen.findByRole("button", { name: "Primaria" }));
}

async function chooseGroup3A(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await choosePrimaria(user);
  await user.click(screen.getByRole("button", { name: "3.º" }));
  await user.click(screen.getByRole("button", { name: "A" }));
}

describe("Horarios — la semana (T16)", () => {
  // ISSUE-T14-016: it opened on the first level's grades ("1°" of Maternal) though nobody chose a level.
  it("with several levels, starts with none chosen and no Grado row", async () => {
    renderManager();
    await screen.findByRole("button", { name: "Primaria" });
    expect(screen.getByRole("button", { name: "Primaria" })).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByRole("button", { name: "Secundaria" })).toHaveAttribute("aria-pressed", "false");
    expect(screen.queryByRole("group", { name: "Grado" })).toBeNull();
  });

  it("with a single level, starts on it and shows its grades", async () => {
    const board = sampleBoard();
    m.getBoard.mockResolvedValue(sampleBoard({ levels: [board.levels[0]!] }));
    renderManager();
    expect(await screen.findByRole("button", { name: "Primaria" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "3.º" })).toBeInTheDocument();
  });

  it("shows the chosen group's week, Monday to Friday", async () => {
    const user = renderManager();
    await chooseGroup3A(user);
    expect(blocks()).toHaveLength(2);
    expect(screen.getByRole("button", { name: "Matemáticas, 3.º A, Ana Ruiz, lunes 08:00 a 08:50" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Español, 3.º A, Luis Pérez, lunes 08:50 a 09:40" })).toBeInTheDocument();
    for (const day of ["Lunes", "Martes", "Miércoles", "Jueves", "Viernes"]) expect(screen.getByText(day)).toBeInTheDocument();
  });

  it("shows a teacher's week across groups", async () => {
    const user = renderManager();
    await screen.findByRole("button", { name: "Primaria" });
    await user.selectOptions(screen.getByLabelText("Docente"), ANA);
    expect(blocks()).toHaveLength(2);
    expect(blocks().every((b) => /Ana Ruiz/.test(b.getAttribute("aria-label") ?? ""))).toBe(true);
  });

  // ISSUE-CC-122: sorted by last name but shown "first last", the list read as unsorted.
  it("lists the teachers in the order their names are shown", async () => {
    renderManager();
    await screen.findByRole("button", { name: "Primaria" });
    const names = within(screen.getByLabelText("Docente")).getAllByRole("option").map((o) => o.textContent).filter((t) => t !== "");
    expect(names).toEqual(["Ana Ruiz", "Luis Pérez"]);
  });

  it("says what to do when a group has no classes yet", async () => {
    m.getBoard.mockResolvedValue(sampleBoard({ blocks: [] }));
    const user = renderManager();
    await chooseGroup3A(user);
    expect(screen.getByText(/Este grupo aún no tiene clases — haz clic en un hueco para agregar una/)).toBeInTheDocument();
  });

  it("sends people to Docentes when the cycle has no teaching assignments", async () => {
    m.getBoard.mockResolvedValue(sampleBoard({ assignments: [], blocks: [] }));
    renderManager();
    expect(await screen.findByText(/asignaciones/i)).toHaveTextContent(/Docentes/);
  });

  it("creates a class from an empty slot with the group's assignment", async () => {
    m.createBlock.mockResolvedValue(sampleBoard().blocks[0]!);
    const user = renderManager();
    await chooseGroup3A(user);
    await user.click(screen.getByRole("button", { name: "Agregar clase el martes a las 09:00" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByLabelText("Inicio")).toHaveValue("09:00");
    expect(within(dialog).getByLabelText("Fin")).toHaveValue("09:50");
    // Only 3.º A's own assignments are offered.
    const options = within(within(dialog).getByLabelText("Asignatura — Docente")).getAllByRole("option").map((o) => o.textContent);
    expect(options).toEqual(["", "Español — Luis Pérez", "Matemáticas — Ana Ruiz"]);
    await user.selectOptions(within(dialog).getByLabelText("Asignatura — Docente"), "Matemáticas — Ana Ruiz");
    await user.click(within(dialog).getByRole("button", { name: "Guardar clase" }));
    await waitFor(() =>
      expect(m.createBlock).toHaveBeenCalledWith(expect.any(String), {
        assignmentId: "60000000-0000-4000-8000-000000000001", weekday: 2, startsAt: "09:00", endsAt: "09:50",
      }),
    );
  });

  it("ISSUE-028 · a class's start and end are typed, within the level's hours", async () => {
    const user = renderManager();
    await chooseGroup3A(user);
    await user.click(screen.getByRole("button", { name: "Agregar clase el martes a las 09:00" }));
    const dialog = screen.getByRole("dialog");
    const start = within(dialog).getByLabelText("Inicio");
    expect(start).toHaveAttribute("type", "time");
    expect(start).toHaveAttribute("min", "07:00");
    expect(start).toHaveAttribute("max", "13:55");
    expect(within(dialog).getByLabelText("Fin")).toHaveAttribute("max", "14:00");
    await typeTime(user, start, "06:00");
    expect(start).toHaveAccessibleDescription("Escribe una hora entre 07:00 y 13:55.");
    await user.selectOptions(within(dialog).getByLabelText("Asignatura — Docente"), "Matemáticas — Ana Ruiz");
    await user.click(within(dialog).getByRole("button", { name: "Guardar clase" }));
    expect(m.createBlock).not.toHaveBeenCalled();
  });

  it("ISSUE-028 · the class's hours follow the level of the group chosen in the drawer", async () => {
    m.getBoard.mockResolvedValue(sampleBoard({ settings: [
      { levelId: PRIMARIA, gridStart: "07:00", gridEnd: "14:00" },
      { levelId: SECUNDARIA, gridStart: "13:00", gridEnd: "20:00" },
    ] }));
    const user = renderManager();
    await chooseGroup3A(user);
    await user.click(screen.getByRole("button", { name: "Agregar clase el martes a las 09:00" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByLabelText("Inicio")).toHaveAttribute("min", "07:00");
    await user.selectOptions(within(dialog).getByLabelText("Grupo"), G1SA);
    expect(within(dialog).getByLabelText("Inicio")).toHaveAttribute("min", "13:00");
    expect(within(dialog).getByLabelText("Fin")).toHaveAttribute("max", "20:00");
  });

  // ISSUE-T14-205: imported data mixes "1° B" and "1º B"; without the level two
  // groups read alike. Every option names grade, level and letter, school order.
  it("T14-205 · the group picker names each group with its level, in school order", async () => {
    m.getBoard.mockResolvedValue(sampleBoard());
    const user = renderManager();
    await chooseGroup3A(user);
    await user.click(screen.getByRole("button", { name: "Agregar clase el martes a las 09:00" }));
    const picker = within(screen.getByRole("dialog")).getByLabelText("Grupo");
    const labels = within(picker).getAllByRole("option").map((o) => o.textContent).filter((t) => t !== "" && t !== "Elige una opción");

    expect(labels).toEqual(["3.º Primaria · A", "3.º Primaria · B", "1.º Secundaria · A"]);
  });

  it("ISSUE-028 · a class saved at an hour the picker would not offer keeps it", async () => {
    const board = sampleBoard();
    const early = { ...board.blocks[0]!, startsAt: "06:32", endsAt: "07:20" };
    m.getBoard.mockResolvedValue(sampleBoard({ blocks: [early, ...board.blocks.slice(1)] }));
    m.updateBlock.mockResolvedValue(early);
    const user = renderManager();
    await chooseGroup3A(user);
    await user.click(screen.getByRole("button", { name: /^Matemáticas, 3.º A/ }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByLabelText("Inicio")).toHaveValue("06:32");
    await user.click(within(dialog).getByRole("button", { name: "Guardar clase" }));
    await waitFor(() => expect(m.updateBlock).toHaveBeenCalledWith(expect.any(String), early.id, expect.objectContaining({ startsAt: "06:32", endsAt: "07:20" })));
  });

  it("keeps the drawer open and shows the server's sentence on a conflict", async () => {
    m.createBlock.mockRejectedValue(new api.ScheduleError(409, "Ana Ruiz ya da Matemáticas a 3.º B el martes de 09:00 a 09:50", "SCHEDULE_TEACHER_BUSY"));
    const user = renderManager();
    await chooseGroup3A(user);
    await user.click(screen.getByRole("button", { name: "Agregar clase el martes a las 09:00" }));
    const dialog = screen.getByRole("dialog");
    await user.selectOptions(within(dialog).getByLabelText("Asignatura — Docente"), "Matemáticas — Ana Ruiz");
    await user.click(within(dialog).getByRole("button", { name: "Guardar clase" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("Ana Ruiz ya da Matemáticas a 3.º B el martes de 09:00 a 09:50");
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  // ISSUE-T14-015: a school that starts mid-cycle builds its timetable after
  // the start; adding a class changes nothing anyone relied on.
  it("once the cycle has started, adding a class asks for no reason", async () => {
    m.getBoard.mockResolvedValue(sampleBoard({ started: true }));
    m.createBlock.mockResolvedValue(sampleBoard().blocks[0]!);
    const user = renderManager();
    await chooseGroup3A(user);
    await user.click(screen.getByRole("button", { name: "Agregar clase el martes a las 09:00" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).queryByLabelText("Motivo del cambio")).toBeNull();
    await user.selectOptions(within(dialog).getByLabelText("Asignatura — Docente"), "Matemáticas — Ana Ruiz");
    await user.click(within(dialog).getByRole("button", { name: "Guardar clase" }));
    await waitFor(() => expect(m.createBlock).toHaveBeenCalledTimes(1));
    expect(m.createBlock.mock.calls[0]![1]).not.toHaveProperty("reason");
  });

  it("once the cycle has started, changing a class asks for a reason", async () => {
    m.getBoard.mockResolvedValue(sampleBoard({ started: true }));
    const user = renderManager();
    await chooseGroup3A(user);
    await user.click(screen.getByRole("button", { name: /^Matemáticas, 3.º A/ }));
    const dialog = screen.getByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: "Guardar clase" }));
    expect(within(dialog).getByLabelText("Motivo del cambio")).toHaveAttribute("aria-invalid", "true");
    expect(m.updateBlock).not.toHaveBeenCalled();
  });

  // The drawer's form is noValidate (T13): the date's min/max no longer stop a
  // day outside the cycle by themselves, so the screen says it.
  it("refuses a one-date change outside the cycle, on blur and on save", async () => {
    const user = renderManager();
    await chooseGroup3A(user);
    await user.click(screen.getByRole("button", { name: /^Matemáticas, 3.º A/ }));
    const dialog = screen.getByRole("dialog");
    await user.click(within(dialog).getByRole("tab", { name: "Cambio solo para una fecha" }));
    const date = within(dialog).getByLabelText("Fecha");
    // A Monday, as the class, but after the cycle ends.
    await user.click(date);
    fireEvent.change(date, { target: { value: "2027-07-12" } });
    await user.tab();
    expect(date).toHaveAccessibleDescription("Elige un día del ciclo: del 2026-08-24 al 2027-07-09.");
    await user.type(within(dialog).getByLabelText(/Motivo/), "Consulta médica");
    await user.click(within(dialog).getByRole("button", { name: "Registrar cambio" }));
    expect(m.createOverride).not.toHaveBeenCalled();
  });

  it("opens a class to edit it and to change it for one date", async () => {
    m.listAvailableTeachers.mockResolvedValue([{ teacherId: LUIS, firstName: "Luis", lastName: "Pérez", match: "grade" }]);
    m.createOverride.mockResolvedValue(undefined);
    const user = renderManager();
    await chooseGroup3A(user);
    await user.click(screen.getByRole("button", { name: /^Matemáticas, 3.º A/ }));
    const dialog = screen.getByRole("dialog");
    await user.click(within(dialog).getByRole("tab", { name: "Cambio solo para una fecha" }));
    // Monday: today is that weekday, so the date defaults to today.
    expect(within(dialog).getByLabelText("Fecha")).toHaveValue("2026-10-05");
    await waitFor(() => expect(m.listAvailableTeachers).toHaveBeenCalledWith(expect.any(String), expect.objectContaining({
      groupId: G3A, subjectId: MATE, date: "2026-10-05", startsAt: "08:00", endsAt: "08:50", excludeBlockId: BLOCK_MATE,
    })));
    await user.selectOptions(await within(dialog).findByLabelText("Suplente"), LUIS);
    await user.type(within(dialog).getByLabelText(/Motivo/), "Consulta médica");
    await user.click(within(dialog).getByRole("button", { name: "Registrar cambio" }));
    await waitFor(() =>
      expect(m.createOverride).toHaveBeenCalledWith(expect.any(String), {
        blockId: BLOCK_MATE, date: "2026-10-05", kind: "substitute", teacherId: LUIS, reason: "Consulta médica",
      }),
    );
  });
});

describe("Horarios — ¿Qué está pasando ahora? (T16)", () => {
  it("shows the classes under way at that minute as cards, a substitute named as such", async () => {
    m.getDay.mockResolvedValue({
      date: "2026-10-05", weekday: 1, nonWorking: null, classes: [
        { blockId: BLOCK_MATE, groupId: G3A, subjectId: MATE, teacherId: LUIS, startsAt: "08:00", endsAt: "08:50", status: "substitute", baseTeacherId: ANA, baseStartsAt: "08:00", baseEndsAt: "08:50", overrideId: "80000000-0000-4000-8000-000000000001", reason: "Consulta", absence: null },
        { blockId: "x", groupId: G3A, subjectId: MATE, teacherId: ANA, startsAt: "09:00", endsAt: "09:50", status: "regular", baseTeacherId: ANA, baseStartsAt: "09:00", baseEndsAt: "09:50", overrideId: null, reason: null, absence: null },
      ],
    });
    const user = renderManager();
    await user.click(await screen.findByRole("tab", { name: "¿Qué está pasando ahora?" }));
    const cards = await screen.findAllByTestId("now-card");
    expect(cards).toHaveLength(1);
    expect(cards[0]).toHaveTextContent("Suplente: Luis Pérez (en lugar de Ana Ruiz)");
    expect(cards[0]).toHaveTextContent("Suplencia");
    expect(m.getDay).toHaveBeenCalledWith(expect.any(String), sampleBoard().cycle.id, "2026-10-05");
  });

  // ISSUE-CC-122: the hour picker offered every five minutes from 00:00.
  it("offers only the hours of the school day, plus the current minute", async () => {
    const user = renderManager();
    await user.click(await screen.findByRole("tab", { name: "¿Qué está pasando ahora?" }));
    const hours = within(await screen.findByLabelText("Hora", { selector: "select" })).getAllByRole("option").map((o) => o.textContent).filter((t) => t !== "");
    expect(hours[0]).toBe("07:00");
    expect(hours.at(-1)).toBe("14:00");
    expect(hours).not.toContain("00:00");
  });

  it("says so when nothing is happening", async () => {
    const user = renderManager();
    await user.click(await screen.findByRole("tab", { name: "¿Qué está pasando ahora?" }));
    expect(await screen.findByText("No hay clases a esta hora")).toBeInTheDocument();
  });
});

describe("Horarios — el docente (T16)", () => {
  it("reads only the teacher's own timetable, without editing", async () => {
    render(<ScheduleScreen onBack={() => undefined} mode="teacher" canWrite={false} now={MONDAY_0810} />);
    expect(await screen.findByLabelText(/^Matemáticas, 3.º A, Ana Ruiz/)).toBeInTheDocument();
    expect(m.getOwnBoard).toHaveBeenCalled();
    expect(m.getBoard).not.toHaveBeenCalled();
    expect(screen.queryByRole("button", { name: /Agregar clase/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Matemáticas, 3.º A/ })).not.toBeInTheDocument();
  });
});

describe("Horarios — la entrada (T16)", () => {
  it("shows the placeholder when module_schedule is off", () => {
    render(<ScheduleEntry enabled={false} permissions={["schedule:read"]} onBack={() => undefined} />);
    expect(screen.getByRole("heading", { name: /Módulo «Horarios»/ })).toBeInTheDocument();
    expect(m.getBoard).not.toHaveBeenCalled();
  });

  it("gives a teacher without schedule:read their own timetable", async () => {
    render(<ScheduleEntry enabled permissions={["teaching:read"]} onBack={() => undefined} />);
    await waitFor(() => expect(m.getOwnBoard).toHaveBeenCalled());
    expect(m.getBoard).not.toHaveBeenCalled();
  });
});

describe("Horarios — cambios reversibles (T16)", () => {
  it("removes a class with an undo before anything reaches the server", async () => {
    const user = renderManager();
    await chooseGroup3A(user);
    await user.click(screen.getByRole("button", { name: /^Matemáticas, 3.º A/ }));
    await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Quitar clase" }));
    expect(screen.queryByRole("button", { name: /^Matemáticas, 3.º A/ })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Deshacer" }));
    expect(screen.getByRole("button", { name: /^Matemáticas, 3.º A/ })).toBeInTheDocument();
    expect(m.deleteBlock).not.toHaveBeenCalled();
  });

  // Gate B1 on #142: closing the tab must not cancel the delete.
  it("closing the tab sends the class's delete with keepalive", async () => {
    m.deleteBlock.mockResolvedValue(undefined);
    const user = renderManager();
    await chooseGroup3A(user);
    await user.click(screen.getByRole("button", { name: /^Matemáticas, 3.º A/ }));
    await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Quitar clase" }));

    window.dispatchEvent(new Event("pagehide"));

    expect(m.deleteBlock).toHaveBeenCalledWith(expect.any(String), BLOCK_MATE, undefined, { keepalive: true });
  });

  // Gate nit on #142: once the delete is confirmed the class stays hidden
  // until the board says so, instead of reappearing (and inviting a 2nd DELETE).
  it("keeps a removed class hidden once its delete is sent", async () => {
    m.deleteBlock.mockResolvedValue(undefined);
    const user = renderManager();
    await chooseGroup3A(user);
    await user.click(screen.getByRole("button", { name: /^Matemáticas, 3.º A/ }));
    await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Quitar clase" }));

    await user.click(screen.getByRole("button", { name: "Cerrar aviso" }));

    await waitFor(() => expect(m.deleteBlock).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(m.getBoard.mock.calls.length).toBeGreaterThan(1));
    expect(screen.queryByRole("button", { name: /^Matemáticas, 3.º A/ })).not.toBeInTheDocument();
  });

  it("undoes a date's change from the day list", async () => {
    m.deleteOverride.mockResolvedValue(undefined);
    m.getDay.mockResolvedValue({
      date: "2026-10-05", weekday: 1, nonWorking: null, classes: [
        { blockId: BLOCK_MATE, groupId: G3A, subjectId: MATE, teacherId: LUIS, startsAt: "08:00", endsAt: "08:50", status: "substitute", baseTeacherId: ANA, baseStartsAt: "08:00", baseEndsAt: "08:50", overrideId: "80000000-0000-4000-8000-000000000001", reason: "Consulta", absence: null },
      ],
    });
    const user = renderManager();
    await user.click(await screen.findByRole("tab", { name: "¿Qué está pasando ahora?" }));
    await user.click(await screen.findByRole("button", { name: "Deshacer el cambio de Matemáticas, 3.º A, 08:00" }));
    await waitFor(() => expect(m.deleteOverride).toHaveBeenCalledWith(expect.any(String), "80000000-0000-4000-8000-000000000001"));
  });

  it("a reader sees the day's changes but cannot undo them", async () => {
    m.getDay.mockResolvedValue({
      date: "2026-10-05", weekday: 1, nonWorking: null, classes: [
        { blockId: BLOCK_MATE, groupId: G3A, subjectId: MATE, teacherId: ANA, startsAt: "08:00", endsAt: "08:50", status: "cancelled", baseTeacherId: ANA, baseStartsAt: "08:00", baseEndsAt: "08:50", overrideId: "80000000-0000-4000-8000-000000000001", reason: "Junta", absence: null },
      ],
    });
    const user = renderManager(false);
    await user.click(await screen.findByRole("tab", { name: "¿Qué está pasando ahora?" }));
    expect((await screen.findAllByText("Cancelada")).length).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: /Deshacer el cambio/ })).not.toBeInTheDocument();
  });
});

describe("Horarios — historial y horas del nivel (T16)", () => {
  it("lists who changed what, when and why", async () => {
    m.listEvents.mockResolvedValue([
      { id: "e1", at: "2026-10-01T15:00:00Z", actorId: "u", actorName: "Dirección", action: "override_created", blockId: BLOCK_MATE, overrideId: null, date: "2026-10-05", reason: "Gripe", summary: "Suplencia el 2026-10-05: Matemáticas, 3.º A" },
    ]);
    const user = renderManager();
    await user.click(await screen.findByRole("tab", { name: "Historial" }));
    const history = await screen.findByRole("list", { name: "Historial de cambios" });
    expect(history).toHaveTextContent("Suplencia el 2026-10-05: Matemáticas, 3.º A");
    expect(history).toHaveTextContent("Dirección");
    expect(history).toHaveTextContent("Motivo: Gripe");
  });

  it("refuses a level day shorter than an hour without asking the server", async () => {
    const user = renderManager();
    await choosePrimaria(user);
    await typeTime(user, screen.getByLabelText("Termina"), "07:30");
    expect(screen.getByLabelText("Termina")).toHaveAttribute("aria-invalid", "true");
    await user.click(screen.getByRole("button", { name: "Guardar horas" }));
    expect(m.putSettings).not.toHaveBeenCalled();
  });

  it("saves the level's hours", async () => {
    m.putSettings.mockResolvedValue({ levelId: "x", gridStart: "07:00", gridEnd: "15:00" });
    const user = renderManager();
    await choosePrimaria(user);
    await typeTime(user, screen.getByLabelText("Termina"), "15:00");
    await user.click(screen.getByRole("button", { name: "Guardar horas" }));
    await waitFor(() => expect(m.putSettings).toHaveBeenCalledWith(expect.any(String), sampleBoard().levels[0]!.id, { gridStart: "07:00", gridEnd: "15:00" }));
  });
});
