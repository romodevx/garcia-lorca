// Regression: ISSUE-001 — Ingresos del periodo did not refresh after Registrar pago
// Found by /qa on 2026-10-01
// Report: .gstack/qa-reports/t8/qa-report-localhost-2026-10-01.md
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import * as financeApi from "../../finance/api";
import * as configApi from "../../config/api";
import * as studentsApi from "../../students/api";
import { FinanceScreen } from "./FinanceScreen";

vi.mock("../../finance/api", () => ({
  listCharges: vi.fn(),
  getOverdue: vi.fn(),
  registerPayment: vi.fn(),
  getAccount: vi.fn(),
  previewDeposit: vi.fn(),
  registerDeposit: vi.fn(),
  getCashUp: vi.fn(),
  getIncome: vi.fn(),
}));
vi.mock("../../config/api", () => ({ listCycles: vi.fn() }));
vi.mock("../../students/api", () => ({ listStudents: vi.fn(), listNotes: vi.fn(), addNote: vi.fn(), editNote: vi.fn(), voidNote: vi.fn(), StudentsError: class extends Error {} }));

const finance = vi.mocked(financeApi);

beforeEach(() => {
  vi.mocked(configApi).listCycles.mockResolvedValue([{ id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" }]);
  vi.mocked(studentsApi).listStudents.mockResolvedValue([{
    id: "s1", curp: "Cs1", firstName: "Valentina", lastName: "Cruz", level: "primaria", levelId: "", gradeGroup: "1° A", guardianName: "Tutor", admissionStatus: "enrolled",
  }]);
  finance.listCharges.mockResolvedValue([{
    id: "ch1", studentId: "s1", cycleId: "c1", conceptId: "k1", description: "Colegiatura", amountCents: 150000,
    discountCents: 0, dueDate: "2026-09-05", status: "pending", paidCents: 0, remainingCents: 150000,
  }]);
  finance.getOverdue.mockResolvedValue([]);
  finance.getIncome.mockResolvedValue({ lines: [], totalCents: 0, creditNoteCents: 0 });
  finance.registerPayment.mockResolvedValue({} as financeApi.PaymentResult);
});
afterEach(() => vi.clearAllMocks());

it("reads the period's income again after a payment is registered", async () => {
  const user = userEvent.setup();
  render(<FinanceScreen onBack={() => {}} />);
  await screen.findByText("Cruz Valentina");
  await waitFor(() => expect(finance.getIncome).toHaveBeenCalledTimes(1));

  await user.click(screen.getAllByRole("button", { name: /Registrar pago/ })[0] as HTMLElement);
  const dialog = await screen.findByRole("dialog", { name: "Registrar pago" });
  await user.click(within(dialog).getByRole("button", { name: /Registrar pago/ }));

  await waitFor(() => expect(finance.registerPayment).toHaveBeenCalled());
  await waitFor(() => expect(finance.getIncome).toHaveBeenCalledTimes(2));
});
