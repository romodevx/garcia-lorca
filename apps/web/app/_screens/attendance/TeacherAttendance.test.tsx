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
  teaching.listGroups.mockResolvedValue([group]);
  teaching.getAttendanceRoster.mockResolvedValue([ada, alan]);
  teaching.saveAttendance.mockResolvedValue({ absentCount: 0, markedCount: 0 });
});

afterEach(() => vi.clearAllMocks());

/** Picks the group (date defaults to `today`) and waits for the pass list. */
async function openRoster(user: ReturnType<typeof userEvent.setup>): Promise<HTMLElement> {
  const picker = await screen.findByRole("form", { name: "Elegir grupo y fecha" });
  await within(picker).findByRole("option", { name: "1° A" });
  await user.selectOptions(within(picker).getByLabelText("Grupo"), "g1");

  await waitFor(() => expect(teaching.getAttendanceRoster).toHaveBeenCalledWith("g1", "2026-09-01"));
  return screen.findByRole("table", { name: "Pase de lista" });
}

describe("TeacherAttendance — groups + roster", () => {
  it("lists the teacher's own groups and loads the default-present roster with names from the roster itself", async () => {
    const user = userEvent.setup();
    render(<TeacherAttendance onBack={() => {}} today="2026-09-01" />);

    const list = await openRoster(user);
    // Names come from the roster row — NOT a students-list lookup.
    expect(within(list).getByText("Ada Lovelace")).toBeInTheDocument();
    expect(within(list).getByText("Alan Turing")).toBeInTheDocument();
    // Default-present: every row opens on "Presente".
    const adaRow = screen.getByRole("radiogroup", { name: "Asistencia de Ada Lovelace" });
    expect(within(adaRow).getByRole("radio", { name: "Presente" })).toHaveAttribute("aria-checked", "true");
  });
});

describe("TeacherAttendance — save", () => {
  it("sends ONLY the exceptions (a student toggled to absent) and reflects the saved counts", async () => {
    const user = userEvent.setup();
    teaching.saveAttendance.mockResolvedValue({ absentCount: 1, markedCount: 1 });
    render(<TeacherAttendance onBack={() => {}} today="2026-09-01" />);

    await openRoster(user);
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

    await openRoster(user);
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

  it("shows an empty hint when the teacher has no groups", async () => {
    teaching.listGroups.mockResolvedValue([]);
    render(<TeacherAttendance onBack={() => {}} today="2026-09-01" />);
    expect(await screen.findByText(/No tienes grupos/)).toBeInTheDocument();
  });

  it("shows an empty hint when the chosen group has no enrolled students", async () => {
    const user = userEvent.setup();
    teaching.getAttendanceRoster.mockResolvedValue([]);
    render(<TeacherAttendance onBack={() => {}} today="2026-09-01" />);

    const picker = await screen.findByRole("form", { name: "Elegir grupo y fecha" });
    await within(picker).findByRole("option", { name: "1° A" });
    await user.selectOptions(within(picker).getByLabelText("Grupo"), "g1");

    expect(await screen.findByText(/no tiene alumnos inscritos/i)).toBeInTheDocument();
  });

  it("shows an error when the roster fails to load", async () => {
    const user = userEvent.setup();
    teaching.getAttendanceRoster.mockRejectedValue(new Error("boom"));
    render(<TeacherAttendance onBack={() => {}} today="2026-09-01" />);

    const picker = await screen.findByRole("form", { name: "Elegir grupo y fecha" });
    await within(picker).findByRole("option", { name: "1° A" });
    await user.selectOptions(within(picker).getByLabelText("Grupo"), "g1");

    expect(await screen.findByRole("alert")).toHaveTextContent(/No se pudo cargar/);
  });
});
