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
  getIncome: vi.fn(),
  listConcepts: vi.fn(),
  createConcept: vi.fn(),
  updateConcept: vi.fn(),
  createCharge: vi.fn(),
  applyDiscount: vi.fn(),
  cancelCharge: vi.fn(),
}));
vi.mock("../../config/api", () => ({ listCycles: vi.fn(), listLabelledGroups: vi.fn() }));
vi.mock("../../students/api", () => ({ listStudents: vi.fn(), listEnrollments: vi.fn() }));

const finance = vi.mocked(financeApi);
const config = vi.mocked(configApi);
const students = vi.mocked(studentsApi);

const CHARGE: financeApi.Charge = {
  id: "ch1", studentId: "s1", cycleId: "c1", conceptId: "k1", description: "Colegiatura", amountCents: 150000,
  discountCents: 0, dueDate: "2026-09-05", status: "pending", paidCents: 0, remainingCents: 150000,
};

beforeEach(() => {
  config.listCycles.mockResolvedValue([{ id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" }]);
  config.listLabelledGroups.mockResolvedValue([]);
  students.listStudents.mockResolvedValue([{ id: "s1", curp: "C1", firstName: "Valentina", lastName: "Cruz", level: "primaria", levelId: "", gradeGroup: "3° A", guardianName: "T", admissionStatus: "enrolled" }]);
  students.listEnrollments.mockResolvedValue([]);
  finance.listCharges.mockResolvedValue([CHARGE]);
  finance.getOverdue.mockResolvedValue([]);
  finance.getIncome.mockResolvedValue({ lines: [], totalCents: 0, creditNoteCents: 0 });
  finance.listConcepts.mockResolvedValue([{ id: "k9", name: "CREDENCIAL", defaultAmountCents: 8000, recurrence: "once", active: true }]);
});
afterEach(() => vi.clearAllMocks());

describe("FinanceScreen · ajustes de cargos (CC-203)", () => {
  it("hides every write action from whoever lacks finance:write", async () => {
    render(<FinanceScreen onBack={vi.fn()} today="2026-10-17" />);

    await screen.findByText("Colegiatura");
    expect(screen.queryByRole("button", { name: "Nuevo cargo" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Ajustar cargo/ })).not.toBeInTheDocument();
  });

  it("cancels a charge from its row: the row turns Cancelado and the screen says so", async () => {
    const user = userEvent.setup();
    finance.cancelCharge.mockResolvedValue({ ...CHARGE, status: "cancelled", remainingCents: 0 });
    render(<FinanceScreen onBack={vi.fn()} today="2026-10-17" canManageCharges />);

    const adjust = await screen.findByRole("button", { name: /^Ajustar cargo de/ });
    expect(adjust).toHaveAttribute("aria-expanded", "false");
    await user.click(adjust);
    const panel = screen.getByRole("form", { name: /Ajustar cargo de/ });
    await user.click(within(panel).getByRole("radio", { name: "Cancelar cargo" }));
    await user.type(within(panel).getByLabelText(/^Motivo/), "Cargo duplicado");
    await user.click(within(panel).getByRole("button", { name: "Cancelar este cargo" }));

    await waitFor(() => expect(finance.cancelCharge).toHaveBeenCalledWith(expect.any(String), "ch1", "Cargo duplicado"));
    expect(await screen.findByRole("status")).toHaveTextContent(/cancelado/);
    expect(screen.queryByRole("form", { name: /Ajustar cargo de/ })).not.toBeInTheDocument();
    expect(screen.getAllByText("Cancelado").length).toBeGreaterThan(0);
  });

  it("creates a manual charge and lists it at once", async () => {
    const user = userEvent.setup();
    finance.createCharge.mockResolvedValue({ ...CHARGE, id: "ch9", conceptId: "k9", description: "CREDENCIAL", amountCents: 8000, remainingCents: 8000, dueDate: "2026-10-17" });
    render(<FinanceScreen onBack={vi.fn()} today="2026-10-17" canManageCharges />);

    await user.click(await screen.findByRole("button", { name: "Nuevo cargo" }));
    const form = screen.getByRole("form", { name: "Nuevo cargo" });
    const pupil = within(form).getByLabelText(/^Alumno/);
    const option = form.querySelector("datalist option");
    await user.type(pupil, option?.getAttribute("value") ?? "");
    await user.selectOptions(await within(form).findByLabelText(/^Concepto/), "k9");
    await user.click(within(form).getByRole("button", { name: "Crear cargo" }));

    await waitFor(() => expect(finance.createCharge).toHaveBeenCalledWith(expect.any(String), expect.objectContaining({ studentId: "s1", cycleId: "c1", conceptId: "k9", amountCentsOverride: 8000 })));
    expect(await screen.findByText("CREDENCIAL")).toBeInTheDocument();
    expect(screen.queryByRole("form", { name: "Nuevo cargo" })).not.toBeInTheDocument();
  });
});

describe("FinanceScreen · conceptos (CC-111)", () => {
  it("keeps the catalog collapsed until asked, then reads it", async () => {
    const user = userEvent.setup();
    render(<FinanceScreen onBack={vi.fn()} today="2026-10-17" canManageCharges />);

    const toggle = await screen.findByRole("button", { name: "Conceptos de cobro" });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(finance.listConcepts).not.toHaveBeenCalled();
    await user.click(toggle);

    expect(await screen.findByRole("table", { name: "Conceptos de cobro" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Agregar concepto" })).toBeInTheDocument();
  });
});
