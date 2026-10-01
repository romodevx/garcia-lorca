import { act, render, renderHook, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import { StudentNotesBeside, useNotesBeside, type NotesAccess } from "./StudentNotesBeside";
import { useStudentNotes } from "./useStudentNotes";

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
        <input aria-label="Monto" />
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

  // The payment form is the same element before and after the notes arrive:
  // remounting it would drop the focus the drawer put on its first field.
  it("keeps the payment form mounted, and focused, when the notes arrive", async () => {
    let answer: (notes: studentsApi.StudentNote[]) => void = () => {};
    api.listNotes.mockReturnValue(new Promise((resolve) => { answer = resolve; }));
    render(<Harness access={cashier} />);
    const amount = screen.getByLabelText("Monto");
    amount.focus();
    await act(async () => answer([note]));
    expect(await screen.findByRole("complementary", { name: "Notas del alumno" })).toBeInTheDocument();
    expect(screen.getByLabelText("Monto")).toBe(amount);
    expect(amount).toHaveFocus();
  });

  // A failed read is not "this family has no notes".
  it("says the notes could not be read, instead of looking empty", async () => {
    api.listNotes.mockRejectedValue(new Error("boom"));
    render(<Harness access={{ ...cashier, canWrite: false }} />);
    expect(await screen.findByRole("alert")).toHaveTextContent("No pudimos leer las notas");
  });
});

describe("useStudentNotes", () => {
  // A slow answer for the previous pupil never lands under the next one.
  it("drops an answer that arrives after the pupil changed", async () => {
    let answerA: (notes: studentsApi.StudentNote[]) => void = () => {};
    api.listNotes.mockImplementation((_base, id) =>
      id === "a" ? new Promise((resolve) => { answerA = resolve; }) : Promise.resolve([{ ...note, studentId: "b", body: "de B" }]));
    const { result, rerender } = renderHook(({ id }) => useStudentNotes(id), { initialProps: { id: "a" } });
    rerender({ id: "b" });
    await waitFor(() => expect(result.current.notes[0]?.body).toBe("de B"));
    await act(async () => answerA([{ ...note, studentId: "a", body: "de A" }]));
    expect(result.current.notes.map((n) => n.body)).toEqual(["de B"]);
  });
});
