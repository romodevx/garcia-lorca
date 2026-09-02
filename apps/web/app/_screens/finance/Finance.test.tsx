import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../config/api";
import * as financeApi from "../../finance/api";
import * as studentsApi from "../../students/api";
import { Providers } from "../../test-utils";
import { Finance } from "./Finance";

// The surface talks to the gateway only through the finance/config/students api
// clients, so we mock them wholesale and assert the calls (the fetch itself is
// covered by app/finance/api.test.ts). Neutral literals only.
vi.mock("../../finance/api", () => ({
  listConcepts: vi.fn(),
  createConcept: vi.fn(),
  updateConcept: vi.fn(),
  listCharges: vi.fn(),
  generateCharges: vi.fn(),
  getOverdue: vi.fn(),
  registerPayment: vi.fn(),
  getAccount: vi.fn(),
  applyDiscount: vi.fn(),
  cancelCharge: vi.fn(),
}));

vi.mock("../../config/api", () => ({
  listCycles: vi.fn(),
  listGroups: vi.fn(),
}));

vi.mock("../../students/api", () => ({
  listStudents: vi.fn(),
}));

const finance = vi.mocked(financeApi);
const config = vi.mocked(configApi);
const students = vi.mocked(studentsApi);

const cycle = { id: "cy1", name: "2026-2027", status: "active" as const, startsOn: "2026-08-01", endsOn: "2027-06-30" };
const group = { id: "g1", cycleId: "cy1", gradeLevelId: "gl1", name: "4° A", capacity: 30 };

const student = {
  id: "st1",
  curp: "AAAA000000AAAAAA01",
  firstName: "Ada",
  lastName: "Lovelace",
  level: "Primaria",
  gradeGroup: "4° A",
  guardianName: "Anne Byron",
  admissionStatus: "enrolled" as const,
};

const concept = { id: "c1", name: "Colegiatura", defaultAmountCents: 185000, recurrence: "monthly" as const, active: true };

const overdueCharge = {
  id: "ch1",
  studentId: "st1",
  cycleId: "cy1",
  conceptId: "c1",
  description: "Colegiatura septiembre",
  amountCents: 185000,
  discountCents: 0,
  dueDate: "2026-09-05",
  status: "pending" as const,
  paidCents: 0,
  remainingCents: 185000,
};

function renderFinance(intent?: "payment"): void {
  render(
    <Providers flags={{ module_finance: true }}>
      <Finance onBack={() => {}} intent={intent} />
    </Providers>,
  );
}

beforeEach(() => {
  config.listCycles.mockResolvedValue([cycle]);
  config.listGroups.mockResolvedValue([group]);
  students.listStudents.mockResolvedValue([student]);
  finance.listConcepts.mockResolvedValue([concept]);
  finance.getOverdue.mockResolvedValue([overdueCharge]);
  finance.listCharges.mockResolvedValue([overdueCharge]);
});

afterEach(() => vi.clearAllMocks());

describe("Finance — adeudos", () => {
  it("lists the overdue charges with the student, the MXN amounts and a status pill", async () => {
    renderFinance();

    expect(await screen.findByText("Colegiatura septiembre")).toBeInTheDocument();
    expect(finance.getOverdue).toHaveBeenCalledWith(expect.any(String), "cy1");
    // Money is rendered in pesos (MXN), never raw cents.
    expect(screen.getAllByText("$1,850.00").length).toBeGreaterThan(0);
    expect(screen.queryByText("185000")).not.toBeInTheDocument();
    // The student id is resolved to a readable name in the row (not just in the filter).
    const nameCells = screen.getAllByText("Ada Lovelace").filter((element) => element.tagName !== "OPTION");
    expect(nameCells).toHaveLength(1);
    expect(screen.getByText("Pendiente")).toBeInTheDocument();
  });

  it("re-reads the charges with the chosen student/cycle filters on the 'Todos' tab", async () => {
    const user = userEvent.setup();
    renderFinance();

    const filters = await screen.findByRole("tablist", { name: "Filtrar por estado" });
    await user.click(within(filters).getByRole("tab", { name: "Todos" }));
    await user.selectOptions(screen.getByLabelText("Alumno"), "st1");

    await waitFor(() =>
      expect(finance.listCharges).toHaveBeenCalledWith(expect.any(String), { studentId: "st1", cycleId: "cy1" }),
    );
  });

  // The gateway refuses an unscoped charge read (it would be the whole tenant
  // ledger), so the surface must ask for a scope instead of firing a request it
  // knows will bounce — and never render the bounce as "no debts".
  it("asks for a student or cycle instead of reading the whole ledger unscoped", async () => {
    const user = userEvent.setup();
    config.listCycles.mockResolvedValue([]);
    renderFinance();

    const filters = await screen.findByRole("tablist", { name: "Filtrar por estado" });
    await user.click(within(filters).getByRole("tab", { name: "Todos" }));

    expect(await screen.findByText("Elige un alumno o un ciclo para ver los cargos.")).toBeInTheDocument();
    expect(finance.listCharges).not.toHaveBeenCalled();
  });
});

