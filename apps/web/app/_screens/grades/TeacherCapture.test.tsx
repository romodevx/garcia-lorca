import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as teachingApi from "../../teaching/api";
import { TeacherCapture } from "./TeacherCapture";

// The teacher surface talks to the gateway EXCLUSIVELY through the self-scoped
// teaching client — never grades/config/students (the teacher lacks those
// permissions). Fetch itself is covered by app/teaching/api.test.ts.
vi.mock("../../teaching/api", () => ({
  listAssignments: vi.fn(),
  teachingGradeSheet: { load: vi.fn(), save: vi.fn(), remove: vi.fn() },
}));

const teaching = vi.mocked(teachingApi);
const sheetClient = vi.mocked(teachingApi.teachingGradeSheet);

const assignment = {
  id: "a1",
  cycleId: "c1",
  groupId: "g1",
  subjectId: "s1",
  teacherId: "t1",
  groupName: "1° A",
  subjectName: "Matemáticas",
};

const sheetFor = (assignmentId: string) => ({
  assignmentId,
  month: "2026-09",
  months: ["2026-09"],
  scale: { min: 5, max: 10, decimals: 1, passing: 6 },
  editable: true,
  rows: [{ enrollmentId: "en1", studentId: "st1", name: "Ada Lovelace", value: 8, updatedAt: null }],
});

beforeEach(() => {
  // The picker keeps its place in the URL: every test starts at the first step.
  window.history.replaceState(null, "", "/calificaciones");
  teaching.listAssignments.mockResolvedValue([assignment]);
  sheetClient.load.mockImplementation(async (q) => sheetFor(q.assignmentId));
  sheetClient.save.mockImplementation(async (i) => ({ enrollmentId: i.enrollmentId, month: i.month, value: i.value, updatedAt: "x" }));
});

afterEach(() => vi.clearAllMocks());

describe("TeacherCapture — the monthly sheet", () => {
  it("with a single class opens its sheet directly, names from the sheet itself", async () => {
    render(<TeacherCapture onBack={() => {}} />);

    expect(await screen.findByRole("textbox", { name: "Calificación de Ada Lovelace" })).toHaveValue("8");
    expect(sheetClient.load).toHaveBeenCalledWith({ assignmentId: "a1" });
    // Monthly now: there is no term to pick.
    expect(screen.queryByLabelText("Periodo")).toBeNull();
  });

  it("saves a cell through the teacher's own client", async () => {
    const user = userEvent.setup();
    render(<TeacherCapture onBack={() => {}} />);

    const cell = await screen.findByRole("textbox", { name: "Calificación de Ada Lovelace" });
    await user.clear(cell);
    await user.type(cell, "9.5{Enter}");

    await waitFor(() => expect(sheetClient.save).toHaveBeenCalledWith({ assignmentId: "a1", month: "2026-09", enrollmentId: "en1", value: 9.5 }));
  });
});

describe("TeacherCapture — picking the class", () => {
  const spanish2B = { ...assignment, id: "a2", groupId: "g2", subjectId: "s2", groupName: "2° B", subjectName: "Español" };

  it("with two subjects, the teacher taps the subject and lands on its only group's sheet", async () => {
    const user = userEvent.setup();
    teaching.listAssignments.mockResolvedValue([assignment, spanish2B]);
    render(<TeacherCapture onBack={() => {}} />);

    await user.click(await screen.findByRole("button", { name: /^Español/ }));

    expect(screen.getByRole("navigation", { name: "Ruta" })).toHaveTextContent("Calificaciones›Español›2° B");
    await waitFor(() => expect(sheetClient.load).toHaveBeenCalledWith({ assignmentId: "a2" }));
  });

  it("switching class with Cambiar loads the other class's sheet", async () => {
    const user = userEvent.setup();
    teaching.listAssignments.mockResolvedValue([assignment, spanish2B]);
    window.history.replaceState(null, "", "/calificaciones?asignatura=s1");
    render(<TeacherCapture onBack={() => {}} />);
    await screen.findByRole("textbox", { name: "Calificación de Ada Lovelace" });

    await user.click(screen.getByRole("button", { name: "Cambiar" }));
    await user.click(screen.getByRole("button", { name: "Español · 2° B" }));

    await waitFor(() => expect(sheetClient.load).toHaveBeenLastCalledWith({ assignmentId: "a2" }));
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
