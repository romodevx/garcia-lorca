import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import { StudentNotesPanel, type StudentNotesPanelProps } from "./StudentNotesPanel";
import { useStudentNotes } from "./useStudentNotes";

vi.mock("../../students/api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../../students/api")>();
  return { StudentsError: real.StudentsError, listNotes: vi.fn(), addNote: vi.fn(), voidNote: vi.fn(), editNote: vi.fn() };
});
const api = vi.mocked(studentsApi);

const WRITTEN = Date.parse("2026-09-24T16:00:00Z");
const mine = { id: "n-2", studentId: "st1", body: "Llegó con fiebre", authorUserId: "u-me", authorName: "Gabriela Rojas", createdAt: "2026-09-24T16:00:00Z", voided: false };
const theirs = { id: "n-1", studentId: "st1", body: "Olvidó la mochila", authorUserId: "u-other", authorName: "Luis Pérez", createdAt: "2026-09-23T15:00:00Z", voided: false };

type PanelProps = Omit<StudentNotesPanelProps, "notes">;

/** The panel as every screen mounts it: fed by the one hook. */
function Harness(props: PanelProps) {
  const notes = useStudentNotes(props.studentId);
  return <StudentNotesPanel notes={notes} {...props} />;
}

function renderPanel(props: Partial<PanelProps> = {}) {
  return render(
    <Harness studentId="st1" userId="u-me" canWrite canVoidAny={false} now={() => WRITTEN + 5 * 60_000} {...props} />,
  );
}

beforeEach(() => {
  api.listNotes.mockResolvedValue([mine, theirs]);
  api.voidNote.mockResolvedValue({ ...mine, voided: true });
});
afterEach(() => vi.clearAllMocks());

// Fake timers WITHOUT shouldAdvanceTime: wall-clock time must never reach the
// fake clock, or a stalled runner fires timers the test did not advance.
// Testing Library only drives fake timers through a global `jest`; this shim
// hands it vitest's, so findBy/waitFor still resolve.
function useFakeClock(): void {
  vi.useFakeTimers();
  Object.assign(globalThis, { jest: { advanceTimersByTime: (ms: number) => vi.advanceTimersByTime(ms) } });
}
function useRealClock(): void {
  Reflect.deleteProperty(globalThis, "jest");
  vi.useRealTimers();
}

