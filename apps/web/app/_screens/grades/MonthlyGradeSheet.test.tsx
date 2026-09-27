import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { GradeRequestError, type GradeSheet, type GradeSheetClient, type SavedMonthlyGrade } from "../../grades/monthly";
import { MonthlyGradeSheet } from "./MonthlyGradeSheet";
import { useGradeSaves } from "./useGradeSaves";

const SHEET: GradeSheet = {
  assignmentId: "a1",
  month: "2026-09",
  months: ["2026-08", "2026-09", "2026-10"],
  scale: { min: 5, max: 10, decimals: 1, passing: 6 },
  editable: true,
  rows: [
    { enrollmentId: "en1", studentId: "st1", name: "Ada Lovelace", value: 9, updatedAt: "2026-09-10T10:00:00Z", rowEditable: true },
    { enrollmentId: "en2", studentId: "st2", name: "Alan Turing", value: null, updatedAt: null, rowEditable: true },
    { enrollmentId: "en3", studentId: "st3", name: "Grace Hopper", value: null, updatedAt: null, rowEditable: true },
  ],
};

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

function fakeClient(sheet: GradeSheet = SHEET): MockClient {
  return {
    load: vi.fn().mockResolvedValue(sheet),
    save: vi.fn().mockImplementation(
      async (i: { enrollmentId: string; month: string; value: number }): Promise<SavedMonthlyGrade> => ({ ...i, updatedAt: "2026-09-12T10:00:00Z" }),
    ),
    remove: vi.fn().mockResolvedValue(undefined),
  } as MockClient;
}

const input = (name: string): HTMLInputElement => screen.getByRole("textbox", { name: `Calificación de ${name}` });

/** The capture screens own the save store, above the sheet — as here. */
function Screen({ client }: { client: GradeSheetClient }) {
  const saves = useGradeSaves(client);
  return <MonthlyGradeSheet assignmentId="a1" client={client} saves={saves} />;
}

async function renderSheet(client: MockClient = fakeClient()): Promise<MockClient> {
  render(<Screen client={client} />);
  await screen.findByRole("table", { name: "Calificaciones del mes" });
  return client;
}

afterEach(() => {
  vi.clearAllMocks();
  vi.restoreAllMocks();
});

