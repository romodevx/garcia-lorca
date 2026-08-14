import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as teachingApi from "../../teaching/api";
import { TeacherCapture } from "./TeacherCapture";

// The teacher surface talks to the gateway EXCLUSIVELY through the self-scoped
// teaching client — never grades/config/students (the teacher lacks those
// permissions). Mock only that client. Fetch itself is covered by
// app/teaching/api.test.ts.
vi.mock("../../teaching/api", () => ({
  listAssignments: vi.fn(),
  listTerms: vi.fn(),
  getRoster: vi.fn(),
  captureGrade: vi.fn(),
}));

const teaching = vi.mocked(teachingApi);

const assignment = {
  id: "a1",
  cycleId: "c1",
  groupId: "g1",
  subjectId: "s1",
  teacherId: "t1",
  groupName: "1° A",
  subjectName: "Matemáticas",
};
const otherCycleTerm = { id: "termX", name: "Otro ciclo", cycleId: "cZ" };
const term = { id: "term1", name: "Bloque 1", cycleId: "c1" };
const rosterRow = { enrollmentId: "en1", studentId: "st1", studentName: "Ada Lovelace", value: 8, hasValue: true };

beforeEach(() => {
  teaching.listAssignments.mockResolvedValue([assignment]);
  teaching.listTerms.mockResolvedValue([otherCycleTerm, term]);
  teaching.getRoster.mockResolvedValue([rosterRow]);
});

afterEach(() => vi.clearAllMocks());

/** Selects the assignment + term and waits for the roster grid to render. */
async function openRoster(user: ReturnType<typeof userEvent.setup>): Promise<HTMLElement> {
  const picker = await screen.findByRole("form", { name: "Elegir asignación y periodo" });
  // The assignment option shows the roster-provided "group · subject" label.
  await within(picker).findByRole("option", { name: "1° A · Matemáticas" });
  await user.selectOptions(within(picker).getByLabelText("Asignación"), "a1");
  // Only terms of the assignment's cycle (c1) are offered — never other cycles.
  await within(picker).findByRole("option", { name: "Bloque 1" });
  expect(within(picker).queryByRole("option", { name: "Otro ciclo" })).not.toBeInTheDocument();
  await user.selectOptions(within(picker).getByLabelText("Periodo"), "term1");

  await waitFor(() => expect(teaching.getRoster).toHaveBeenCalledWith("a1", "term1"));
  return screen.findByRole("table", { name: "Roster de calificaciones" });
}

describe("TeacherCapture — assignments + roster", () => {
  it("lists the teacher's own assignments and renders the roster with names from the roster itself", async () => {
    const user = userEvent.setup();
    render(<TeacherCapture onBack={() => {}} />);

    const grid = await openRoster(user);
    // The student name comes from the roster row — NOT a students-list lookup.
    expect(within(grid).getByText("Ada Lovelace")).toBeInTheDocument();
    expect(within(grid).getByLabelText("Calificación de Ada Lovelace")).toHaveValue(8);
  });
});

describe("TeacherCapture — capture", () => {
  it("captures a grade with {assignmentId, enrollmentId, termId, value} and NO subject/teacher", async () => {
    const user = userEvent.setup();
    teaching.captureGrade.mockResolvedValue({ id: "ge1", enrollmentId: "en1", subjectId: "s1", termId: "term1", teacherId: "t1", value: 9.5 });
    render(<TeacherCapture onBack={() => {}} />);

    const grid = await openRoster(user);
    const input = within(grid).getByLabelText("Calificación de Ada Lovelace");
    await user.clear(input);
    await user.type(input, "9.5");
    await user.click(within(grid).getByRole("button", { name: "Guardar" }));

    await waitFor(() =>
      expect(teaching.captureGrade).toHaveBeenCalledWith({
        assignmentId: "a1",
        enrollmentId: "en1",
        termId: "term1",
        value: 9.5,
      }),
    );
    const payload = teaching.captureGrade.mock.calls[0]?.[0];
    expect(payload).not.toHaveProperty("subjectId");
    expect(payload).not.toHaveProperty("teacherId");
  });

  it("blocks an out-of-range value (11) client-side without calling the api", async () => {
    const user = userEvent.setup();
    render(<TeacherCapture onBack={() => {}} />);

    const grid = await openRoster(user);
    const input = within(grid).getByLabelText("Calificación de Ada Lovelace");
    await user.clear(input);
    await user.type(input, "11");
    await user.click(within(grid).getByRole("button", { name: "Guardar" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/entre 0 y 10/);
    expect(teaching.captureGrade).not.toHaveBeenCalled();
  });
});

describe("TeacherCapture — states", () => {
  it("shows an error when assignments fail to load", async () => {
    teaching.listAssignments.mockRejectedValue(new Error("boom"));
    render(<TeacherCapture onBack={() => {}} />);
    expect(await screen.findByRole("alert")).toHaveTextContent(/No se pudieron cargar/);
  });

  it("shows an empty hint when the teacher has no assignments", async () => {
    teaching.listAssignments.mockResolvedValue([]);
    render(<TeacherCapture onBack={() => {}} />);
    expect(await screen.findByText(/No tienes asignaciones/)).toBeInTheDocument();
  });
});
