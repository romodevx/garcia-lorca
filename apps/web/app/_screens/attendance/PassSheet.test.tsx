import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AttendanceRequestError, type AttendanceSheet, type SaveSheetInput } from "../../attendance/sheet";
import { PassSheet } from "./PassSheet";

const TODAY = "2026-09-10";

const sheetOf = (over: Partial<AttendanceSheet> = {}): AttendanceSheet => ({
  mode: "daily",
  date: TODAY,
  editable: true,
  windowDays: 7,
  session: null,
  rows: [
    { enrollmentId: "en1", studentId: "st1", name: "Ada Lovelace", status: "present", note: "" },
    { enrollmentId: "en2", studentId: "st2", name: "Alan Turing", status: "present", note: "" },
    { enrollmentId: "en3", studentId: "st3", name: "Grace Hopper", status: "excused", note: "Cita médica" },
  ],
  ...over,
});

const load = vi.fn<(q: { groupId: string; date: string; subjectId?: string }) => Promise<AttendanceSheet>>();
const save = vi.fn<(input: SaveSheetInput) => Promise<void>>();

function renderSheet(props: Partial<Parameters<typeof PassSheet>[0]> = {}) {
  return render(
    <PassSheet title="1° SECUNDARIA · A" groupId="g1" today={TODAY} restrictDates load={load} save={save} {...props} />,
  );
}

const counter = (label: string): HTMLElement => within(screen.getByRole("list", { name: "Resumen de la lista" })).getByText(label).closest("li") as HTMLElement;

