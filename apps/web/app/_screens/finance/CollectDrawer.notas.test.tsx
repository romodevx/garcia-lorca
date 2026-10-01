import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as financeApi from "../../finance/api";
import * as studentsApi from "../../students/api";
import { CollectDrawer } from "./CollectDrawer";

vi.mock("../../finance/api", () => ({
  getAccount: vi.fn(),
  getExpected: vi.fn(),
  previewDeposit: vi.fn(),
  registerDeposit: vi.fn(),
}));
vi.mock("../../students/api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../../students/api")>();
  return { StudentsError: real.StudentsError, listNotes: vi.fn(), addNote: vi.fn(), voidNote: vi.fn(), editNote: vi.fn() };
});

const finance = vi.mocked(financeApi);
const students = vi.mocked(studentsApi);

const student = { id: "s1", firstName: "Valentina", lastName: "Cruz", gradeGroup: "1° A" };
const cashier = { userId: "u-caja", canRead: true, canWrite: true, canVoidAny: false };
const note = {
  id: "n-1", studentId: "s1", body: "Tiene convenio: abona cada quincena", authorUserId: "u-dir", authorName: "Dirección",
  createdAt: "2026-09-20T15:00:00Z", voided: false,
};

beforeEach(() => {
  finance.getAccount.mockResolvedValue({
    charges: [], payments: [], totalBalanceCents: 0, unallocatedCents: 0, dueTodayCents: 0,
    surchargeCents: 0, earlyDiscountCents: 0, asOf: "",
  });
  finance.getExpected.mockResolvedValue({ amountCents: 0, surchargeCents: 0, discountCents: 0, annualBalanceCents: 0, lines: [] });
  finance.previewDeposit.mockResolvedValue({ allocations: [], surcharges: [], unallocatedCents: 0 });
});
afterEach(() => vi.clearAllMocks());

// T6: the pupil's notes beside Registrar pago — the same source as the sheet.
describe("CollectDrawer · notas", () => {
  it("shows the pupil's notes on the left, in a wider drawer", async () => {
    students.listNotes.mockResolvedValue([note]);
    render(<CollectDrawer student={student} notes={cashier} onClose={() => {}} onCollected={() => {}} />);
    const panel = await screen.findByRole("complementary", { name: "Notas del alumno" });
    expect(within(panel).getByText("Tiene convenio: abona cada quincena")).toBeInTheDocument();
    expect(students.listNotes).toHaveBeenCalledWith(expect.any(String), "s1");
    expect(screen.getByRole("dialog").className).toContain("drawer-xl");
  });

  it("offers 'Agregar nota' when the pupil has none, and widens to write it", async () => {
    const user = userEvent.setup();
    students.listNotes.mockResolvedValue([]);
    render(<CollectDrawer student={student} notes={cashier} onClose={() => {}} onCollected={() => {}} />);
    const add = await screen.findByRole("button", { name: "Agregar nota" });
    expect(screen.getByRole("dialog").className).toContain("drawer-lg");
    await user.click(add);
    expect(screen.getByRole("dialog").className).toContain("drawer-xl");
    await waitFor(() => expect(screen.getByLabelText("Nueva nota")).toHaveFocus());
  });

  it("asks for no notes without access to them", async () => {
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} />);
    await screen.findByTestId("amount-expected");
    expect(students.listNotes).not.toHaveBeenCalled();
    expect(screen.queryByRole("button", { name: "Agregar nota" })).not.toBeInTheDocument();
  });
});
