import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { EvaluationPeriodRef, GradeSheetClient, PeriodSheet, SavedPeriodGrade } from "../../grades/periodSheet";
import { GradeRequestError } from "../../grades/requestError";
import { PeriodGradeSheet } from "./PeriodGradeSheet";
import { useGradeSaves } from "./useGradeSaves";

const P1: EvaluationPeriodRef = { id: "p1", ordinal: 1, name: "1.er trimestre", startsOn: "2026-08-24", endsOn: "2026-11-30" };
const P2: EvaluationPeriodRef = { id: "p2", ordinal: 2, name: "2.º trimestre", startsOn: "2026-12-07", endsOn: "2027-03-26" };
const P3: EvaluationPeriodRef = { id: "p3", ordinal: 3, name: "3.er trimestre", startsOn: "2027-04-12", endsOn: "2027-07-09" };

/** Mid-January: the 1st period is over, the 2nd is current, the 3rd has not started. */
const SHEET: PeriodSheet = {
  assignmentId: "a1",
  periodId: "p2",
  periods: [P1, P2, P3],
  currentPeriodId: "p2",
  today: "2027-01-15",
  closedPeriodIds: [],
  scale: { min: 5, max: 10, decimals: 0, passing: 6 },
  gradingEnabled: true,
  editable: true,
  rows: [
    { enrollmentId: "en1", studentId: "st1", name: "Ada Lovelace", value: 9, updatedAt: "2026-12-10T10:00:00Z", rowEditable: true },
    { enrollmentId: "en2", studentId: "st2", name: "Alan Turing", value: null, updatedAt: null, rowEditable: true },
    { enrollmentId: "en3", studentId: "st3", name: "Grace Hopper", value: null, updatedAt: null, rowEditable: true },
  ],
};

const FIRST: PeriodSheet = { ...SHEET, periodId: "p1" };

interface Deferred<T> {
  promise: Promise<T>;
  resolve: (v: T) => void;
  reject: (e: unknown) => void;
}

