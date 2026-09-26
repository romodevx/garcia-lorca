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

/** The field itself, not its (?) help button. */
const CAMPO = { selector: "input, select, textarea" } as const;

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
  // The picker keeps its place in the URL: every test starts at the first step.
  window.history.replaceState(null, "", "/calificaciones");
  teaching.listAssignments.mockResolvedValue([assignment]);
  teaching.listTerms.mockResolvedValue([otherCycleTerm, term]);
  teaching.getRoster.mockResolvedValue([rosterRow]);
});

afterEach(() => vi.clearAllMocks());

/**
 * With a single assignment the picker skips both steps: the capture list opens
 * directly, and the teacher only picks the term. Waits for the roster grid.
 */
async function openRoster(user: ReturnType<typeof userEvent.setup>): Promise<HTMLElement> {
  const picker = await screen.findByRole("form", { name: "Elegir periodo" });
  // Only terms of the assignment's cycle (c1) are offered — never other cycles.
  await within(picker).findByRole("option", { name: "Bloque 1" });
  expect(within(picker).queryByRole("option", { name: "Otro ciclo" })).not.toBeInTheDocument();
  await user.selectOptions(within(picker).getByLabelText("Periodo", CAMPO), "term1");

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

describe("TeacherCapture — picking the class", () => {
  const spanish2B = { ...assignment, id: "a2", groupId: "g2", subjectId: "s2", groupName: "2° B", subjectName: "Español" };

  it("with two subjects, the teacher taps the subject and lands on its only group's list", async () => {
    const user = userEvent.setup();
    teaching.listAssignments.mockResolvedValue([assignment, spanish2B]);
    render(<TeacherCapture onBack={() => {}} />);

    await user.click(await screen.findByRole("button", { name: /^Español/ }));

    expect(screen.getByRole("navigation", { name: "Ruta" })).toHaveTextContent("Calificaciones›Español›2° B");
    await user.selectOptions(screen.getByLabelText("Periodo", CAMPO), "term1");
    await waitFor(() => expect(teaching.getRoster).toHaveBeenCalledWith("a2", "term1"));
  });

  it("switching class with Cambiar starts the new list without the old term", async () => {
    const user = userEvent.setup();
    teaching.listAssignments.mockResolvedValue([assignment, spanish2B]);
    window.history.replaceState(null, "", "/calificaciones?asignatura=Matem%C3%A1ticas");
    render(<TeacherCapture onBack={() => {}} />);
    await openRoster(user);

    await user.click(screen.getByRole("button", { name: "Cambiar" }));
    await user.click(screen.getByRole("button", { name: "Español · 2° B" }));

    expect(screen.getByLabelText("Periodo", CAMPO)).toHaveValue("");
    expect(screen.queryByRole("table", { name: "Roster de calificaciones" })).toBeNull();
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

  it("tells a teacher without assignments who to ask", async () => {
    teaching.listAssignments.mockResolvedValue([]);
    render(<TeacherCapture onBack={() => {}} />);
    expect(await screen.findByText(/Aún no tienes grupos asignados\. Pide a la dirección/)).toBeInTheDocument();
  });
});
