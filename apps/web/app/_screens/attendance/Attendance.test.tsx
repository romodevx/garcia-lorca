import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as attendanceApi from "../../attendance/api";
import * as configApi from "../../config/api";
import * as studentsApi from "../../students/api";
import { Providers } from "../../test-utils";
import { Attendance } from "./Attendance";

// The surface talks to the gateway only through the attendance/config/students
// api clients, so we mock them wholesale and assert the calls (the fetch itself
// is covered by app/attendance/api.test.ts). Neutral literals only.
vi.mock("../../attendance/api", () => ({
  getDailyRoster: vi.fn(),
  saveDailyAttendance: vi.fn(),
  listAttendance: vi.fn(),
}));
vi.mock("../../config/api", () => ({
  listCycles: vi.fn(),
  listGroups: vi.fn(),
}));
vi.mock("../../students/api", () => ({
  listStudents: vi.fn(),
}));

const attendance = vi.mocked(attendanceApi);
const config = vi.mocked(configApi);
const students = vi.mocked(studentsApi);

// The screen is rendered with a FIXED date so the suite never depends on "today".
const DATE = "2026-08-06";

const cycle = { id: "cy1", name: "2026-2027", status: "active" as const, startsOn: "2026-08-01", endsOn: "2027-06-30" };
const group = { id: "grp1", cycleId: "cy1", gradeLevelId: "gl1", name: "4° A", capacity: 30 };

const ada = {
  id: "st1",
  curp: "AAAA000000AAAAAA01",
  firstName: "Ada",
  lastName: "Lovelace",
  level: "Primaria",
  levelId: "",
  gradeGroup: "4° A",
  guardianName: "Anne Byron",
  admissionStatus: "enrolled" as const,
};
const alan = { ...ada, id: "st2", curp: "AAAA000000AAAAAA02", firstName: "Alan", lastName: "Turing" };

const rosterAllPresent = [
  { enrollmentId: "en1", studentId: "st1", status: "present" as const },
  { enrollmentId: "en2", studentId: "st2", status: "present" as const },
];

beforeEach(() => {
  config.listCycles.mockResolvedValue([cycle]);
  config.listGroups.mockResolvedValue([group]);
  students.listStudents.mockResolvedValue([ada, alan]);
  attendance.getDailyRoster.mockResolvedValue(rosterAllPresent);
  attendance.saveDailyAttendance.mockResolvedValue({ absentCount: 1, markedCount: 1 });
});

afterEach(() => vi.clearAllMocks());

function renderScreen(): void {
  render(
    <Providers>
      <Attendance onBack={() => {}} today={DATE} />
    </Providers>,
  );
}

/** Picks the group and waits for the day's pass list to render. */
async function openList(user: ReturnType<typeof userEvent.setup>): Promise<HTMLElement> {
  const picker = await screen.findByRole("form", { name: "Elegir grupo y fecha" });
  await within(picker).findByRole("option", { name: "4° A" });
  await user.selectOptions(within(picker).getByLabelText("Grupo"), "grp1");

  await waitFor(() => expect(attendance.getDailyRoster).toHaveBeenCalledWith(expect.any(String), { groupId: "grp1", date: DATE }));
  return screen.findByRole("table", { name: "Lista de asistencia" });
}

/** The status control of one student's row. */
function rowOf(list: HTMLElement, name: string): HTMLElement {
  return within(list).getByRole("radiogroup", { name: `Asistencia de ${name}` });
}

function isChecked(row: HTMLElement, status: string): boolean {
  return within(row).getByRole("radio", { name: status }).getAttribute("aria-checked") === "true";
}