describe("MonthlyGradeSheet — loading", () => {
  it("asks the server for the current month and shows one field per student", async () => {
    const client = await renderSheet();

    expect(client.load).toHaveBeenCalledWith({ assignmentId: "a1" });
    expect(input("Ada Lovelace")).toHaveValue("9");
    expect(input("Alan Turing")).toHaveValue("");
    expect(input("Ada Lovelace")).toHaveAttribute("inputmode", "decimal");
    expect(screen.getByRole("button", { name: "Sep" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "Ago" })).toHaveAttribute("aria-pressed", "false");
  });

  it("counts captured, averages them and counts the failing", async () => {
    await renderSheet(fakeClient({ ...SHEET, rows: [...SHEET.rows.slice(0, 2), { ...SHEET.rows[2]!, value: 5.5 }] }));

    const counters = screen.getByRole("group", { name: "Resumen del mes" });
    expect(counters).toHaveTextContent("Capturadas2 de 3");
    expect(counters).toHaveTextContent("Promedio7.3");
    expect(counters).toHaveTextContent("Reprobados1");
  });

  it("says when a level has no numeric grade", async () => {
    const client = fakeClient();
    client.load.mockRejectedValue(new GradeRequestError(400, "Este nivel no lleva calificación numérica"));
    render(<Screen client={client} />);

    expect(await screen.findByText("Este nivel no lleva calificación numérica")).toBeInTheDocument();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("offers a retry when the sheet fails to load", async () => {
    const user = userEvent.setup();
    const client = fakeClient();
    client.load.mockRejectedValueOnce(new Error("boom"));
    render(<Screen client={client} />);

    await user.click(await screen.findByRole("button", { name: "Reintentar" }));
    expect(await screen.findByRole("table", { name: "Calificaciones del mes" })).toBeInTheDocument();
  });

  it("is read-only when the month is closed, and says why", async () => {
    await renderSheet(fakeClient({ ...SHEET, editable: false, readOnlyReason: "El mes ya se cerró." }));

    expect(screen.getByText("El mes ya se cerró.")).toBeInTheDocument();
    expect(screen.queryByRole("textbox")).toBeNull();
    expect(within(screen.getByRole("table")).getByText("9")).toBeInTheDocument();
  });
});

describe("MonthlyGradeSheet — keyboard capture", () => {
  it("Enter saves the cell (a decimal comma reads as a point) and moves to the next student", async () => {
    const user = userEvent.setup();
    const client = await renderSheet();

    await user.click(input("Alan Turing"));
    await user.keyboard("9,5{Enter}");

    expect(client.save).toHaveBeenCalledWith({ assignmentId: "a1", month: "2026-09", enrollmentId: "en2", value: 9.5 });
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
    expect(client.save).toHaveBeenCalledWith({ assignmentId: "a1", month: "2026-09", enrollmentId: "en2", value: 8 });

    await user.keyboard("{Shift>}{Tab}{/Shift}");
    expect(input("Alan Turing")).toHaveFocus();
  });

  it("an emptied cell is deleted", async () => {
    const user = userEvent.setup();
    const client = await renderSheet();

    await user.clear(input("Ada Lovelace"));
    await user.keyboard("{Tab}");

    expect(client.remove).toHaveBeenCalledWith({ assignmentId: "a1", month: "2026-09", enrollmentId: "en1" });
    expect(client.save).not.toHaveBeenCalled();
  });
});

describe("MonthlyGradeSheet — validation", () => {
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

  it("checks the decimals the level allows", async () => {
    const user = userEvent.setup();
    const client = await renderSheet();

    await user.type(input("Alan Turing"), "9.55");
    await user.tab();

    expect(await screen.findByText("Solo se permite 1 decimal")).toBeInTheDocument();
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
    client.save.mockRejectedValue(new GradeRequestError(400, "Solo se permite 1 decimal"));
    await renderSheet(client);

    await user.type(input("Alan Turing"), "9{Enter}");

    const row = screen.getByRole("row", { name: /Alan Turing/ });
    expect(await within(row).findByText("Solo se permite 1 decimal")).toBeInTheDocument();
    expect(within(row).queryByRole("button", { name: "Error — reintentar" })).toBeNull();
  });
});

describe("MonthlyGradeSheet — saving status", () => {
  it("shows Guardando… and guards leaving the page while a save is in flight", async () => {
    const user = userEvent.setup();
    const client = fakeClient();
    const pending = deferred<SavedMonthlyGrade>();
    client.save.mockReturnValue(pending.promise);
    await renderSheet(client);

    await user.type(input("Alan Turing"), "8{Enter}");

    const row = screen.getByRole("row", { name: /Alan Turing/ });
    expect(within(row).getByText("Guardando…")).toBeInTheDocument();
    expect(screen.getByText("Guardando 1 calificación…")).toBeInTheDocument();
    const leaving = new Event("beforeunload", { cancelable: true });
    window.dispatchEvent(leaving);
    expect(leaving.defaultPrevented).toBe(true);

    await act(async () => pending.resolve({ enrollmentId: "en2", month: "2026-09", value: 8, updatedAt: "x" }));

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
    expect(client.save).toHaveBeenLastCalledWith({ assignmentId: "a1", month: "2026-09", enrollmentId: "en2", value: 8 });
    expect(await screen.findByText("Guardado")).toBeInTheDocument();
  });

  it("updates the counters with each saved grade", async () => {
    const user = userEvent.setup();
    await renderSheet();

    await user.type(input("Alan Turing"), "5{Enter}");

    const counters = screen.getByRole("group", { name: "Resumen del mes" });
    await waitFor(() => expect(counters).toHaveTextContent("Capturadas2 de 3"));
    expect(counters).toHaveTextContent("Promedio7.0");
    expect(counters).toHaveTextContent("Reprobados1");
  });
});

describe("MonthlyGradeSheet — months", () => {
  it("switching month refetches, and a late answer for the old month is dropped", async () => {
    const user = userEvent.setup();
    const client = fakeClient();
    const august = deferred<GradeSheet>();
    const october = deferred<GradeSheet>();
    client.load.mockResolvedValueOnce(SHEET).mockReturnValueOnce(august.promise).mockReturnValueOnce(october.promise);
    await renderSheet(client);

    await user.click(screen.getByRole("button", { name: "Ago" }));
    await user.click(screen.getByRole("button", { name: "Oct" }));
    expect(client.load).toHaveBeenLastCalledWith({ assignmentId: "a1", month: "2026-10" });

    const octRows = [{ ...SHEET.rows[0]!, value: 7 }];
    await act(async () => october.resolve({ ...SHEET, month: "2026-10", rows: octRows }));
    await act(async () => august.resolve({ ...SHEET, month: "2026-08", rows: [{ ...SHEET.rows[0]!, value: 10 }] }));

    expect(input("Ada Lovelace")).toHaveValue("7");
    expect(screen.getByRole("button", { name: "Oct" })).toHaveAttribute("aria-pressed", "true");
  });
});

describe("MonthlyGradeSheet — one save per cell at a time", () => {
  it("8 then 9: the 9 waits for the 8, and Guardado only once the 9 is confirmed", async () => {
    const user = userEvent.setup();
    const client = fakeClient();
    const first = deferred<SavedMonthlyGrade>();
    const second = deferred<SavedMonthlyGrade>();
    client.save.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    await renderSheet(client);
    const row = screen.getByRole("row", { name: /Alan Turing/ });

    await user.type(input("Alan Turing"), "8{Enter}");
    await user.clear(input("Alan Turing"));
    await user.type(input("Alan Turing"), "9{Enter}");
    expect(client.save).toHaveBeenCalledTimes(1);

    await act(async () => first.resolve({ enrollmentId: "en2", month: "2026-09", value: 8, updatedAt: "x" }));
    expect(client.save).toHaveBeenCalledTimes(2);
    expect(client.save).toHaveBeenLastCalledWith({ assignmentId: "a1", month: "2026-09", enrollmentId: "en2", value: 9 });
    expect(within(row).getByText("Guardando…")).toBeInTheDocument();
    expect(within(row).queryByText("Guardado")).toBeNull();

    await act(async () => second.resolve({ enrollmentId: "en2", month: "2026-09", value: 9, updatedAt: "x" }));
    expect(within(row).getByText("Guardado")).toBeInTheDocument();
    expect(screen.getByRole("group", { name: "Resumen del mes" })).toHaveTextContent("Promedio9.0");
  });

  it("8 then clear: the delete waits for the save and the cell ends empty", async () => {
    const user = userEvent.setup();
    const client = fakeClient();
    const first = deferred<SavedMonthlyGrade>();
    client.save.mockReturnValueOnce(first.promise);
    await renderSheet(client);

    await user.type(input("Alan Turing"), "8{Enter}");
    await user.clear(input("Alan Turing"));
    await user.keyboard("{Enter}");
    expect(client.remove).not.toHaveBeenCalled();

    await act(async () => first.resolve({ enrollmentId: "en2", month: "2026-09", value: 8, updatedAt: "x" }));
    await waitFor(() => expect(client.remove).toHaveBeenCalledWith({ assignmentId: "a1", month: "2026-09", enrollmentId: "en2" }));
    const counters = screen.getByRole("group", { name: "Resumen del mes" });
    await waitFor(() => expect(counters).toHaveTextContent("Capturadas1 de 3"));
    expect(within(screen.getByRole("row", { name: /Alan Turing/ })).getByText("Guardado")).toBeInTheDocument();
  });
});

describe("MonthlyGradeSheet — nothing unsaved passes for saved", () => {
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

  it("switching month with a failed cell asks first; declined, the month stays", async () => {
    const { user, client } = await failOne();
    const ask = vi.spyOn(window, "confirm").mockReturnValue(false);

    await user.click(screen.getByRole("button", { name: "Oct" }));

    expect(ask).toHaveBeenCalledTimes(1);
    expect(client.load).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("button", { name: "Sep" })).toHaveAttribute("aria-pressed", "true");
  });

  it("accepted, the unsaved value is kept and comes back with the month", async () => {
    const { user, client } = await failOne();
    vi.spyOn(window, "confirm").mockReturnValue(true);

    await user.click(screen.getByRole("button", { name: "Oct" }));
    await waitFor(() => expect(client.load).toHaveBeenLastCalledWith({ assignmentId: "a1", month: "2026-10" }));
    await user.click(await screen.findByRole("button", { name: "Sep" }));

    await waitFor(() => expect(input("Alan Turing")).toHaveValue("8"));
    expect(screen.getByRole("button", { name: "Error — reintentar" })).toBeInTheDocument();
  });

  it("switching month with everything saved asks nothing", async () => {
    const user = userEvent.setup();
    const ask = vi.spyOn(window, "confirm");
    const client = await renderSheet();
    await user.click(screen.getByRole("button", { name: "Oct" }));
    await waitFor(() => expect(client.load).toHaveBeenCalledTimes(2));
    expect(ask).not.toHaveBeenCalled();
  });

  it("coming back to a month waits for its saves in flight before loading it", async () => {
    const user = userEvent.setup();
    const client = fakeClient();
    const slow = deferred<SavedMonthlyGrade>();
    client.save.mockReturnValueOnce(slow.promise);
    await renderSheet(client);

    await user.type(input("Alan Turing"), "8{Enter}");
    await user.click(screen.getByRole("button", { name: "Oct" }));
    await waitFor(() => expect(client.load).toHaveBeenCalledTimes(2));
    await user.click(await screen.findByRole("button", { name: "Sep" }));
    expect(client.load).toHaveBeenCalledTimes(2);

    client.load.mockResolvedValueOnce({ ...SHEET, rows: [SHEET.rows[0]!, { ...SHEET.rows[1]!, value: 8 }, SHEET.rows[2]!] });
    await act(async () => slow.resolve({ enrollmentId: "en2", month: "2026-09", value: 8, updatedAt: "x" }));

    await waitFor(() => expect(client.load).toHaveBeenCalledTimes(3));
    expect(await screen.findByRole("textbox", { name: "Calificación de Alan Turing" })).toHaveValue("8");
  });
});

describe("MonthlyGradeSheet — rows of the previous group", () => {
  const PREVIOUS = "Ese mes corresponde al grupo anterior del alumno";
  const WITH_TRANSFER: GradeSheet = {
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

  it("is not counted in the grades of the month it cannot change", async () => {
    await renderSheet(fakeClient(WITH_TRANSFER));
    expect(screen.queryByText(/sin guardar/)).toBeNull();
  });

  it("a save refused with MONTH_PREVIOUS_GROUP turns the row read-only, not unsaved", async () => {
    const user = userEvent.setup();
    const client = fakeClient();
    client.save.mockRejectedValue(new GradeRequestError(400, PREVIOUS, "MONTH_PREVIOUS_GROUP"));
    await renderSheet(client);

    await user.type(input("Alan Turing"), "9{Enter}");

    const row = screen.getByRole("row", { name: /Alan Turing/ });
    expect(await within(row).findByText(PREVIOUS)).toBeInTheDocument();
    expect(input("Alan Turing")).toHaveAttribute("aria-readonly", "true");
    expect(input("Alan Turing")).toHaveValue("");
    expect(screen.queryByText(/sin guardar/)).toBeNull();
  });
});

describe("MonthlyGradeSheet — refusals by code", () => {
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