function deferred<T>(): Deferred<T> {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

type MockClient = { [K in keyof GradeSheetClient]: ReturnType<typeof vi.fn> & GradeSheetClient[K] };

function fakeClient(sheet: PeriodSheet = SHEET): MockClient {
  return {
    load: vi.fn().mockResolvedValue(sheet),
    save: vi.fn().mockImplementation(
      async (i: { enrollmentId: string; periodId: string; value: number }): Promise<SavedPeriodGrade> => ({ ...i, updatedAt: "2027-01-12T10:00:00Z" }),
    ),
    remove: vi.fn().mockResolvedValue(undefined),
  } as MockClient;
}

const input = (name: string): HTMLInputElement => screen.getByRole("textbox", { name: `Calificación de ${name}` });

/** The capture screens own the save store, above the sheet — as here. */
function Screen({ client }: { client: GradeSheetClient }) {
  const saves = useGradeSaves(client);
  return <PeriodGradeSheet assignmentId="a1" client={client} saves={saves} />;
}

async function renderSheet(client: MockClient = fakeClient()): Promise<MockClient> {
  render(<Screen client={client} />);
  await screen.findByRole("table", { name: "Calificaciones del periodo" });
  return client;
}

afterEach(() => {
  vi.clearAllMocks();
  vi.restoreAllMocks();
});

const chip = (name: RegExp): HTMLElement => screen.getByRole("button", { name });
const FIRST_CHIP = /^1\.er trimestre/;
const SECOND_CHIP = /^2\.º trimestre/;
const THIRD_CHIP = /^3\.er trimestre/;

describe("PeriodGradeSheet — loading", () => {
  it("asks the server for the current period and shows one whole-number field per student", async () => {
    const client = await renderSheet();

    expect(client.load).toHaveBeenCalledWith({ assignmentId: "a1" });
    expect(input("Ada Lovelace")).toHaveValue("9");
    expect(input("Alan Turing")).toHaveValue("");
    expect(input("Ada Lovelace")).toHaveAttribute("inputmode", "numeric");
    expect(chip(SECOND_CHIP)).toHaveAttribute("aria-pressed", "true");
    expect(chip(FIRST_CHIP)).toHaveAttribute("aria-pressed", "false");
  });

  it("names each period with its dates and marks the current one", async () => {
    await renderSheet();

    expect(chip(FIRST_CHIP)).toHaveTextContent("24 ago – 30 nov");
    expect(chip(SECOND_CHIP)).toHaveAttribute("data-current", "true");
    expect(chip(SECOND_CHIP)).toHaveTextContent("En curso");
    expect(chip(FIRST_CHIP)).not.toHaveAttribute("data-current");
  });

  it("counts captured and failing, and averages the exact mean truncated to 2 decimals", async () => {
    await renderSheet(fakeClient({ ...SHEET, rows: [SHEET.rows[0]!, { ...SHEET.rows[1]!, value: 9 }, { ...SHEET.rows[2]!, value: 8 }] }));

    const counters = screen.getByRole("group", { name: "Resumen del periodo" });
    expect(counters).toHaveTextContent("Capturadas3 de 3");
    expect(counters).toHaveTextContent("Promedio8.66");
    expect(counters).toHaveTextContent("Reprobados0");
  });

  it("counts a failing grade and prints an average without trailing zeros", async () => {
    await renderSheet(fakeClient({ ...SHEET, rows: [...SHEET.rows.slice(0, 2), { ...SHEET.rows[2]!, value: 5 }] }));

    const counters = screen.getByRole("group", { name: "Resumen del periodo" });
    expect(counters).toHaveTextContent("Capturadas2 de 3");
    expect(counters).toHaveTextContent(/Promedio7Reprobados/);
    expect(counters).toHaveTextContent("Reprobados1");
  });

  it("says when a level has no numeric grade", async () => {
    const client = fakeClient();
    client.load.mockRejectedValue(new GradeRequestError(400, "Este nivel no lleva calificación numérica"));
    render(<Screen client={client} />);

    expect(await screen.findByText("Este nivel no lleva calificación numérica")).toBeInTheDocument();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("a period that has not started cannot be chosen, and says so", async () => {
    await renderSheet();

    const third = chip(THIRD_CHIP);
    expect(third).toBeDisabled();
    expect(third).toHaveAttribute("data-not-started", "true");
    expect(third).toHaveTextContent("Aún no empieza");
    expect(chip(FIRST_CHIP)).not.toBeDisabled();
  });

  it("a grade without evaluation periods points to where they are configured", async () => {
    const none: PeriodSheet = { ...SHEET, periodId: null, periods: [], currentPeriodId: null, editable: false, readOnlyReason: "Este grado no tiene periodos de evaluación" };
    render(<Screen client={fakeClient(none)} />);

    expect(await screen.findByText("Este grado no tiene periodos de evaluación configurados")).toBeInTheDocument();
    expect(screen.getByText(/Configuración › Periodos por grado/)).toBeInTheDocument();
    expect(screen.queryByRole("table")).toBeNull();
    expect(screen.queryByRole("textbox")).toBeNull();
  });

  it("offers a retry when the sheet fails to load", async () => {
    const user = userEvent.setup();
    const client = fakeClient();
    client.load.mockRejectedValueOnce(new Error("boom"));
    render(<Screen client={client} />);

    await user.click(await screen.findByRole("button", { name: "Reintentar" }));
    expect(await screen.findByRole("table", { name: "Calificaciones del periodo" })).toBeInTheDocument();
  });

  it("is read-only when the server says so, and says why", async () => {
    await renderSheet(fakeClient({ ...SHEET, editable: false, readOnlyReason: "Aún no empieza ese periodo" }));

    expect(screen.getByText("Aún no empieza ese periodo")).toBeInTheDocument();
    expect(screen.queryByRole("textbox")).toBeNull();
    expect(within(screen.getByRole("table")).getByText("9")).toBeInTheDocument();
  });
});

describe("PeriodGradeSheet — keyboard capture", () => {
  it("Enter saves the cell and moves to the next student", async () => {
    const user = userEvent.setup();
    const client = await renderSheet();

    await user.click(input("Alan Turing"));
    await user.keyboard("9{Enter}");

    expect(client.save).toHaveBeenCalledWith({ assignmentId: "a1", periodId: "p2", enrollmentId: "en2", value: 9 });
    expect(input("Grace Hopper")).toHaveFocus();
    expect(await screen.findByText("Guardado")).toBeInTheDocument();
    expect(client.save).toHaveBeenCalledTimes(1);
  });

  it("Tab saves on leaving and moves to the next student; Shift+Tab goes back", async () => {
    const user = userEvent.setup();
    const client = await renderSheet();

    await user.click(input("Ada Lovelace"));
    await user.keyboard("{Tab}");
    expect(input("Alan Turing")).toHaveFocus();
    expect(client.save).not.toHaveBeenCalled();

    await user.keyboard("8{Tab}");
    expect(input("Grace Hopper")).toHaveFocus();
    expect(client.save).toHaveBeenCalledWith({ assignmentId: "a1", periodId: "p2", enrollmentId: "en2", value: 8 });

    await user.keyboard("{Shift>}{Tab}{/Shift}");
    expect(input("Alan Turing")).toHaveFocus();
  });

  it("an emptied cell is deleted", async () => {
    const user = userEvent.setup();
    const client = await renderSheet();

    await user.clear(input("Ada Lovelace"));
    await user.keyboard("{Tab}");

    expect(client.remove).toHaveBeenCalledWith({ assignmentId: "a1", periodId: "p2", enrollmentId: "en1" });
    expect(client.save).not.toHaveBeenCalled();
  });
});

describe("PeriodGradeSheet — validation", () => {
  it("checks the range on leaving, never while typing, and clears on a valid keystroke", async () => {
    const user = userEvent.setup();
    const client = await renderSheet();

    await user.type(input("Alan Turing"), "11");
    expect(screen.queryByText("La calificación debe estar entre 5 y 10")).toBeNull();

    await user.tab();
    expect(await screen.findByText("La calificación debe estar entre 5 y 10")).toBeInTheDocument();
    expect(input("Alan Turing")).toHaveAttribute("aria-invalid", "true");
    expect(client.save).not.toHaveBeenCalled();

    await user.clear(input("Alan Turing"));
    await user.type(input("Alan Turing"), "7");
    expect(screen.queryByText("La calificación debe estar entre 5 y 10")).toBeNull();
  });

  it("refuses any decimal on leaving — a comma too — and never rounds it", async () => {
    const user = userEvent.setup();
    const client = await renderSheet();

    await user.type(input("Alan Turing"), "9,5");
    expect(screen.queryByText("Solo se permiten calificaciones enteras")).toBeNull();
    await user.tab();

    expect(await screen.findByText("Solo se permiten calificaciones enteras")).toBeInTheDocument();
    expect(input("Alan Turing")).toHaveValue("9,5");
    expect(client.save).not.toHaveBeenCalled();
  });

  it("announces the error it shows on leaving the field", async () => {
    const user = userEvent.setup();
    await renderSheet();

    await user.type(input("Alan Turing"), "1e1");
    await user.tab();

    expect(await screen.findByRole("alert")).toHaveTextContent("La calificación debe ser un número");
  });

  it("shows the server's rejection on its row, without a retry", async () => {
    const user = userEvent.setup();
    const client = fakeClient();
    client.save.mockRejectedValue(new GradeRequestError(400, "Solo se permiten calificaciones enteras", "GRADE_NOT_INTEGER"));
    await renderSheet(client);

    await user.type(input("Alan Turing"), "9{Enter}");

    const row = screen.getByRole("row", { name: /Alan Turing/ });
    expect(await within(row).findByText("Solo se permiten calificaciones enteras")).toBeInTheDocument();
    expect(within(row).queryByRole("button", { name: "Error — reintentar" })).toBeNull();
  });
});

describe("PeriodGradeSheet — saving status", () => {
  it("shows Guardando… and guards leaving the page while a save is in flight", async () => {
    const user = userEvent.setup();
    const client = fakeClient();
    const pending = deferred<SavedPeriodGrade>();
    client.save.mockReturnValue(pending.promise);
    await renderSheet(client);

    await user.type(input("Alan Turing"), "8{Enter}");

    const row = screen.getByRole("row", { name: /Alan Turing/ });
    expect(within(row).getByText("Guardando…")).toBeInTheDocument();
    expect(screen.getByText("Guardando 1 calificación…")).toBeInTheDocument();
    const leaving = new Event("beforeunload", { cancelable: true });
    window.dispatchEvent(leaving);
    expect(leaving.defaultPrevented).toBe(true);

    await act(async () => pending.resolve({ enrollmentId: "en2", periodId: "p2", value: 8, updatedAt: "x" }));

    expect(within(row).getByText("Guardado")).toBeInTheDocument();
    expect(screen.getByText("Todo guardado")).toBeInTheDocument();
    const later = new Event("beforeunload", { cancelable: true });
    window.dispatchEvent(later);
    expect(later.defaultPrevented).toBe(false);
  });

  it("a failed save offers a retry that sends the same value again", async () => {
    const user = userEvent.setup();
    const client = fakeClient();
    client.save.mockRejectedValueOnce(new Error("network"));
    await renderSheet(client);

    await user.type(input("Alan Turing"), "8{Enter}");
    await user.click(await screen.findByRole("button", { name: "Error — reintentar" }));

    await waitFor(() => expect(client.save).toHaveBeenCalledTimes(2));
    expect(client.save).toHaveBeenLastCalledWith({ assignmentId: "a1", periodId: "p2", enrollmentId: "en2", value: 8 });
    expect(await screen.findByText("Guardado")).toBeInTheDocument();
  });

  it("updates the counters with each saved grade", async () => {
    const user = userEvent.setup();
    await renderSheet();

    await user.type(input("Alan Turing"), "5{Enter}");

    const counters = screen.getByRole("group", { name: "Resumen del periodo" });
    await waitFor(() => expect(counters).toHaveTextContent("Capturadas2 de 3"));
    expect(counters).toHaveTextContent(/Promedio7Reprobados/);
    expect(counters).toHaveTextContent("Reprobados1");
  });
});

describe("PeriodGradeSheet — periods", () => {
  it("switching period refetches, and a late answer for the period already left is dropped", async () => {
    const user = userEvent.setup();
    const client = fakeClient();
    const first = deferred<PeriodSheet>();
    const backToSecond = deferred<PeriodSheet>();
    client.load.mockResolvedValueOnce(SHEET).mockReturnValueOnce(first.promise).mockReturnValueOnce(backToSecond.promise);
    await renderSheet(client);

    await user.click(chip(FIRST_CHIP));
    expect(client.load).toHaveBeenLastCalledWith({ assignmentId: "a1", periodId: "p1" });
    await user.click(chip(SECOND_CHIP));
    expect(client.load).toHaveBeenLastCalledWith({ assignmentId: "a1", periodId: "p2" });

    await act(async () => backToSecond.resolve({ ...SHEET, rows: [{ ...SHEET.rows[0]!, value: 7 }] }));
    await act(async () => first.resolve({ ...FIRST, rows: [{ ...SHEET.rows[0]!, value: 10 }] }));

    expect(input("Ada Lovelace")).toHaveValue("7");
    expect(chip(SECOND_CHIP)).toHaveAttribute("aria-pressed", "true");
  });
});

describe("PeriodGradeSheet — one save per cell at a time", () => {
  it("8 then 9: the 9 waits for the 8, and Guardado only once the 9 is confirmed", async () => {
    const user = userEvent.setup();
    const client = fakeClient();
    const first = deferred<SavedPeriodGrade>();
    const second = deferred<SavedPeriodGrade>();
    client.save.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    await renderSheet(client);
    const row = screen.getByRole("row", { name: /Alan Turing/ });

    await user.type(input("Alan Turing"), "8{Enter}");
    await user.clear(input("Alan Turing"));
    await user.type(input("Alan Turing"), "9{Enter}");
    expect(client.save).toHaveBeenCalledTimes(1);

    await act(async () => first.resolve({ enrollmentId: "en2", periodId: "p2", value: 8, updatedAt: "x" }));
    expect(client.save).toHaveBeenCalledTimes(2);
    expect(client.save).toHaveBeenLastCalledWith({ assignmentId: "a1", periodId: "p2", enrollmentId: "en2", value: 9 });
    expect(within(row).getByText("Guardando…")).toBeInTheDocument();
    expect(within(row).queryByText("Guardado")).toBeNull();

    await act(async () => second.resolve({ enrollmentId: "en2", periodId: "p2", value: 9, updatedAt: "x" }));
    expect(within(row).getByText("Guardado")).toBeInTheDocument();
    expect(screen.getByRole("group", { name: "Resumen del periodo" })).toHaveTextContent(/Promedio9Reprobados/);
  });

  it("8 then clear: the delete waits for the save and the cell ends empty", async () => {
    const user = userEvent.setup();
    const client = fakeClient();
    const first = deferred<SavedPeriodGrade>();
    client.save.mockReturnValueOnce(first.promise);
    await renderSheet(client);

    await user.type(input("Alan Turing"), "8{Enter}");
    await user.clear(input("Alan Turing"));
    await user.keyboard("{Enter}");
    expect(client.remove).not.toHaveBeenCalled();

    await act(async () => first.resolve({ enrollmentId: "en2", periodId: "p2", value: 8, updatedAt: "x" }));
    await waitFor(() => expect(client.remove).toHaveBeenCalledWith({ assignmentId: "a1", periodId: "p2", enrollmentId: "en2" }));
    const counters = screen.getByRole("group", { name: "Resumen del periodo" });
    await waitFor(() => expect(counters).toHaveTextContent("Capturadas1 de 3"));
    expect(within(screen.getByRole("row", { name: /Alan Turing/ })).getByText("Guardado")).toBeInTheDocument();
  });
});

describe("PeriodGradeSheet — nothing unsaved passes for saved", () => {
  async function failOne(): Promise<{ user: ReturnType<typeof userEvent.setup>; client: MockClient }> {
    const user = userEvent.setup();
    const client = fakeClient();
    client.save.mockRejectedValueOnce(new Error("network"));
    await renderSheet(client);
    await user.type(input("Alan Turing"), "8{Enter}");
    await screen.findByRole("button", { name: "Error — reintentar" });
    return { user, client };
  }

  it("a failed cell is counted as unsaved, with a retry for all", async () => {
    const { user, client } = await failOne();

    expect(screen.queryByText("Todo guardado")).toBeNull();
    await user.click(screen.getByRole("button", { name: "1 sin guardar — reintentar" }));

    await waitFor(() => expect(client.save).toHaveBeenCalledTimes(2));
    expect(await screen.findByText("Todo guardado")).toBeInTheDocument();
  });

  it("a failed cell still guards leaving the page", async () => {
    await failOne();
    const leaving = new Event("beforeunload", { cancelable: true });
    window.dispatchEvent(leaving);
    expect(leaving.defaultPrevented).toBe(true);
  });

  it("an invalid cell is counted too", async () => {
    const user = userEvent.setup();
    await renderSheet();
    await user.type(input("Alan Turing"), "11");
    await user.tab();
    expect(await screen.findByText("1 sin guardar")).toBeInTheDocument();
  });

  it("Tab from a failed cell reaches its retry button", async () => {
    const { user } = await failOne();
    await user.click(input("Alan Turing"));
    await user.tab();
    expect(screen.getByRole("button", { name: "Error — reintentar" })).toHaveFocus();
  });

  it("switching period with a failed cell asks first; declined, the period stays", async () => {
    const { user, client } = await failOne();
    const ask = vi.spyOn(window, "confirm").mockReturnValue(false);

    await user.click(chip(FIRST_CHIP));

    expect(ask).toHaveBeenCalledTimes(1);
    expect(client.load).toHaveBeenCalledTimes(1);
    expect(chip(SECOND_CHIP)).toHaveAttribute("aria-pressed", "true");
  });

  it("accepted, the unsaved value is kept and comes back with the period", async () => {
    const { user, client } = await failOne();
    vi.spyOn(window, "confirm").mockReturnValue(true);

    await user.click(chip(FIRST_CHIP));
    await waitFor(() => expect(client.load).toHaveBeenLastCalledWith({ assignmentId: "a1", periodId: "p1" }));
    await user.click(await screen.findByRole("button", { name: SECOND_CHIP }));

    await waitFor(() => expect(input("Alan Turing")).toHaveValue("8"));
    expect(screen.getByRole("button", { name: "Error — reintentar" })).toBeInTheDocument();
  });

  it("switching period with everything saved asks nothing", async () => {
    const user = userEvent.setup();
    const ask = vi.spyOn(window, "confirm");
    const client = await renderSheet();
    await user.click(chip(FIRST_CHIP));
    await waitFor(() => expect(client.load).toHaveBeenCalledTimes(2));
    expect(ask).not.toHaveBeenCalled();
  });

  it("coming back to a period waits for its saves in flight before loading it", async () => {
    const user = userEvent.setup();
    const client = fakeClient();
    const slow = deferred<SavedPeriodGrade>();
    client.save.mockReturnValueOnce(slow.promise);
    await renderSheet(client);

    await user.type(input("Alan Turing"), "8{Enter}");
    await user.click(chip(FIRST_CHIP));
    await waitFor(() => expect(client.load).toHaveBeenCalledTimes(2));
    await user.click(await screen.findByRole("button", { name: SECOND_CHIP }));
    expect(client.load).toHaveBeenCalledTimes(2);

    client.load.mockResolvedValueOnce({ ...SHEET, rows: [SHEET.rows[0]!, { ...SHEET.rows[1]!, value: 8 }, SHEET.rows[2]!] });
    await act(async () => slow.resolve({ enrollmentId: "en2", periodId: "p2", value: 8, updatedAt: "x" }));

    await waitFor(() => expect(client.load).toHaveBeenCalledTimes(3));
    expect(await screen.findByRole("textbox", { name: "Calificación de Alan Turing" })).toHaveValue("8");
  });
});

describe("PeriodGradeSheet — rows of the previous group", () => {
  const PREVIOUS = "Ese periodo corresponde al grupo anterior del alumno";
  const WITH_TRANSFER: PeriodSheet = {
    ...SHEET,
    rows: SHEET.rows.map((r) => (r.enrollmentId === "en2" ? { ...r, value: 7, rowEditable: false, rowReadOnlyReason: PREVIOUS } : r)),
  };

  it("shows the row read-only with its reason under the name", async () => {
    await renderSheet(fakeClient(WITH_TRANSFER));

    const row = screen.getByRole("row", { name: /Alan Turing/ });
    expect(within(row).getByText(PREVIOUS)).toBeInTheDocument();
    expect(input("Alan Turing")).toHaveAttribute("readonly");
    expect(input("Alan Turing")).toHaveAttribute("aria-readonly", "true");
    expect(input("Alan Turing")).toHaveValue("7");
  });

  it("Enter and Tab skip it, both ways", async () => {
    const user = userEvent.setup();
    await renderSheet(fakeClient(WITH_TRANSFER));

    await user.click(input("Ada Lovelace"));
    await user.keyboard("{Enter}");
    expect(input("Grace Hopper")).toHaveFocus();

    await user.keyboard("{Shift>}{Tab}{/Shift}");
    expect(input("Ada Lovelace")).toHaveFocus();

    await user.keyboard("{Tab}");
    expect(input("Grace Hopper")).toHaveFocus();
  });

  it("is not counted in the grades of the period it cannot change", async () => {
    await renderSheet(fakeClient(WITH_TRANSFER));
    expect(screen.queryByText(/sin guardar/)).toBeNull();
  });

  it("a save refused with PERIOD_PREVIOUS_GROUP turns the row read-only, not unsaved", async () => {
    const user = userEvent.setup();
    const client = fakeClient();
    client.save.mockRejectedValue(new GradeRequestError(400, PREVIOUS, "PERIOD_PREVIOUS_GROUP"));
    await renderSheet(client);

    await user.type(input("Alan Turing"), "9{Enter}");

    const row = screen.getByRole("row", { name: /Alan Turing/ });
    expect(await within(row).findByText(PREVIOUS)).toBeInTheDocument();
    expect(input("Alan Turing")).toHaveAttribute("aria-readonly", "true");
    expect(input("Alan Turing")).toHaveValue("");
    expect(screen.queryByText(/sin guardar/)).toBeNull();
  });
});

describe("PeriodGradeSheet — refusals by code", () => {
  it("a LEVEL_* refusal is a notice for the whole sheet", async () => {
    const user = userEvent.setup();
    const client = fakeClient();
    client.save.mockRejectedValue(new GradeRequestError(400, "El nivel aún no tiene escala de calificación", "LEVEL_SCALE_NOT_CONFIGURED"));
    await renderSheet(client);

    await user.type(input("Alan Turing"), "9{Enter}");

    const notice = await screen.findByRole("alert");
    expect(notice).toHaveTextContent("El nivel aún no tiene escala de calificación");
    expect(within(screen.getByRole("row", { name: /Alan Turing/ })).queryByText("El nivel aún no tiene escala de calificación")).toBeNull();
  });

  it("an unknown code still shows the server's text on the row", async () => {
    const user = userEvent.setup();
    const client = fakeClient();
    client.save.mockRejectedValue(new GradeRequestError(400, "Algo nuevo que el servidor explica", "BRAND_NEW_CODE"));
    await renderSheet(client);

    await user.type(input("Alan Turing"), "9{Enter}");

    const row = screen.getByRole("row", { name: /Alan Turing/ });
    expect(await within(row).findByText("Algo nuevo que el servidor explica")).toBeInTheDocument();
  });
});

describe("PeriodGradeSheet — closed period", () => {
  it("a PERIOD_LOCKED refusal is a notice for the whole sheet, and is not retried", async () => {
    const user = userEvent.setup();
    const client = fakeClient();
    client.save.mockRejectedValue(new GradeRequestError(409, "El periodo está cerrado", "PERIOD_LOCKED"));
    await renderSheet(client);

    await user.type(input("Alan Turing"), "9{Enter}");

    expect(await screen.findByRole("alert")).toHaveTextContent("El periodo está cerrado");
    expect(screen.queryByRole("button", { name: /Reintentar/ })).toBeNull();
    expect(client.save).toHaveBeenCalledTimes(1);
  });

  it("puts a lock on a closed period (still selectable) and shows the read-only reason", async () => {
    const closed: PeriodSheet = { ...SHEET, closedPeriodIds: ["p1", "p2"], editable: false, readOnlyReason: "El periodo está cerrado" };
    await renderSheet(fakeClient(closed));

    const second = chip(SECOND_CHIP);
    expect(second).toHaveAttribute("data-closed", "true");
    expect(second).toHaveTextContent("Cerrado");
    expect(chip(FIRST_CHIP)).not.toBeDisabled();
    expect(chip(THIRD_CHIP)).not.toHaveAttribute("data-closed");
    expect(screen.getByRole("note")).toHaveTextContent("El periodo está cerrado");
    expect(screen.queryByRole("textbox")).toBeNull();
  });
});
