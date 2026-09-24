import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import { NotesPanel } from "./NotesPanel";

vi.mock("../../students/api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../../students/api")>();
  return { StudentsError: real.StudentsError, listNotes: vi.fn(), addNote: vi.fn(), voidNote: vi.fn() };
});
const api = vi.mocked(studentsApi);

const WRITTEN = Date.parse("2026-09-24T16:00:00Z");
const mine = { id: "n-2", studentId: "st1", body: "Llegó con fiebre", authorUserId: "u-me", authorName: "Gabriela Rojas", createdAt: "2026-09-24T16:00:00Z", voided: false };
const theirs = { id: "n-1", studentId: "st1", body: "Olvidó la mochila", authorUserId: "u-other", authorName: "Luis Pérez", createdAt: "2026-09-23T15:00:00Z", voided: false };

function renderPanel(props: Partial<Parameters<typeof NotesPanel>[0]> = {}) {
  return render(
    <NotesPanel studentId="st1" userId="u-me" canWrite canVoidAny={false} now={() => WRITTEN + 5 * 60_000} {...props} />,
  );
}

beforeEach(() => {
  api.listNotes.mockResolvedValue([mine, theirs]);
  api.voidNote.mockResolvedValue({ ...mine, voided: true });
});
afterEach(() => vi.clearAllMocks());

describe("NotesPanel", () => {
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

  // Reversible: gone at once, told to the server only when the 8 s run out
  // (or the page is left).
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
    expect(api.voidNote).toHaveBeenCalledWith(expect.any(String), "st1", "n-2");
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
    vi.useFakeTimers({ shouldAdvanceTime: true });
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
      vi.useRealTimers();
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
    vi.useFakeTimers({ shouldAdvanceTime: true });
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
      vi.useRealTimers();
    }
  });
});
