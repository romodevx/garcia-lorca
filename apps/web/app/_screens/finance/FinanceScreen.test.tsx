import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as financeApi from "../../finance/api";
import * as configApi from "../../config/api";
import * as studentsApi from "../../students/api";
import { FinanceScreen } from "./FinanceScreen";

vi.mock("../../finance/api", () => ({
  listCharges: vi.fn(),
  getOverdue: vi.fn(),
  registerPayment: vi.fn(),
}));
vi.mock("../../config/api", () => ({ listCycles: vi.fn() }));
vi.mock("../../students/api", () => ({ listStudents: vi.fn() }));

const finance = vi.mocked(financeApi);
const config = vi.mocked(configApi);
const students = vi.mocked(studentsApi);

const charge = (id: string, studentId: string, remaining: number): financeApi.Charge => ({
  id,
  studentId,
  cycleId: "c1",
  conceptId: "k1",
  description: "Colegiatura",
  amountCents: 150000,
  discountCents: 0,
  dueDate: "2026-09-05",
  status: "pending",
  paidCents: 150000 - remaining,
  remainingCents: remaining,
});
const student = (id: string, first: string, last: string): studentsApi.Student => ({
  id, curp: `C${id}`, firstName: first, lastName: last, level: "primaria", gradeGroup: "1° A", guardianName: "Tutor", admissionStatus: "enrolled",
});

beforeEach(() => {
  config.listCycles.mockResolvedValue([{ id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" }]);
  students.listStudents.mockResolvedValue([student("s1", "Valentina", "Cruz"), student("s2", "Mateo", "Sánchez")]);
  finance.listCharges.mockResolvedValue([charge("ch1", "s1", 100000), charge("ch2", "s2", 0)]);
  finance.getOverdue.mockResolvedValue([]);
});
afterEach(() => vi.clearAllMocks());

describe("FinanceScreen", () => {
  it("resolves student names and shows charge status", async () => {
    render(<FinanceScreen onBack={() => {}} />);
    expect(await screen.findByRole("heading", { name: "Finanzas y Adeudos" })).toBeInTheDocument();
    expect(await screen.findByText("Valentina Cruz")).toBeInTheDocument();
    expect(screen.getByText("Mateo Sánchez")).toBeInTheDocument();
    // ch2 is fully paid → "Al corriente"; ch1 has a balance → a "Registrar pago" action
    expect(screen.getByText("Al corriente")).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: /Registrar pago/ }).length).toBeGreaterThan(0);
  });

  it("registers a payment for a charge from the drawer", async () => {
    const user = userEvent.setup();
    finance.registerPayment.mockResolvedValue({} as financeApi.PaymentResult);
    render(<FinanceScreen onBack={() => {}} />);
    await screen.findByText("Valentina Cruz");

    await user.click(screen.getAllByRole("button", { name: /Registrar pago/ })[0] as HTMLElement);
    const dialog = await screen.findByRole("dialog", { name: "Registrar pago" });
    await user.clear(within(dialog).getByLabelText("Monto"));
    await user.type(within(dialog).getByLabelText("Monto"), "500");
    await user.click(within(dialog).getByRole("button", { name: /Registrar pago/ }));

    await waitFor(() =>
      expect(finance.registerPayment).toHaveBeenCalledWith(expect.any(String), expect.objectContaining({ chargeId: "ch1", amountCents: 50000, method: "cash" })),
    );
  });
});