describe("Finance — a cancelled charge", () => {
  const cancelledCharge = { ...overdueCharge, id: "ch9", description: "Cargo anulado", status: "cancelled" as const, remainingCents: 0 };

  it("labels it from its own status — never 'Pagado', which it was not", async () => {
    finance.getOverdue.mockResolvedValue([cancelledCharge]);
    renderFinance();

    expect(await screen.findByText("Cargo anulado")).toBeInTheDocument();
    // "Cancelado" is the pill AND the row label; "Pagado" must appear nowhere in
    // the row (the column HEADER "Pagado" is the only legitimate occurrence).
    const labels = screen.getAllByText("Pagado").filter((element) => element.tagName !== "OPTION");
    expect(labels).toHaveLength(1);
    expect(labels[0]).toHaveTextContent("Pagado");
    expect(screen.getAllByText("Cancelado").length).toBe(2);
  });

  it("offers no money actions on it (nothing left to collect, discount or void)", async () => {
    finance.getOverdue.mockResolvedValue([cancelledCharge]);
    renderFinance();

    await screen.findByText("Cargo anulado");
    expect(screen.queryByRole("button", { name: /Registrar pago de/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Más acciones de/ })).not.toBeInTheDocument();
  });
});

describe("Finance — descuento y cancelación", () => {
  /** Opens the secondary-actions disclosure of the single overdue row. */
  async function openActions(user: ReturnType<typeof userEvent.setup>): Promise<void> {
    renderFinance();
    await user.click(await screen.findByRole("button", { name: "Más acciones de Colegiatura septiembre" }));
  }

  it("applies a discount as INTEGER cents with its reason, and updates the row", async () => {
    const user = userEvent.setup();
    finance.applyDiscount.mockResolvedValue({ ...overdueCharge, discountCents: 25050, remainingCents: 159950 });

    await openActions(user);
    await user.type(screen.getByLabelText("Descuento (pesos)"), "250.50");
    await user.type(screen.getByLabelText("Motivo del descuento"), "beca parcial");
    await user.click(screen.getByRole("button", { name: "Aplicar descuento" }));

    await waitFor(() => expect(finance.applyDiscount).toHaveBeenCalledWith(expect.any(String), "ch1", 25050, "beca parcial"));
    const [, , discountCents] = finance.applyDiscount.mock.calls[0]!;
    expect(Number.isInteger(discountCents)).toBe(true);
    // The gateway's recomputed charge replaces the row, and the toast confirms.
    expect(await screen.findByRole("status")).toHaveTextContent("Descuento aplicado · $250.50");
    // The new balance shows in the row AND in the recomputed KPI card.
    expect(screen.getAllByText("$1,599.50")).toHaveLength(2);
  });

  it("refuses a discount whose amount is not unambiguously pesos (no NaN reaches the gateway)", async () => {
    const user = userEvent.setup();

    await openActions(user);
    // A decimal COMMA: silently coerced this would forgive 100x the intended amount.
    await user.type(screen.getByLabelText("Descuento (pesos)"), "250,50");
    await user.type(screen.getByLabelText("Motivo del descuento"), "beca parcial");
    await user.click(screen.getByRole("button", { name: "Aplicar descuento" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Captura un monto mayor a cero.");
    expect(finance.applyDiscount).not.toHaveBeenCalled();
  });

  it("requires a reason for the discount (it is the gateway's audit trail)", async () => {
    const user = userEvent.setup();

    await openActions(user);
    await user.type(screen.getByLabelText("Descuento (pesos)"), "250.50");
    await user.click(screen.getByRole("button", { name: "Aplicar descuento" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Captura el motivo del descuento.");
    expect(finance.applyDiscount).not.toHaveBeenCalled();
  });

  it("cancels a charge only after an explicit confirmation, then shows it cancelled", async () => {
    const user = userEvent.setup();
    finance.cancelCharge.mockResolvedValue({ ...overdueCharge, status: "cancelled" as const, remainingCents: 0 });

    await openActions(user);

    // First click arms the action; nothing is sent yet.
    await user.click(screen.getByRole("button", { name: "Cancelar cargo" }));
    expect(finance.cancelCharge).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: "Confirmar cancelación" }));

    await waitFor(() => expect(finance.cancelCharge).toHaveBeenCalledWith(expect.any(String), "ch1"));
    expect(await screen.findByRole("status")).toHaveTextContent("Cargo cancelado");
    // The row is now cancelled (pill + label) and offers no money actions.
    await waitFor(() => expect(screen.getAllByText("Cancelado")).toHaveLength(2));
    expect(screen.queryByRole("button", { name: /Registrar pago de/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Más acciones de/ })).not.toBeInTheDocument();
  });

  it("surfaces the gateway's refusal next to the form instead of failing silently", async () => {
    const user = userEvent.setup();
    finance.applyDiscount.mockRejectedValue(new Error("discount exceeds charge amount"));

    await openActions(user);
    await user.type(screen.getByLabelText("Descuento (pesos)"), "999999");
    await user.type(screen.getByLabelText("Motivo del descuento"), "error de captura");
    await user.click(screen.getByRole("button", { name: "Aplicar descuento" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("discount exceeds charge amount");
  });
});

describe("Finance — ventanilla", () => {
  it("prefills the remaining balance, posts integer cents and confirms with a toast", async () => {
    const user = userEvent.setup();
    finance.registerPayment.mockResolvedValue({
      payment: { id: "p1", chargeId: "ch1", amountCents: 185000, method: "cash", reference: "", paidAt: "2026-09-01T10:00:00Z" },
      charge: { ...overdueCharge, paidCents: 185000, remainingCents: 0, status: "paid" as const },
    });

    renderFinance();

    // One click from the row to the window form, with the charge preselected.
    await user.click(await screen.findByRole("button", { name: "Registrar pago de Colegiatura septiembre" }));

    const amount = screen.getByLabelText("Monto (pesos)");
    expect(amount).toHaveValue("1850.00");

    await user.click(screen.getByRole("button", { name: "Registrar pago" }));

    await waitFor(() =>
      expect(finance.registerPayment).toHaveBeenCalledWith(expect.any(String), {
        chargeId: "ch1",
        amountCents: 185000,
        method: "cash",
        reference: "",
        idempotencyKey: expect.any(String),
      }),
    );
    expect(await screen.findByRole("status")).toHaveTextContent("Pago registrado");
  });

  it("sends the chosen method and reference, converting pesos to cents", async () => {
    const user = userEvent.setup();
    finance.registerPayment.mockResolvedValue({
      payment: { id: "p1", chargeId: "ch1", amountCents: 50025, method: "transfer" as const, reference: "SPEI-77", paidAt: "2026-09-01T10:00:00Z" },
      charge: { ...overdueCharge, paidCents: 50025, remainingCents: 134975, status: "partial" as const },
    });

    renderFinance();

    await user.click(await screen.findByRole("button", { name: "Registrar pago de Colegiatura septiembre" }));

    const amount = screen.getByLabelText("Monto (pesos)");
    await user.clear(amount);
    await user.type(amount, "500.25");
    await user.click(screen.getByLabelText("Transferencia"));
    await user.type(screen.getByLabelText("Referencia (opcional)"), "SPEI-77");
    await user.click(screen.getByRole("button", { name: "Registrar pago" }));

    await waitFor(() =>
      expect(finance.registerPayment).toHaveBeenCalledWith(expect.any(String), {
        chargeId: "ch1",
        amountCents: 50025,
        method: "transfer",
        reference: "SPEI-77",
        idempotencyKey: expect.any(String),
      }),
    );
  });

  it("surfaces the gateway's rejection (e.g. an overpay) instead of a silent failure", async () => {
    const user = userEvent.setup();
    finance.registerPayment.mockRejectedValue(new Error("payment exceeds remaining balance"));

    renderFinance();

    await user.click(await screen.findByRole("button", { name: "Registrar pago de Colegiatura septiembre" }));
    await user.click(screen.getByRole("button", { name: "Registrar pago" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("payment exceeds remaining balance");
  });

  it("retries a failed submit under the SAME idempotency key, and mints a fresh one after a success", async () => {
    const user = userEvent.setup();
    const keyOf = (nth: number): string | undefined => finance.registerPayment.mock.calls[nth]?.[1].idempotencyKey;
    // A PARTIAL collection, so the charge stays on the window form and a third
    // submit is possible.
    finance.registerPayment.mockRejectedValueOnce(new Error("gateway unreachable")).mockResolvedValue({
      payment: { id: "p1", chargeId: "ch1", amountCents: 50000, method: "cash", reference: "", paidAt: "2026-09-01T10:00:00Z" },
      charge: { ...overdueCharge, paidCents: 50000, remainingCents: 135000, status: "partial" as const },
    });

    renderFinance();

    await user.click(await screen.findByRole("button", { name: "Registrar pago de Colegiatura septiembre" }));
    await user.click(screen.getByRole("button", { name: "Registrar pago" }));
    expect(await screen.findByRole("alert")).toBeInTheDocument();

    // The retry is the SAME attempt: reusing the key is what lets the server
    // recognise it and refuse to collect twice.
    await user.click(screen.getByRole("button", { name: "Registrar pago" }));
    await waitFor(() => expect(finance.registerPayment).toHaveBeenCalledTimes(2));
    expect(keyOf(1)).toBe(keyOf(0));
    expect(keyOf(1)).toEqual(expect.any(String));

    // That payment is banked, so the next collection is a NEW attempt and must
    // not be deduplicated against it.
    await user.click(screen.getByRole("button", { name: "Registrar pago" }));
    await waitFor(() => expect(finance.registerPayment).toHaveBeenCalledTimes(3));
    expect(keyOf(2)).not.toBe(keyOf(1));
  });

  it("opens directly on the window form when the palette asks for the payment intent", async () => {
    renderFinance("payment");

    const tabs = await screen.findByRole("tablist", { name: "Secciones de finanzas" });
    expect(within(tabs).getByRole("tab", { name: "Registrar pago" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByLabelText("Cargo")).toBeInTheDocument();
  });
});

describe("Finance — conceptos", () => {
  it("creates a concept, converting the peso amount to integer cents", async () => {
    const user = userEvent.setup();
    finance.createConcept.mockResolvedValue({ id: "c2", name: "Inscripción", defaultAmountCents: 185050, recurrence: "once", active: true });

    renderFinance();

    const tabs = await screen.findByRole("tablist", { name: "Secciones de finanzas" });
    await user.click(within(tabs).getByRole("tab", { name: "Conceptos" }));

    await user.type(screen.getByLabelText("Nombre del concepto"), "Inscripción");
    await user.type(screen.getByLabelText("Monto (pesos)"), "1850.50");
    await user.click(screen.getByLabelText("Única"));
    await user.click(screen.getByRole("button", { name: "Crear concepto" }));

    await waitFor(() =>
      expect(finance.createConcept).toHaveBeenCalledWith(expect.any(String), {
        name: "Inscripción",
        defaultAmountCents: 185050,
        recurrence: "once",
      }),
    );
  });

  it("re-reads the list including the inactive concepts when asked", async () => {
    const user = userEvent.setup();
    renderFinance();

    const tabs = await screen.findByRole("tablist", { name: "Secciones de finanzas" });
    await user.click(within(tabs).getByRole("tab", { name: "Conceptos" }));
    await user.click(screen.getByLabelText("Incluir inactivos"));

    await waitFor(() => expect(finance.listConcepts).toHaveBeenCalledWith(expect.any(String), true));
  });

  it("edits a concept in place (name, amount, recurrence, active)", async () => {
    const user = userEvent.setup();
    finance.updateConcept.mockResolvedValue({ ...concept, defaultAmountCents: 190000, active: false });

    renderFinance();

    const tabs = await screen.findByRole("tablist", { name: "Secciones de finanzas" });
    await user.click(within(tabs).getByRole("tab", { name: "Conceptos" }));
    await user.click(screen.getByRole("button", { name: "Editar Colegiatura" }));

    // Editing reuses the single concept form (no duplicate amount field on screen).
    const amount = screen.getByLabelText("Monto (pesos)");
    await user.clear(amount);
    await user.type(amount, "1900");
    await user.click(screen.getByLabelText("Activo"));
    await user.click(screen.getByRole("button", { name: "Guardar concepto" }));

    await waitFor(() =>
      expect(finance.updateConcept).toHaveBeenCalledWith(expect.any(String), "c1", {
        name: "Colegiatura",
        defaultAmountCents: 190000,
        recurrence: "monthly",
        active: false,
      }),
    );
  });
});

describe("Finance — generar cargos", () => {
  it("bulk-generates for a concept·cycle·group and reports created/skipped", async () => {
    const user = userEvent.setup();
    finance.generateCharges.mockResolvedValue({ created: 24, skipped: 3 });

    renderFinance();

    const tabs = await screen.findByRole("tablist", { name: "Secciones de finanzas" });
    await user.click(within(tabs).getByRole("tab", { name: "Generar cargos" }));

    await user.selectOptions(screen.getByLabelText("Concepto"), "c1");
    await user.selectOptions(await screen.findByLabelText("Grupo"), "g1");
    fireEvent.change(screen.getByLabelText("Fecha de vencimiento"), { target: { value: "2026-09-05" } });
    await user.click(screen.getByRole("button", { name: "Generar cargos" }));

    await waitFor(() =>
      expect(finance.generateCharges).toHaveBeenCalledWith(expect.any(String), {
        conceptId: "c1",
        cycleId: "cy1",
        groupId: "g1",
        amountCentsOverride: 0,
        dueDate: "2026-09-05",
      }),
    );
    expect(await screen.findByRole("status")).toHaveTextContent("24 cargos creados · 3 omitidos");
  });
});

describe("Finance — estado de cuenta", () => {
  it("loads a student's statement with its charges, payments and total balance", async () => {
    const user = userEvent.setup();
    finance.getAccount.mockResolvedValue({
      charges: [overdueCharge],
      payments: [{ id: "p1", chargeId: "ch1", amountCents: 50000, method: "cash" as const, reference: "", paidAt: "2026-09-01T10:00:00Z" }],
      totalBalanceCents: 135000,
      unallocatedCents: 0,
    });

    renderFinance();

    const tabs = await screen.findByRole("tablist", { name: "Secciones de finanzas" });
    await user.click(within(tabs).getByRole("tab", { name: "Estado de cuenta" }));

    await user.selectOptions(screen.getByLabelText("Alumno"), "st1");
    await user.click(screen.getByRole("button", { name: "Ver estado de cuenta" }));

    await waitFor(() => expect(finance.getAccount).toHaveBeenCalledWith(expect.any(String), "st1"));
    expect(await screen.findByText("$1,350.00")).toBeInTheDocument();
    expect(screen.getByText("$500.00")).toBeInTheDocument();
  });
});

describe("Finance — load failures", () => {
  it("shows a single readable error when the shared boot reads fail", async () => {
    config.listCycles.mockRejectedValue(new Error("HTTP 503"));
    renderFinance();
    expect(await screen.findByRole("alert")).toHaveTextContent("No se pudieron cargar las finanzas.");
  });

  it("keeps the surface usable and surfaces the detail when only the charge read fails", async () => {
    finance.getOverdue.mockRejectedValue(new Error("HTTP 503"));
    renderFinance();
    expect(await screen.findByRole("alert")).toHaveTextContent("No se pudieron cargar los cargos: HTTP 503");
    expect(screen.getByRole("tablist", { name: "Secciones de finanzas" })).toBeInTheDocument();
  });

  it("keeps the debts table intact when only the concept read fails", async () => {
    // Regression: a transient concepts read used to set the SHARED load error,
    // which replaced the whole surface — wiping a debts table that was fine.
    finance.listConcepts.mockRejectedValue(new Error("HTTP 503"));
    renderFinance();

    expect(await screen.findByText("Colegiatura septiembre")).toBeInTheDocument();
    expect(screen.queryByText("No se pudieron cargar las finanzas.")).not.toBeInTheDocument();
  });

  it("reports a failed concept read inside the concepts panel", async () => {
    const user = userEvent.setup();
    finance.listConcepts.mockRejectedValue(new Error("HTTP 503"));
    renderFinance();

    const tabs = await screen.findByRole("tablist", { name: "Secciones de finanzas" });
    await user.click(within(tabs).getByRole("tab", { name: "Conceptos" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("No se pudieron cargar los conceptos: HTTP 503");
  });
});
