import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import * as configApi from "../../config/api";
import * as statementApi from "../../finance/statement";
import { StudentsScreen } from "./StudentsScreen";

vi.mock("../../students/api", async (importOriginal) => ({
  ...(await importOriginal<typeof studentsApi>()),
  listStudents: vi.fn(),
  listEnrollments: vi.fn(),
  listNotes: vi.fn(),
  addNote: vi.fn(),
  editNote: vi.fn(),
}));
vi.mock("../../config/api", () => ({
  listCycles: vi.fn(),
  listGroups: vi.fn(),
  listLabelledGroups: vi.fn(),
  listLevels: vi.fn(),
  listGradeLevels: vi.fn(),
}));

vi.mock("../../finance/statement", async (importOriginal) => ({
  ...(await importOriginal<typeof statementApi>()),
  getStatement: vi.fn(),
  downloadStatementPdf: vi.fn(),
}));

const students = vi.mocked(studentsApi);
const statements = vi.mocked(statementApi);
const config = vi.mocked(configApi);

const valentina: studentsApi.Student = {
  id: "st-1", curp: "CURP1", firstName: "Valentina", lastName: "Cruz", level: "primaria", levelId: "",
  gradeGroup: "1° A", guardianName: "Ana Tutor", admissionStatus: "enrolled",
};
const note: studentsApi.StudentNote = {
  id: "n-1", studentId: "st-1", body: "Alérgica a la nuez", authorUserId: "u-me", authorName: "Gabriela Rojas",
  createdAt: "2026-09-24T16:00:00Z", voided: false,
};

beforeEach(() => {
  window.history.replaceState(null, "", "/alumnos");
  students.listStudents.mockResolvedValue([valentina]);
  students.listEnrollments.mockResolvedValue([]);
  students.listNotes.mockResolvedValue([note]);
  config.listCycles.mockResolvedValue([]);
  config.listGroups.mockResolvedValue([]);
  config.listLabelledGroups.mockResolvedValue([]);
  config.listLevels.mockResolvedValue([]);
  config.listGradeLevels.mockResolvedValue([]);
  statements.getStatement.mockResolvedValue({
    movements: [{ kind: "charge", date: "2026-09-10", description: "Colegiatura septiembre", amountCents: 320000, balanceCents: 320000, accrued: false, notYetDue: false, voided: false }],
    totals: { chargedCents: 320000, discountCents: 0, paidCents: 0, balanceCents: 320000, notYetDueCents: 0 },
    dueTodayCents: 320000, surchargeCents: 0, earlyDiscountCents: 0, unallocatedCents: 0, asOf: "2026-10-01",
  });
});
afterEach(() => vi.clearAllMocks());

// T7: the account statement opens from the list, for whoever can see the money.
describe("StudentsScreen · estado de cuenta", () => {
  it("opens a pupil's statement from the row and gives the focus back on close", async () => {
    const user = userEvent.setup();
    render(<StudentsScreen onBack={() => {}} canReadStatement />);
    const trigger = await screen.findByRole("button", { name: "Estado de cuenta de Cruz Valentina" });
    await user.click(trigger);

    const dialog = await screen.findByRole("dialog", { name: "Estado de cuenta · Cruz Valentina" });
    expect(await within(dialog).findByText("Colegiatura septiembre")).toBeInTheDocument();
    expect(statements.getStatement).toHaveBeenCalledWith(expect.any(String), "st-1");

    await user.click(within(dialog).getByRole("button", { name: "Cerrar" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    await waitFor(() => expect(trigger).toHaveFocus());
  });

  it("is not offered to whoever cannot see the money", async () => {
    render(<StudentsScreen onBack={() => {}} />);
    await screen.findByRole("button", { name: "Notas de Cruz Valentina" });
    expect(screen.queryByRole("button", { name: "Estado de cuenta de Cruz Valentina" })).not.toBeInTheDocument();
  });
});
