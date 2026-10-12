import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as api from "../../expenses/api";
import { clearIntent, requestIntent } from "../../routing/pendingIntent";
import { ExpensesScreen } from "./ExpensesScreen";

vi.mock("../../expenses/api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../../expenses/api")>();
  return {
    ...real,
    listCategories: vi.fn(),
    createCategory: vi.fn(),
    updateCategory: vi.fn(),
    listExpenses: vi.fn(),
    createExpense: vi.fn(),
    updateExpense: vi.fn(),
    cancelExpense: vi.fn(),
    getExpense: vi.fn(),
    uploadReceipt: vi.fn(),
    getReceiptBlob: vi.fn(),
    deleteReceipt: vi.fn(),
    getSummary: vi.fn(),
    getReport: vi.fn(),
    downloadReport: vi.fn(),
  };
});
const m = vi.mocked(api);

const CAT = "c1000000-0000-0000-0000-000000000001";
const EXP = "e1000000-0000-0000-0000-000000000001";
const zero = { cashCents: 0, cardCents: 0, transferCents: 0, totalCents: 0 };
const writer = ["expenses:read", "expenses:write"];

const toner: api.Expense = {
  id: EXP, spentOn: "2026-10-01", categoryId: CAT, categoryName: "Papelería", amountCents: 125050,
  method: "cash", description: "Tóner", status: "active", createdByName: "Laura Caja",
};

beforeEach(() => {
  m.listCategories.mockResolvedValue([{ id: CAT, name: "Papelería", active: true }]);
  m.listExpenses.mockResolvedValue([toner, { ...toner, id: "e2", description: "Doble", status: "cancelled", cancelReason: "duplicado" }]);
  m.getSummary.mockResolvedValue({
    income: { ...zero, cashCents: 500000, totalCents: 500000 },
    creditNoteCents: 20000,
    expenses: { ...zero, cashCents: 125050, totalCents: 125050 },
    balance: { ...zero, cashCents: 374950, totalCents: 374950 },
    expensesByCategory: [{ categoryId: CAT, categoryName: "Papelería", amountCents: 125050, count: 1 }],
  });
  m.getExpense.mockResolvedValue({ expense: toner, events: [{ id: "v1", kind: "created", before: {}, after: { amountCents: 125050 }, actorName: "Laura Caja", at: "2026-10-01T15:00:00Z" }] });
});
afterEach(() => vi.clearAllMocks());

function renderScreen(permissions: string[] = writer): void {
  render(<ExpensesScreen onBack={() => undefined} permissions={permissions} today="2026-10-15" />);
}