describe("Attendance — pass list", () => {
  it("renders every enrolled student PRESENT by default, resolving names from the students list", async () => {
    const user = userEvent.setup();
    renderScreen();

    const list = await openList(user);
    expect(within(list).getByText("Ada Lovelace")).toBeInTheDocument();
    expect(within(list).getByText("Alan Turing")).toBeInTheDocument();
    expect(isChecked(rowOf(list, "Ada Lovelace"), "Presente")).toBe(true);
    expect(isChecked(rowOf(list, "Alan Turing"), "Presente")).toBe(true);
  });

  it("reflects a stored exception when the day was already taken", async () => {
    attendance.getDailyRoster.mockResolvedValue([
      { enrollmentId: "en1", studentId: "st1", status: "late" as const },
      { enrollmentId: "en2", studentId: "st2", status: "present" as const },
    ]);
    const user = userEvent.setup();
    renderScreen();

    const list = await openList(user);
    expect(isChecked(rowOf(list, "Ada Lovelace"), "Retardo")).toBe(true);
    expect(isChecked(rowOf(list, "Alan Turing"), "Presente")).toBe(true);
  });
});

describe("Attendance — save", () => {
  it("sends ONLY the marked exception and never a present student", async () => {
    const user = userEvent.setup();
    renderScreen();

    const list = await openList(user);
    await user.click(within(rowOf(list, "Alan Turing")).getByRole("radio", { name: "Ausente" }));
    await user.click(screen.getByRole("button", { name: "Guardar" }));

    await waitFor(() =>
      expect(attendance.saveDailyAttendance).toHaveBeenCalledWith(expect.any(String), {
        groupId: "grp1",
        date: DATE,
        marks: [{ enrollmentId: "en2", status: "absent" }],
      }),
    );
    const payload = attendance.saveDailyAttendance.mock.calls[0]?.[1];
    expect(payload?.marks).toHaveLength(1);
    expect(payload?.marks.some((mark) => mark.enrollmentId === "en1")).toBe(false);
  });

  it("keeps the saved state on screen and confirms with the absent count", async () => {
    const user = userEvent.setup();
    renderScreen();

    const list = await openList(user);
    await user.click(within(rowOf(list, "Alan Turing")).getByRole("radio", { name: "Ausente" }));
    await user.click(screen.getByRole("button", { name: "Guardar" }));

    expect(await screen.findByRole("status")).toHaveTextContent("1 ausente");
    expect(isChecked(rowOf(list, "Alan Turing"), "Ausente")).toBe(true);
    expect(isChecked(rowOf(list, "Ada Lovelace"), "Presente")).toBe(true);
  });

  it("saves an all-present day with an empty marks array", async () => {
    attendance.saveDailyAttendance.mockResolvedValue({ absentCount: 0, markedCount: 0 });
    const user = userEvent.setup();
    renderScreen();

    await openList(user);
    await user.click(screen.getByRole("button", { name: "Guardar" }));

    await waitFor(() =>
      expect(attendance.saveDailyAttendance).toHaveBeenCalledWith(expect.any(String), {
        groupId: "grp1",
        date: DATE,
        marks: [],
      }),
    );
  });

  it("surfaces a save failure instead of pretending the day was stored", async () => {
    attendance.saveDailyAttendance.mockRejectedValue(new Error("HTTP 403"));
    const user = userEvent.setup();
    renderScreen();

    const list = await openList(user);
    await user.click(within(rowOf(list, "Alan Turing")).getByRole("radio", { name: "Ausente" }));
    await user.click(screen.getByRole("button", { name: "Guardar" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("No se pudo guardar la lista");
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });
});

describe("Attendance — read failure", () => {
  it("shows the roster error and no list when the day cannot be loaded", async () => {
    attendance.getDailyRoster.mockRejectedValue(new Error("HTTP 500"));
    const user = userEvent.setup();
    renderScreen();

    const picker = await screen.findByRole("form", { name: "Elegir grupo y fecha" });
    await within(picker).findByRole("option", { name: "4° A" });
    await user.selectOptions(within(picker).getByLabelText("Grupo"), "grp1");

    expect(await screen.findByRole("alert")).toHaveTextContent("No se pudo cargar la lista");
    expect(screen.queryByRole("table", { name: "Lista de asistencia" })).not.toBeInTheDocument();
  });
});
