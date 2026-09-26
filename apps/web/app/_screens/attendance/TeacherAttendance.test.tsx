import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as teachingApi from "../../teaching/api";
import { TeacherAttendance } from "./TeacherAttendance";

// The teacher surface talks to the gateway EXCLUSIVELY through the self-scoped
// teaching client — never the admin attendance/config/students clients (the
// teacher lacks those permissions). Mock only that client. The transport itself
// is covered by app/teaching/api.test.ts.
vi.mock("../../teaching/api", () => ({
  listGroups: vi.fn(),
  getAttendanceRoster: vi.fn(),
  saveAttendance: vi.fn(),
}));

const teaching = vi.mocked(teachingApi);

const group = { id: "g1", name: "1° A" };
const ada = { enrollmentId: "en1", studentId: "st1", studentName: "Ada Lovelace", status: "present" as const };
const alan = { enrollmentId: "en2", studentId: "st2", studentName: "Alan Turing", status: "present" as const };

beforeEach(() => {
  // The picker keeps its place in the URL: every test starts at the first step.
  window.history.replaceState(null, "", "/asistencia");
  teaching.listGroups.mockResolvedValue([group]);
  teaching.getAttendanceRoster.mockResolvedValue([ada, alan]);
  teaching.saveAttendance.mockResolvedValue({ absentCount: 0, markedCount: 0 });
});

afterEach(() => vi.clearAllMocks());

/** One group: the picker skips straight to today's pass list. */
async function openRoster(): Promise<HTMLElement> {
  await waitFor(() => expect(teaching.getAttendanceRoster).toHaveBeenCalledWith("g1", "2026-09-01"));
  return screen.findByRole("table", { name: "Pase de lista" });
}

describe("TeacherAttendance — groups + roster", () => {
  it("lists the teacher's own groups and loads the default-present roster with names from the roster itself", async () => {
    render(<TeacherAttendance onBack={() => {}} today="2026-09-01" />);

    const list = await openRoster();
    // Names come from the roster row — NOT a students-list lookup.
    expect(within(list).getByText("Ada Lovelace")).toBeInTheDocument();
    expect(within(list).getByText("Alan Turing")).toBeInTheDocument();
    // Default-present: every row opens on "Presente".
    const adaRow = screen.getByRole("radiogroup", { name: "Asistencia de Ada Lovelace" });
    expect(within(adaRow).getByRole("radio", { name: "Presente" })).toHaveAttribute("aria-checked", "true");
  });
});

describe("TeacherAttendance — picking the group", () => {
  it("with one group, today's list opens with no step at all", async () => {
    render(<TeacherAttendance onBack={() => {}} today="2026-09-01" />);

    await openRoster();
    expect(screen.queryByRole("heading", { name: "Elige el grupo" })).toBeNull();
    expect(screen.getByRole("navigation", { name: "Ruta" })).toHaveTextContent("Asistencia›1° A");
  });

  it("with two groups, the teacher taps one and gets its list", async () => {
    const user = userEvent.setup();
    teaching.listGroups.mockResolvedValue([group, { id: "g2", name: "2° B" }]);
    render(<TeacherAttendance onBack={() => {}} today="2026-09-01" />);

    await user.click(await screen.findByRole("button", { name: "2° B" }));

    await waitFor(() => expect(teaching.getAttendanceRoster).toHaveBeenCalledWith("g2", "2026-09-01"));
    expect(window.location.search).toBe("?grupo=g2");
  });
});

describe("TeacherAttendance — save", () => {
  it("sends ONLY the exceptions (a student toggled to absent) and reflects the saved counts", async () => {
    const user = userEvent.setup();
    teaching.saveAttendance.mockResolvedValue({ absentCount: 1, markedCount: 1 });
    render(<TeacherAttendance onBack={() => {}} today="2026-09-01" />);

    await openRoster();
    const adaRow = screen.getByRole("radiogroup", { name: "Asistencia de Ada Lovelace" });
    await user.click(within(adaRow).getByRole("radio", { name: "Ausente" }));
    await user.click(screen.getByRole("button", { name: "Guardar lista" }));

    // Default-present reconcile: only the non-present row travels; Alan is omitted.
    await waitFor(() =>
      expect(teaching.saveAttendance).toHaveBeenCalledWith({
        groupId: "g1",
        date: "2026-09-01",
        marks: [{ enrollmentId: "en1", status: "absent" }],
      }),
    );
    // The saved counts are reflected back to the teacher.
    expect(await screen.findByRole("status")).toHaveTextContent(/1/);
  });

  it("sends an empty marks array when nobody is marked (everybody present)", async () => {
    const user = userEvent.setup();
    render(<TeacherAttendance onBack={() => {}} today="2026-09-01" />);

    await openRoster();
    await user.click(screen.getByRole("button", { name: "Guardar lista" }));

    await waitFor(() =>
      expect(teaching.saveAttendance).toHaveBeenCalledWith({ groupId: "g1", date: "2026-09-01", marks: [] }),
    );
  });
});

describe("TeacherAttendance — states", () => {
  it("shows an error when groups fail to load", async () => {
    teaching.listGroups.mockRejectedValue(new Error("boom"));
    render(<TeacherAttendance onBack={() => {}} today="2026-09-01" />);
    expect(await screen.findByRole("alert")).toHaveTextContent(/No se pudieron cargar/);
  });

  it("tells a teacher without groups who to ask", async () => {
    teaching.listGroups.mockResolvedValue([]);
    render(<TeacherAttendance onBack={() => {}} today="2026-09-01" />);
    expect(await screen.findByText(/Aún no tienes grupos asignados\. Pide a la dirección/)).toBeInTheDocument();
  });

  it("shows an empty hint when the chosen group has no enrolled students", async () => {
    teaching.getAttendanceRoster.mockResolvedValue([]);
    render(<TeacherAttendance onBack={() => {}} today="2026-09-01" />);

    expect(await screen.findByText(/no tiene alumnos inscritos/i)).toBeInTheDocument();
  });

  it("shows an error when the roster fails to load", async () => {
    teaching.getAttendanceRoster.mockRejectedValue(new Error("boom"));
    render(<TeacherAttendance onBack={() => {}} today="2026-09-01" />);

    expect(await screen.findByRole("alert")).toHaveTextContent(/No se pudo cargar/);
  });
});
