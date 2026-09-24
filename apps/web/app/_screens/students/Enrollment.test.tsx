import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import * as configApi from "../../config/api";
import { Enrollment } from "./Enrollment";

// The surface talks to the gateway exclusively through the students + config api
// clients, so we mock both wholesale and assert the calls (the fetch itself is
// covered by app/students/api.test.ts). Neutral literals only.
vi.mock("../../students/api", () => ({
  listStudents: vi.fn(),
  createStudent: vi.fn(),
  enroll: vi.fn(),
  listEnrollments: vi.fn(),
  transfer: vi.fn(),
  withdraw: vi.fn(),
}));
vi.mock("../../config/api", () => ({
  listCycles: vi.fn(),
  listGroups: vi.fn(),
}));

const students = vi.mocked(studentsApi);
const config = vi.mocked(configApi);

const ada = {
  id: "st1",
  curp: "AAAA000000AAAAAA01",
  firstName: "Ada",
  lastName: "Lovelace",
  level: "Primaria",
  levelId: "",
  gradeGroup: "4° A",
  guardianName: "Marisol Ríos",
  admissionStatus: "accepted" as const,
};
const grace = {
  id: "st2",
  curp: "BBBB000000BBBBBB02",
  firstName: "Grace",
  lastName: "Hopper",
  level: "Primaria",
  levelId: "",
  gradeGroup: "4° A",
  guardianName: "Ana Mendoza",
  admissionStatus: "accepted" as const,
};

const cycle = { id: "c1", name: "2026-2027", status: "active" as const, startsOn: "2026-08-01", endsOn: "2027-06-30" };
const groupA = { id: "grp1", cycleId: "c1", gradeLevelId: "g1", name: "A", capacity: 30 };
const groupB = { id: "grp2", cycleId: "c1", gradeLevelId: "g1", name: "B", capacity: 30 };

const enrollmentAda = {
  id: "en1",
  studentId: "st1",
  cycleId: "c1",
  groupId: "grp1",
  termId: "",
  status: "active" as const,
  enrolledAt: "2026-08-06T00:00:00Z",
};

beforeEach(() => {
  students.listStudents.mockResolvedValue([ada, grace]);
  students.listEnrollments.mockResolvedValue([]);
  config.listCycles.mockResolvedValue([cycle]);
  config.listGroups.mockResolvedValue([groupA, groupB]);
});

afterEach(() => vi.clearAllMocks());

describe("Enrollment — enroll", () => {
  it("enrolls a student into a group and NEVER sends cycleId (server derives it)", async () => {
    const user = userEvent.setup();
    students.enroll.mockResolvedValue(enrollmentAda);

    render(<Enrollment onBack={() => {}} />);

    const form = await screen.findByRole("form", { name: "Inscribir alumno" });
    // Cycle auto-selected (active) → its groups load for the group dropdown.
    await waitFor(() => expect(config.listGroups).toHaveBeenCalledWith(expect.any(String), "c1"));

    await user.selectOptions(within(form).getByLabelText("Alumno"), "st1");
    await user.selectOptions(within(form).getByLabelText("Grupo"), "grp1");
    await user.click(within(form).getByRole("button", { name: "Inscribir alumno" }));

    await waitFor(() =>
      expect(students.enroll).toHaveBeenCalledWith(expect.any(String), { studentId: "st1", groupId: "grp1" }),
    );
    const payload = students.enroll.mock.calls[0]?.[1];
    expect(payload).not.toHaveProperty("cycleId");
  });
});

describe("Enrollment — roster", () => {
  it("lists the students enrolled in the selected group, resolving names by id", async () => {
    const user = userEvent.setup();
    students.listEnrollments.mockResolvedValue([enrollmentAda]);

    render(<Enrollment onBack={() => {}} />);

    const roster = await screen.findByRole("form", { name: "Ver roster del grupo" });
    const groupSelect = within(roster).getByLabelText("Grupo");
    await within(groupSelect).findByRole("option", { name: "A" }); // wait for the cycle's groups to load
    await user.selectOptions(groupSelect, "grp1");

    await waitFor(() => expect(students.listEnrollments).toHaveBeenCalledWith(expect.any(String), { groupId: "grp1" }));
    const list = await screen.findByRole("list", { name: "Alumnos inscritos" });
    expect(within(list).getByText(/Ada Lovelace/)).toBeInTheDocument();
  });
});

describe("Enrollment — transfer", () => {
  it("requires a reason and transfers to another group in the same cycle", async () => {
    const user = userEvent.setup();
    students.listEnrollments.mockResolvedValue([enrollmentAda]);
    students.transfer.mockResolvedValue({ ...enrollmentAda, groupId: "grp2" });

    render(<Enrollment onBack={() => {}} />);

    const roster = await screen.findByRole("form", { name: "Ver roster del grupo" });
    const groupSelect = within(roster).getByLabelText("Grupo");
    await within(groupSelect).findByRole("option", { name: "A" }); // wait for the cycle's groups to load
    await user.selectOptions(groupSelect, "grp1");
    const list = await screen.findByRole("list", { name: "Alumnos inscritos" });
    await within(list).findByText(/Ada Lovelace/);

    const transferForm = await screen.findByRole("form", { name: "Trasladar alumno" });
    // The transfer button is disabled until a target group AND a reason are set.
    await user.selectOptions(within(transferForm).getByLabelText("Nuevo grupo"), "grp2");
    const submit = within(transferForm).getByRole("button", { name: "Trasladar" });
    expect(submit).toBeDisabled();

    await user.type(within(transferForm).getByLabelText("Motivo del traslado"), "cambio de turno");
    expect(submit).toBeEnabled();
    await user.click(submit);

    await waitFor(() =>
      expect(students.transfer).toHaveBeenCalledWith(expect.any(String), {
        enrollmentId: "en1",
        toGroupId: "grp2",
        reason: "cambio de turno",
      }),
    );
  });

  it("withdraws an active enrollment", async () => {
    const user = userEvent.setup();
    students.listEnrollments.mockResolvedValue([enrollmentAda]);
    students.withdraw.mockResolvedValue({ ...enrollmentAda, status: "withdrawn" });

    render(<Enrollment onBack={() => {}} />);

    const roster = await screen.findByRole("form", { name: "Ver roster del grupo" });
    const groupSelect = within(roster).getByLabelText("Grupo");
    await within(groupSelect).findByRole("option", { name: "A" }); // wait for the cycle's groups to load
    await user.selectOptions(groupSelect, "grp1");
    const list = await screen.findByRole("list", { name: "Alumnos inscritos" });
    await within(list).findByText(/Ada Lovelace/);

    await user.click(screen.getByRole("button", { name: "Dar de baja" }));

    await waitFor(() => expect(students.withdraw).toHaveBeenCalledWith(expect.any(String), "en1"));
  });
});
