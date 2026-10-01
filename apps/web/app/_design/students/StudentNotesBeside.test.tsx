import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import { StudentNotesBeside, useNotesBeside, type NotesAccess } from "./StudentNotesBeside";

vi.mock("../../students/api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../../students/api")>();
  return { StudentsError: real.StudentsError, listNotes: vi.fn(), addNote: vi.fn(), voidNote: vi.fn(), editNote: vi.fn() };
});
const api = vi.mocked(studentsApi);

const note = {
  id: "n-1", studentId: "st1", body: "Paga el papá los viernes", authorUserId: "u-other", authorName: "Luis Pérez",
  createdAt: "2026-09-23T15:00:00Z", voided: false,
};
const cashier: NotesAccess = { userId: "u-me", canRead: true, canWrite: true, canVoidAny: false };

function Harness({ access }: { access?: NotesAccess }) {
  const beside = useNotesBeside("st1", access);
  return (
    <div data-testid="drawer" data-wide={String(beside.wide)}>
      <StudentNotesBeside beside={beside} studentId="st1">
        <p>forma de cobro</p>
      </StudentNotesBeside>
    </div>
  );
}

afterEach(() => vi.clearAllMocks());

// T6: Registrar pago shows the pupil's notes on its left when there are any.
describe("StudentNotesBeside", () => {
  it("shows the notes in a panel beside the form, and widens the drawer", async () => {
    api.listNotes.mockResolvedValue([note]);
    render(<Harness access={cashier} />);
    const panel = await screen.findByRole("complementary", { name: "Notas del alumno" });
    expect(panel).toHaveTextContent("Paga el papá los viernes");
    expect(screen.getByText("forma de cobro")).toBeInTheDocument();
    expect(screen.getByTestId("drawer")).toHaveAttribute("data-wide", "true");
    // The empty field waits behind a button: the counter's first field stays the payment's.
    expect(screen.queryByLabelText("Nueva nota")).not.toBeInTheDocument();
  });

  it("offers 'Agregar nota' when there are none, and widens to capture it", async () => {
    const user = userEvent.setup();
    api.listNotes.mockResolvedValue([]);
    api.addNote.mockResolvedValue({ ...note, id: "n-2", body: "Pidió factura", authorUserId: "u-me" });
    render(<Harness access={cashier} />);
    await user.click(await screen.findByRole("button", { name: "Agregar nota" }));

    expect(screen.getByTestId("drawer")).toHaveAttribute("data-wide", "true");
    const field = screen.getByLabelText("Nueva nota");
    expect(field).toHaveFocus();
    await user.type(field, "Pidió factura");
    await user.click(screen.getByRole("button", { name: "Agregar nota" }));
    expect(await screen.findByText("Pidió factura")).toBeInTheDocument();
    expect(api.addNote).toHaveBeenCalledWith(expect.any(String), "st1", "Pidió factura");
  });

  it("shows nothing extra to whoever may read but not write, when there are no notes", async () => {
    api.listNotes.mockResolvedValue([]);
    render(<Harness access={{ ...cashier, canWrite: false }} />);
    await vi.waitFor(() => expect(api.listNotes).toHaveBeenCalled());
    expect(screen.queryByRole("button", { name: "Agregar nota" })).not.toBeInTheDocument();
    expect(screen.queryByRole("complementary")).not.toBeInTheDocument();
    expect(screen.getByTestId("drawer")).toHaveAttribute("data-wide", "false");
  });

  it("reads nothing for whoever may not read the pupil", () => {
    render(<Harness />);
    expect(api.listNotes).not.toHaveBeenCalled();
    expect(screen.getByText("forma de cobro")).toBeInTheDocument();
  });
});