describe("ExpensesScreen (T10)", () => {
  it("leads with the month's income, expenses and balance", async () => {
    renderScreen();
    expect(await screen.findByTestId("stat-income")).toHaveTextContent("$5,000.00");
    expect(screen.getByTestId("stat-expenses")).toHaveTextContent("$1,250.50");
    expect(screen.getByTestId("stat-balance")).toHaveTextContent("$3,749.50");
    // Credit notes are not income: on a card of their own, said so.
    expect(screen.getByTestId("stat-credit-notes")).toHaveTextContent(/no es ingreso/i);
    expect(m.getSummary).toHaveBeenCalledWith(expect.any(String), "2026-10-01", "2026-10-31");
  });

  it("lists the month's expenses, and the cancelled ones on their own tab", async () => {
    const user = userEvent.setup();
    renderScreen();
    const rows = await screen.findAllByTestId("expense-row");
    expect(rows).toHaveLength(1);
    expect(rows[0]).toHaveTextContent("Tóner");
    await user.click(screen.getByRole("tab", { name: /Canceladas/ }));
    expect(screen.getAllByTestId("expense-row")[0]).toHaveTextContent("duplicado");
  });

  it("records an expense from the quick-add drawer, defaulting to today and cash", async () => {
    m.createExpense.mockResolvedValue(toner);
    const user = userEvent.setup();
    renderScreen();
    await user.click(await screen.findByRole("button", { name: /Registrar egreso/ }));
    const dialog = await screen.findByRole("dialog");
    await user.selectOptions(within(dialog).getByRole("combobox", { name: /Categoría/ }), CAT);
    await user.type(within(dialog).getByRole("textbox", { name: /Monto/ }), "1250.50");
    await user.type(within(dialog).getByRole("textbox", { name: /Descripción/ }), "Tóner");
    await user.click(within(dialog).getByRole("button", { name: "Registrar egreso" }));

    expect(m.createExpense).toHaveBeenCalledWith(expect.any(String), {
      spentOn: "2026-10-15", categoryId: CAT, amountCents: 125050, method: "cash", description: "Tóner",
    });
    expect(await screen.findByRole("status")).toHaveTextContent(/registrado/i);
  });

  it("asks for what is missing instead of sending it", async () => {
    const user = userEvent.setup();
    renderScreen();
    await user.click(await screen.findByRole("button", { name: /Registrar egreso/ }));
    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: "Registrar egreso" }));
    expect(m.createExpense).not.toHaveBeenCalled();
    expect(within(dialog).getAllByText("Falta este dato").length).toBeGreaterThanOrEqual(2);
  });

  it("uploads the receipt chosen with the expense", async () => {
    m.createExpense.mockResolvedValue(toner);
    m.uploadReceipt.mockResolvedValue({ ...toner, receipt: { contentType: "application/pdf", sizeBytes: 4 } });
    const user = userEvent.setup();
    renderScreen();
    await user.click(await screen.findByRole("button", { name: /Registrar egreso/ }));
    const dialog = await screen.findByRole("dialog");
    await user.selectOptions(within(dialog).getByRole("combobox", { name: /Categoría/ }), CAT);
    await user.type(within(dialog).getByRole("textbox", { name: /Monto/ }), "10");
    const file = new File(["%PDF"], "ticket.pdf", { type: "application/pdf" });
    await user.upload(within(dialog).getByLabelText("Comprobante", { selector: "input" }), file);
    await user.click(within(dialog).getByRole("button", { name: "Registrar egreso" }));
    await waitFor(() => expect(m.uploadReceipt).toHaveBeenCalledWith(expect.any(String), EXP, file));
  });

  // Review T10: the expense is saved before its receipt goes up. A failed
  // upload closes the drawer and says so, so a retry cannot record it twice.
  it("a failed receipt upload does not invite a second create", async () => {
    m.createExpense.mockResolvedValue(toner);
    m.uploadReceipt.mockRejectedValue(new Error("el comprobante debe ser una imagen JPG o PNG, o un PDF"));
    const user = userEvent.setup();
    renderScreen();
    await user.click(await screen.findByRole("button", { name: /Registrar egreso/ }));
    const dialog = await screen.findByRole("dialog");
    await user.selectOptions(within(dialog).getByRole("combobox", { name: /Categoría/ }), CAT);
    await user.type(within(dialog).getByRole("textbox", { name: /Monto/ }), "10");
    await user.upload(within(dialog).getByLabelText("Comprobante", { selector: "input" }), new File(["x"], "falso.pdf", { type: "application/pdf" }));
    await user.click(within(dialog).getByRole("button", { name: "Registrar egreso" }));
    expect(await screen.findByRole("status")).toHaveTextContent(/no se subió/);
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(m.createExpense).toHaveBeenCalledTimes(1);
  });

  it("opens an expense with its history, edits it with a reason, and cancels it with one", async () => {
    m.updateExpense.mockResolvedValue({ ...toner, amountCents: 100000 });
    m.cancelExpense.mockResolvedValue({ ...toner, status: "cancelled" });
    const user = userEvent.setup();
    renderScreen();
    await user.click((await screen.findAllByTestId("expense-row"))[0] as HTMLElement);
    const dialog = await screen.findByRole("dialog");
    expect(await within(dialog).findByText(/Registrado por Laura Caja/)).toBeInTheDocument();

    const amount = within(dialog).getByRole("textbox", { name: /Monto/ });
    await user.clear(amount);
    await user.type(amount, "1000");
    await user.type(within(dialog).getByRole("textbox", { name: /Motivo del cambio/ }), "monto mal");
    await user.click(within(dialog).getByRole("button", { name: "Guardar cambios" }));
    expect(m.updateExpense).toHaveBeenCalledWith(expect.any(String), EXP, expect.objectContaining({ amountCents: 100000, reason: "monto mal" }));

    await user.click((await screen.findAllByTestId("expense-row"))[0] as HTMLElement);
    const again = await screen.findByRole("dialog");
    await user.click(within(again).getByRole("button", { name: "Cancelar egreso" }));
    await user.click(within(again).getByRole("button", { name: "Confirmar cancelación" }));
    expect(m.cancelExpense).not.toHaveBeenCalled();
    await user.type(within(again).getByRole("textbox", { name: /Motivo de la cancelación/ }), "duplicado");
    await user.click(within(again).getByRole("button", { name: "Confirmar cancelación" }));
    expect(m.cancelExpense).toHaveBeenCalledWith(expect.any(String), EXP, "duplicado");
  });

  it("a reader sees everything and changes nothing", async () => {
    renderScreen(["expenses:read"]);
    expect(await screen.findAllByTestId("expense-row")).toHaveLength(1);
    expect(screen.queryByRole("button", { name: /Registrar egreso/ })).not.toBeInTheDocument();
  });

  it("manages the categories inline", async () => {
    m.createCategory.mockResolvedValue({ id: "c2", name: "LUZ", active: true });
    m.updateCategory.mockResolvedValue({ id: CAT, name: "Papelería", active: false });
    const user = userEvent.setup();
    renderScreen();
    await user.click(await screen.findByRole("tab", { name: /Categorías/ }));
    await user.click(await screen.findByRole("button", { name: /Nueva categoría/ }));
    await user.type(screen.getByRole("textbox", { name: /Nombre/ }), "Luz{Enter}");
    await waitFor(() => expect(m.createCategory).toHaveBeenCalledWith(expect.any(String), "LUZ"));
    await user.click(screen.getByRole("button", { name: "Desactivar Papelería" }));
    expect(m.updateCategory).toHaveBeenCalledWith(expect.any(String), CAT, { name: "Papelería", active: false });
  });

  it("reports by period and exports to PDF and Excel", async () => {
    m.getReport.mockResolvedValue({
      period: "weekly", from: "2026-10-12", to: "2026-10-18", label: "Semana del 12 al 18 de octubre de 2026",
      summary: {
        income: { ...zero, cardCents: 70000, totalCents: 70000 }, creditNoteCents: 0,
        expenses: { ...zero }, balance: { ...zero, cardCents: 70000, totalCents: 70000 }, expensesByCategory: [],
      },
      incomeByConcept: { lines: [{ conceptId: "k", concept: "Colegiatura", amountCents: 70000 }], totalCents: 70000, creditNoteCents: 0 },
      expenses: [],
    });
    m.downloadReport.mockResolvedValue({ blob: new Blob(["PK"]), filename: "reporte.xlsx" });
    const createObjectURL = vi.fn(() => "blob:x");
    vi.stubGlobal("URL", Object.assign(URL, { createObjectURL, revokeObjectURL: vi.fn() }));
    const user = userEvent.setup();
    renderScreen();
    await user.click(await screen.findByRole("tab", { name: /Balance y reportes/ }));
    await user.click(screen.getByRole("radio", { name: "Semana" }));
    expect(await screen.findByText("Semana del 12 al 18 de octubre de 2026")).toBeInTheDocument();
    expect(m.getReport).toHaveBeenLastCalledWith(expect.any(String), "weekly", "2026-10-15");
    expect(screen.getByText("Colegiatura")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /Excel/ }));
    expect(m.downloadReport).toHaveBeenCalledWith(expect.any(String), "weekly", "2026-10-15", "xlsx");
    vi.unstubAllGlobals();
  });
});

// Redesign F1: "Registrar egreso" from ⌘K or the home lands ON the drawer.
describe("opened from a shortcut", () => {
  afterEach(clearIntent);

  it("opens the new-expense drawer", async () => {
    requestIntent("expenses", "new");
    renderScreen();
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
  });

  it("a plain visit opens the plain screen", async () => {
    renderScreen();
    await screen.findByRole("button", { name: /Registrar egreso/ });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("never opens the drawer for a role that cannot write", async () => {
    requestIntent("expenses", "new");
    renderScreen(["expenses:read"]);
    await waitFor(() => expect(m.listExpenses).toHaveBeenCalled());
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
});