describe("StudentNotesPanel", () => {
  it("shows the history newest first, with who wrote each note", async () => {
    renderPanel();
    const items = await screen.findAllByRole("listitem");
    expect(items[0]).toHaveTextContent("Llegó con fiebre");
    expect(items[0]).toHaveTextContent("Gabriela Rojas");
    expect(items[1]).toHaveTextContent("Olvidó la mochila");
  });

  it("marks a voided note as such", async () => {
    api.listNotes.mockResolvedValue([{ ...theirs, voided: true, voidedAt: "2026-09-24T17:00:00Z" }]);
    renderPanel({ canVoidAny: true });
    expect(await screen.findByText("Anulada")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Anular/ })).not.toBeInTheDocument();
  });

  it("writes a note and puts it on top", async () => {
    const user = userEvent.setup();
    api.addNote.mockResolvedValue({ ...mine, id: "n-3", body: "Salió temprano" });
    renderPanel();
    await screen.findByText("Llegó con fiebre");
    await user.type(screen.getByLabelText("Nueva nota"), "Salió temprano");
    await user.click(screen.getByRole("button", { name: "Agregar nota" }));

    await waitFor(() => expect(api.addNote).toHaveBeenCalledWith(expect.any(String), "st1", "Salió temprano"));
    expect((await screen.findAllByRole("listitem"))[0]).toHaveTextContent("Salió temprano");
    expect(screen.getByLabelText("Nueva nota")).toHaveValue("");
  });

  it("refuses an empty note on the field", async () => {
    const user = userEvent.setup();
    renderPanel();
    await screen.findByText("Llegó con fiebre");
    await user.click(screen.getByRole("button", { name: "Agregar nota" }));
    expect(api.addNote).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Nueva nota")).toHaveAttribute("aria-invalid", "true");
  });

  it("offers no writing to whoever may only read", async () => {
    renderPanel({ canWrite: false });
    await screen.findByText("Llegó con fiebre");
    expect(screen.queryByLabelText("Nueva nota")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Anular/ })).not.toBeInTheDocument();
  });

  it("lets the author void their own note within 15 minutes, and nobody else's", async () => {
    renderPanel();
    const items = await screen.findAllByRole("listitem");
    expect(within(items[0] as HTMLElement).getByRole("button", { name: "Anular nota" })).toBeInTheDocument();
    expect(within(items[1] as HTMLElement).queryByRole("button", { name: "Anular nota" })).not.toBeInTheDocument();
  });

  it("no longer offers the author a void after 15 minutes", async () => {
    renderPanel({ now: () => WRITTEN + 16 * 60_000 });
    await screen.findByText("Llegó con fiebre");
    expect(screen.queryByRole("button", { name: "Anular nota" })).not.toBeInTheDocument();
  });

  it("lets whoever may void any note void every one", async () => {
    renderPanel({ canVoidAny: true, now: () => WRITTEN + 24 * 3_600_000 });
    await screen.findByText("Llegó con fiebre");
    expect(screen.getAllByRole("button", { name: "Anular nota" })).toHaveLength(2);
  });

  // Reversible: gone at once, told to the server when the offer is closed,
  // another note is voided, the page is left — or the window is about to end.
  it("voids with an undo", async () => {
    const user = userEvent.setup();
    const { unmount } = renderPanel();
    await screen.findByText("Llegó con fiebre");
    await user.click(screen.getByRole("button", { name: "Anular nota" }));
    expect(screen.queryByText("Llegó con fiebre")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /Deshacer/ }));
    expect(screen.getByText("Llegó con fiebre")).toBeInTheDocument();
    expect(api.voidNote).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: "Anular nota" }));
    unmount();
    expect(api.voidNote).toHaveBeenCalledWith(expect.any(String), "st1", "n-2", { keepalive: false });
  });

  // Gate B1 on #142: closing the tab must not cancel the void.
  it("closing the tab sends the void with keepalive", async () => {
    const user = userEvent.setup();
    renderPanel();
    await screen.findByText("Llegó con fiebre");
    await user.click(screen.getByRole("button", { name: "Anular nota" }));

    window.dispatchEvent(new Event("pagehide"));

    expect(api.voidNote).toHaveBeenCalledWith(expect.any(String), "st1", "n-2", { keepalive: true });
  });

  // Gate B2 on #142: Deshacer has no clock any more, but the author's 15
  // minutes do. Left open, the offer would outlive the window and the server
  // would refuse the void. It is sent while the window is still open.
  it("sends an author's void before the 15 minutes run out, even with Deshacer still open", async () => {
    useFakeClock();
    try {
      renderPanel();
      await screen.findByText("Llegó con fiebre");
      fireEvent.click(screen.getByRole("button", { name: "Anular nota" }));

      act(() => vi.advanceTimersByTime(9 * 60_000));
      expect(api.voidNote).not.toHaveBeenCalled();

      act(() => vi.advanceTimersByTime(60_000));
      expect(api.voidNote).toHaveBeenCalledWith(expect.any(String), "st1", "n-2", { keepalive: false });
      expect(screen.queryByRole("button", { name: /Deshacer/ })).not.toBeInTheDocument();
      // The offer left without anybody closing it: say what happened.
      expect(screen.getByRole("status")).toHaveTextContent("Se anuló la nota.");
    } finally {
      useRealClock();
    }
  });

  // The void reaches the server only after the 8 s undo: with less than that
  // left, the server would refuse it. The button goes before the window does.
  it("stops offering the author a void when the undo would outlast the window", async () => {
    renderPanel({ now: () => WRITTEN + 15 * 60_000 - 5_000 });
    await screen.findByText("Llegó con fiebre");
    expect(screen.queryByRole("button", { name: "Anular nota" })).not.toBeInTheDocument();
  });

  // A page left open does not keep offering a void the server will refuse.
  it("takes the button away when the window closes on an open page", async () => {
    useFakeClock();
    try {
      let clock = WRITTEN + 14 * 60_000;
      renderPanel({ now: () => clock });
      await screen.findByText("Llegó con fiebre");
      expect(screen.getByRole("button", { name: "Anular nota" })).toBeInTheDocument();
      clock = WRITTEN + 16 * 60_000;
      await act(async () => {
        vi.advanceTimersByTime(60_000);
      });
      expect(screen.queryByRole("button", { name: "Anular nota" })).not.toBeInTheDocument();
    } finally {
      useRealClock();
    }
  });

  it("refuses a note longer than 2000 characters on the field", async () => {
    const user = userEvent.setup();
    renderPanel();
    await screen.findByText("Llegó con fiebre");
    fireEvent.change(screen.getByLabelText("Nueva nota"), { target: { value: "a".repeat(2001) } });
    await user.click(screen.getByRole("button", { name: "Agregar nota" }));
    expect(api.addNote).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Nueva nota")).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByText(/2000 caracteres/)).toBeInTheDocument();
  });

  it("says why the server refused a note, instead of 'try again'", async () => {
    const user = userEvent.setup();
    api.addNote.mockRejectedValue(new studentsApi.StudentsError(400, "a note has 1 to 2000 characters"));
    renderPanel();
    await screen.findByText("Llegó con fiebre");
    await user.type(screen.getByLabelText("Nueva nota"), "x");
    await user.click(screen.getByRole("button", { name: "Agregar nota" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(/de 1 a 2000 caracteres/);
  });

  // The button leaves at the exact cutoff, not at the next periodic re-check:
  // a click just after it would be voided after the server's window.
  it("takes the button away at the cutoff itself", async () => {
    useFakeClock();
    try {
      const cutoff = WRITTEN + 15 * 60_000 - 10_000;
      let clock = cutoff - 100;
      renderPanel({ now: () => clock });
      await screen.findByText("Llegó con fiebre");
      expect(screen.getByRole("button", { name: "Anular nota" })).toBeInTheDocument();
      clock = cutoff + 1;
      await act(async () => {
        vi.advanceTimersByTime(200);
      });
      expect(screen.queryByRole("button", { name: "Anular nota" })).not.toBeInTheDocument();
    } finally {
      useRealClock();
    }
  });

  // A timer can fire while the clock still reads before the cutoff (it fired
  // early, or the wall clock stepped back). It must re-arm, or nothing ever
  // takes the button away.
  it("re-arms the cutoff when its timer fires before the clock reaches it", async () => {
    useFakeClock();
    try {
      const cutoff = WRITTEN + 15 * 60_000 - 10_000;
      let clock = cutoff - 100;
      renderPanel({ now: () => clock });
      await screen.findByText("Llegó con fiebre");
      await act(async () => {
        vi.advanceTimersByTime(500); // the timer fires; the clock has not moved
      });
      expect(screen.getByRole("button", { name: "Anular nota" })).toBeInTheDocument();
      clock = cutoff + 1;
      await act(async () => {
        vi.advanceTimersByTime(200);
      });
      expect(screen.queryByRole("button", { name: "Anular nota" })).not.toBeInTheDocument();
    } finally {
      useRealClock();
    }
  });

  // Timers stop while a laptop sleeps: the button can outlive the window on a
  // page that just woke up. The click itself re-checks, so it never starts a
  // void the server would refuse.
  // The click is synchronous on purpose: userEvent spreads a click over several
  // ticks, and any re-render in between reads the moved clock and takes the
  // button away first (seen on a loaded CI runner).
  it("refuses a click that comes after the cutoff, whatever the screen still shows", async () => {
    let clock = WRITTEN + 60_000;
    renderPanel({ now: () => clock });
    await screen.findByText("Llegó con fiebre");
    const button = screen.getByRole("button", { name: "Anular nota" });
    clock = WRITTEN + 20 * 60_000; // no timer ran: the machine was asleep
    fireEvent.click(button);

    expect(screen.getByText("Llegó con fiebre")).toBeInTheDocument();
    expect(screen.queryByText("Se anuló la nota")).not.toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent(/15 minutos/);
    expect(api.voidNote).not.toHaveBeenCalled();
  });

  // T6: a note can be edited; the server keeps the text it replaced.
  it("shows who last edited a note, and when", async () => {
    api.listNotes.mockResolvedValue([{ ...theirs, updatedAt: "2026-09-25T15:00:00Z", updatedByName: "Dirección" }]);
    renderPanel();
    const item = await screen.findByRole("listitem");
    expect(item).toHaveTextContent("Luis Pérez");
    expect(item).toHaveTextContent(/Editada por Dirección/);
  });

  it("lets the author edit their own note in place, and nobody else's", async () => {
    const user = userEvent.setup();
    api.editNote.mockResolvedValue({ ...mine, body: "Llegó con fiebre de 38", updatedAt: "2026-09-24T16:30:00Z", updatedByName: "Gabriela Rojas" });
    renderPanel({ now: () => WRITTEN + 3 * 24 * 3_600_000 });
    const items = await screen.findAllByRole("listitem");
    expect(within(items[1] as HTMLElement).queryByRole("button", { name: "Editar nota" })).not.toBeInTheDocument();

    await user.click(within(items[0] as HTMLElement).getByRole("button", { name: "Editar nota" }));
    const field = screen.getByLabelText("Texto de la nota");
    expect(field).toHaveValue("Llegó con fiebre");
    expect(field).toHaveFocus();
    await user.clear(field);
    await user.type(field, "Llegó con fiebre de 38");
    await user.click(screen.getByRole("button", { name: "Guardar nota" }));

    await waitFor(() => expect(api.editNote).toHaveBeenCalledWith(expect.any(String), "st1", "n-2", "Llegó con fiebre de 38", "Llegó con fiebre"));
    const first = (await screen.findAllByRole("listitem"))[0] as HTMLElement;
    expect(first).toHaveTextContent("Llegó con fiebre de 38");
    expect(first).toHaveTextContent(/Editada por Gabriela Rojas/);
    expect(screen.queryByLabelText("Texto de la nota")).not.toBeInTheDocument();
  });

  it("lets whoever may void any note edit every one", async () => {
    renderPanel({ canVoidAny: true });
    await screen.findByText("Llegó con fiebre");
    expect(screen.getAllByRole("button", { name: "Editar nota" })).toHaveLength(2);
  });

  it("offers no editing to whoever may only read", async () => {
    renderPanel({ canWrite: false });
    await screen.findByText("Llegó con fiebre");
    expect(screen.queryByRole("button", { name: "Editar nota" })).not.toBeInTheDocument();
  });

  it("does not send an edit that changes nothing, and cancels back to the note", async () => {
    const user = userEvent.setup();
    renderPanel();
    await screen.findByText("Llegó con fiebre");
    await user.click(screen.getAllByRole("button", { name: "Editar nota" })[0] as HTMLElement);
    await user.click(screen.getByRole("button", { name: "Guardar nota" }));
    expect(api.editNote).not.toHaveBeenCalled();
    expect(screen.queryByLabelText("Texto de la nota")).not.toBeInTheDocument();

    await user.click(screen.getAllByRole("button", { name: "Editar nota" })[0] as HTMLElement);
    await user.type(screen.getByLabelText("Texto de la nota"), " y tos");
    await user.click(screen.getByRole("button", { name: "Cancelar edición" }));
    expect(screen.getByText("Llegó con fiebre")).toBeInTheDocument();
    expect(api.editNote).not.toHaveBeenCalled();
  });

  it("says why the server refused an edit, and keeps what was typed", async () => {
    const user = userEvent.setup();
    api.editNote.mockRejectedValue(new studentsApi.StudentsError(409, "the note was voided or changed"));
    renderPanel();
    await screen.findByText("Llegó con fiebre");
    await user.click(screen.getAllByRole("button", { name: "Editar nota" })[0] as HTMLElement);
    await user.type(screen.getByLabelText("Texto de la nota"), " y tos");
    await user.click(screen.getByRole("button", { name: "Guardar nota" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(/cambió o fue anulada/);
    expect(screen.getByLabelText("Texto de la nota")).toHaveValue("Llegó con fiebre y tos");
  });

  // In a modal or beside a payment, the empty field waits behind a button.
  it("keeps the new-note field behind a button when asked to", async () => {
    const user = userEvent.setup();
    renderPanel({ composer: "onDemand" });
    await screen.findByText("Llegó con fiebre");
    expect(screen.queryByLabelText("Nueva nota")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Agregar nota" }));
    expect(screen.getByLabelText("Nueva nota")).toHaveFocus();
  });

  it("opens the new-note field at once when told to", async () => {
    renderPanel({ composer: "onDemand", startComposing: true });
    expect(await screen.findByLabelText("Nueva nota")).toBeInTheDocument();
  });

  // A refused edit (the note changed or was voided meanwhile) re-reads the
  // history, so the next try starts from what the note says now.
  it("re-reads the notes when an edit is refused as stale", async () => {
    const user = userEvent.setup();
    api.editNote.mockRejectedValue(new studentsApi.StudentsError(409, "changed"));
    renderPanel();
    await screen.findByText("Llegó con fiebre");
    api.listNotes.mockResolvedValue([{ ...mine, body: "Llegó con fiebre (corregida)" }, theirs]);
    await user.click(screen.getAllByRole("button", { name: "Editar nota" })[0] as HTMLElement);
    await user.type(screen.getByLabelText("Texto de la nota"), " y tos");
    await user.click(screen.getByRole("button", { name: "Guardar nota" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(/cambió o fue anulada/);
    await user.click(screen.getByRole("button", { name: "Cancelar edición" }));
    expect(await screen.findByText("Llegó con fiebre (corregida)")).toBeInTheDocument();
  });

  it("says a note no longer exists when the edit answers 404", async () => {
    const user = userEvent.setup();
    api.editNote.mockRejectedValue(new studentsApi.StudentsError(404, "not found"));
    renderPanel();
    await screen.findByText("Llegó con fiebre");
    api.listNotes.mockResolvedValue([theirs]);
    await user.click(screen.getAllByRole("button", { name: "Editar nota" })[0] as HTMLElement);
    await user.type(screen.getByLabelText("Texto de la nota"), " y tos");
    await user.click(screen.getByRole("button", { name: "Guardar nota" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(/ya no existe/);
    await user.click(screen.getByRole("button", { name: "Cancelar edición" }));
    await waitFor(() => expect(screen.queryByText("Llegó con fiebre")).not.toBeInTheDocument());
  });
});
