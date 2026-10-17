import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as financeApi from "../../finance/api";
import * as configApi from "../../config/api";
import * as studentsApi from "../../students/api";
import { FinanceScreen } from "./FinanceScreen";

vi.mock("../../finance/api", () => ({
  listCharges: vi.fn(),
  getOverdue: vi.fn(),
  getExpected: vi.fn(),
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
const config = vi.mocked(configApi);
const students = vi.mocked(studentsApi);

beforeEach(() => {
  vi.mocked(financeApi).getExpected.mockResolvedValue({ amountCents: 0, surchargeCents: 0, discountCents: 0, annualBalanceCents: 0, onTimeBalanceCents: 0, lines: [] });
  // 20:00 on 31 October in Morelia = 02:00 UTC on 1 November (ADR 0035).
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date(Date.UTC(2026, 10, 1, 2, 0)));
  config.listCycles.mockResolvedValue([{ id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" }]);
  students.listStudents.mockResolvedValue([]);
  finance.listCharges.mockResolvedValue([]);
  finance.getOverdue.mockResolvedValue([]);
  finance.getIncome.mockResolvedValue({ lines: [], totalCents: 0, creditNoteCents: 0 });
});
afterEach(() => {
  vi.useRealTimers();
  vi.clearAllMocks();
});

describe("FinanceScreen · the school's day", () => {
  it("asks for the month's income up to the school's today, not UTC's", async () => {
    render(<FinanceScreen onBack={() => {}} />);
    expect(await screen.findByRole("heading", { name: "Finanzas" })).toBeInTheDocument();
    await waitFor(() => expect(finance.getIncome).toHaveBeenCalled());
    expect(finance.getIncome).toHaveBeenLastCalledWith(expect.anything(), "2026-10-01", "2026-10-31");
  });
});