beforeEach(() => {
  load.mockResolvedValue(sheetOf());
  save.mockResolvedValue(undefined);
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

describe("PassSheet — opening", () => {
  it("opens on today, loads the list (with the subject for a per-class list) and shows every row present by default", async () => {
    renderSheet({ subjectId: "s-mat" });

    await screen.findByRole("button", { name: "Ada Lovelace: Presente" });
    expect(load).toHaveBeenCalledWith({ groupId: "g1", date: TODAY, subjectId: "s-mat" });
    expect(screen.getByLabelText("Fecha")).toHaveValue(TODAY);
    expect(screen.getByRole("button", { name: "Grace Hopper: Justificada" })).toBeInTheDocument();
    expect(screen.getByText("Cita médica")).toBeInTheDocument();
  });

  it("limits a teacher's date to today…today−7, and leaves the director's open", async () => {
    const { unmount } = renderSheet();
    await screen.findByRole("button", { name: "Ada Lovelace: Presente" });
    expect(screen.getByLabelText("Fecha")).toHaveAttribute("min", "2026-09-03");
    expect(screen.getByLabelText("Fecha")).toHaveAttribute("max", TODAY);
    unmount();

    renderSheet({ restrictDates: false });
    await screen.findByRole("button", { name: "Ada Lovelace: Presente" });
    expect(screen.getByLabelText("Fecha")).not.toHaveAttribute("min");
  });

  it("says the list has not been taken, or who saved it and when", async () => {
    const { unmount } = renderSheet();
    expect(await screen.findByText("Lista aún no tomada")).toBeInTheDocument();
    unmount();

    load.mockResolvedValue(sheetOf({ session: { takenAt: "2026-09-10T08:05:00", takenByName: "Mtra. Rojas" } }));
    renderSheet();
    expect(await screen.findByText("Lista guardada a las 8:05 por Mtra. Rojas")).toBeInTheDocument();
  });

  it("reloads for another day", async () => {
    renderSheet();
    await screen.findByRole("button", { name: "Ada Lovelace: Presente" });

    fireEvent.change(screen.getByLabelText("Fecha"), { target: { value: "2026-09-08" } });

    await waitFor(() => expect(load).toHaveBeenLastCalledWith({ groupId: "g1", date: "2026-09-08" }));
  });

  it("shows the server's reason when the list cannot load", async () => {
    load.mockRejectedValue(new AttendanceRequestError(400, "Esta lista es por clase: elige la asignatura"));
    renderSheet();
    expect(await screen.findByRole("alert")).toHaveTextContent("Esta lista es por clase: elige la asignatura");
  });
});

describe("PassSheet — marking", () => {
  it("one tap flips a student to Ausente and back, and the counters follow", async () => {
    const user = userEvent.setup();
    renderSheet();

    await user.click(await screen.findByRole("button", { name: "Ada Lovelace: Presente" }));
    expect(screen.getByRole("button", { name: "Ada Lovelace: Ausente" })).toBeInTheDocument();
    expect(counter("Presentes")).toHaveTextContent("1");
    expect(counter("Ausentes")).toHaveTextContent("1");
    expect(counter("Justificadas")).toHaveTextContent("1");

    await user.click(screen.getByRole("button", { name: "Ada Lovelace: Ausente" }));
    expect(counter("Presentes")).toHaveTextContent("2");
  });

  it("Más opens Retardo, Justificada and a note of at most 500 characters with a counter", async () => {
    const user = userEvent.setup();
    renderSheet();

    await user.click(await screen.findByRole("button", { name: "Más opciones de Alan Turing" }));
    await user.click(screen.getByRole("button", { name: "Retardo" }));
    expect(screen.getByRole("button", { name: "Alan Turing: Retardo" })).toBeInTheDocument();
    expect(counter("Retardos")).toHaveTextContent("1");

    const note = screen.getByRole("textbox", { name: "Nota de Alan Turing" });
    expect(note).toHaveAttribute("maxLength", "500");
    await user.type(note, "Llegó 8:20");
    expect(screen.getByText("10/500")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Justificada" }));
    expect(screen.getByRole("button", { name: "Alan Turing: Justificada" })).toBeInTheDocument();
  });

  it("Todos presentes clears every exception", async () => {
    const user = userEvent.setup();
    renderSheet();

    await user.click(await screen.findByRole("button", { name: "Ada Lovelace: Presente" }));
    await user.click(screen.getByRole("button", { name: "Todos presentes" }));

    expect(counter("Presentes")).toHaveTextContent("3");
    expect(counter("Ausentes")).toHaveTextContent("0");
  });
});

describe("PassSheet — saving", () => {
  it("sends only the exceptions, notes included, then toasts and refreshes the session line", async () => {
    const user = userEvent.setup();
    renderSheet({ subjectId: "s-mat" });

    await user.click(await screen.findByRole("button", { name: "Ada Lovelace: Presente" }));
    load.mockResolvedValue(sheetOf({ session: { takenAt: "2026-09-10T08:05:00", takenByName: "Mtra. Rojas" } }));
    await user.click(screen.getByRole("button", { name: "Guardar lista" }));

    await waitFor(() =>
      expect(save).toHaveBeenCalledWith({
        groupId: "g1",
        date: TODAY,
        subjectId: "s-mat",
        marks: [
          { enrollmentId: "en1", status: "absent" },
          { enrollmentId: "en3", status: "excused", note: "Cita médica" },
        ],
      }),
    );
    expect(await screen.findByRole("status")).toHaveTextContent("Lista guardada");
    expect(await screen.findByText("Lista guardada a las 8:05 por Mtra. Rojas")).toBeInTheDocument();
  });

  it("the save button is never disabled, and says so while saving", async () => {
    const user = userEvent.setup();
    let finish: () => void = () => {};
    save.mockReturnValue(new Promise<void>((resolve) => { finish = resolve; }));
    renderSheet();

    const button = await screen.findByRole("button", { name: "Guardar lista" });
    expect(button).toBeEnabled();
    await user.click(button);
    expect(screen.getByRole("button", { name: "Guardando…" })).toBeEnabled();
    await act(async () => finish());
  });

  it("shows the server's reason when the save is rejected", async () => {
    const user = userEvent.setup();
    save.mockRejectedValue(new AttendanceRequestError(400, "Solo puedes corregir hasta 7 días atrás"));
    renderSheet();

    await user.click(await screen.findByRole("button", { name: "Guardar lista" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Solo puedes corregir hasta 7 días atrás");
  });

  it("a list that is not editable is read-only and says why", async () => {
    load.mockResolvedValue(sheetOf({ editable: false }));
    renderSheet();

    expect(await screen.findByText(/solo puedes corregir hasta 7 días atrás/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Guardar lista" })).toBeNull();
    expect(screen.queryByRole("button", { name: /Ada Lovelace/ })).toBeNull();
    expect(screen.getByText("Ada Lovelace")).toBeInTheDocument();
  });
});

describe("PassSheet — unsaved changes", () => {
  it("reports dirtiness, and asks before switching day with unsaved marks", async () => {
    const user = userEvent.setup();
    const onDirtyChange = vi.fn();
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    renderSheet({ onDirtyChange });

    await user.click(await screen.findByRole("button", { name: "Ada Lovelace: Presente" }));
    expect(onDirtyChange).toHaveBeenLastCalledWith(true);

    fireEvent.change(screen.getByLabelText("Fecha"), { target: { value: "2026-09-08" } });
    expect(confirm).toHaveBeenCalled();
    expect(screen.getByLabelText("Fecha")).toHaveValue(TODAY);
  });
});
