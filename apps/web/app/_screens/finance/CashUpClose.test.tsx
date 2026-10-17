import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as financeApi from "../../finance/api";
import * as expensesApi from "../../expenses/api";
import { CashUpPanel } from "./CashUpPanel";

vi.mock("../../finance/api", () => ({ getCashUp: vi.fn() }));
vi.mock("../../expenses/api", () => ({ closeCashCut: vi.fn(), reopenCashCut: vi.fn() }));
const finance = vi.mocked(financeApi);
const expenses = vi.mocked(expensesApi);

const zero = { cashCents: 0, cardCents: 0, transferCents: 0, totalCents: 0 };

const expense = (id: string, cents: number, status: "active" | "cancelled" = "active"): expensesApi.Expense => ({
  id, spentOn: "2026-10-10", categoryId: "c", categoryName: "Papelería", amountCents: cents,
  method: "cash", description: "Tóner", status, ...(status === "cancelled" ? { cancelReason: "duplicado" } : {}),
});

const closure: expensesApi.CashCutClosure = {
  id: "cl-1", day: "2026-10-10", expectedCashCents: 290000, countedCashCents: 285000, differenceCents: -5000,
  income: { ...zero, cashCents: 320000, totalCents: 320000 }, expenses: { ...zero, cashCents: 30000, totalCents: 30000 },
  creditNoteCents: 0, closedByName: "Laura Caja", closedAt: "2026-10-10T23:00:00Z",
};

function cut(extra: Partial<financeApi.CashUp> = {}): financeApi.CashUp {
  return {
    deposits: [], cancelled: [], superseded: [], payments: [],
    cashCents: 320000, cardCents: 0, transferCents: 0, totalCents: 320000, creditNoteCents: 0,
    expenses: [expense("e1", 30000), expense("e2", 99900, "cancelled")],
    expensesByMethod: { ...zero, cashCents: 30000, totalCents: 30000 },
    expectedCashCents: 290000, modifiedAfterClose: false, reopened: [],
    ...extra,
  };
}

const access = { expenses: true, canClose: true, canReopen: true };

beforeEach(() => {
  finance.getCashUp.mockResolvedValue(cut());
});
afterEach(() => vi.clearAllMocks());

describe("CashUpPanel · egresos y cierre (T10)", () => {
  it("lists the day's expenses and the cash the drawer should hold", async () => {
    render(<CashUpPanel day="2026-10-10" access={access} />);

    expect(await screen.findByTestId("cash-up-expected")).toHaveTextContent("$2,900.00");
    const out = screen.getByTestId("cash-up-expenses");
    expect(out).toHaveTextContent("Papelería");
    expect(out).toHaveTextContent("$300.00");
    // A cancelled expense is listed, struck through, counted as nothing.
    expect(within(out).getByText("$999.00")).toHaveStyle({ textDecoration: "line-through" });
  });

  it("shows nothing of T10 when the module is off", async () => {
    render(<CashUpPanel day="2026-10-10" />);
    expect(await screen.findByTestId("cash-up-total")).toBeInTheDocument();
    expect(screen.queryByTestId("cash-up-expected")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Cerrar corte" })).not.toBeInTheDocument();
  });

  it("closes the day with the counted cash and shows the difference live", async () => {
    expenses.closeCashCut.mockResolvedValue(closure);
    const user = userEvent.setup();
    render(<CashUpPanel day="2026-10-10" access={access} />);

    await user.type(await screen.findByRole("textbox", { name: /Efectivo contado/ }), "2850");
    expect(screen.getByTestId("cash-up-difference")).toHaveTextContent("-$50.00");
    await user.click(screen.getByRole("button", { name: "Cerrar corte" }));

    expect(expenses.closeCashCut).toHaveBeenCalledWith(expect.any(String), { day: "2026-10-10", countedCashCents: 285000, note: "" });
    expect(finance.getCashUp).toHaveBeenCalledTimes(2);
  });

  it("asks for the counted cash before closing", async () => {
    const user = userEvent.setup();
    render(<CashUpPanel day="2026-10-10" access={access} />);
    await user.click(await screen.findByRole("button", { name: "Cerrar corte" }));
    expect(expenses.closeCashCut).not.toHaveBeenCalled();
    expect(await screen.findByText("Falta este dato")).toBeInTheDocument();
  });

  it("without expenses:write the open day cannot be closed from here", async () => {
    render(<CashUpPanel day="2026-10-10" access={{ ...access, canClose: false }} />);
    expect(await screen.findByTestId("cash-up-expected")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Cerrar corte" })).not.toBeInTheDocument();
  });

  it("a closed day shows its snapshot, who closed it, and the difference", async () => {
    finance.getCashUp.mockResolvedValue(cut({ closure }));
    render(<CashUpPanel day="2026-10-10" access={access} />);

    const card = await screen.findByTestId("cash-up-closure");
    expect(card).toHaveTextContent("Laura Caja");
    expect(card).toHaveTextContent("$2,850.00");
    expect(card).toHaveTextContent("-$50.00");
    expect(screen.queryByText(/Modificado después del cierre/)).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Cerrar corte" })).not.toBeInTheDocument();
  });

  it("says when the day changed after it was closed, and the director reopens it", async () => {
    finance.getCashUp.mockResolvedValue(cut({ closure, modifiedAfterClose: true }));
    expenses.reopenCashCut.mockResolvedValue({ ...closure, reopenedAt: "2026-10-11T10:00:00Z", reopenReason: "pago corregido" });
    const user = userEvent.setup();
    render(<CashUpPanel day="2026-10-10" access={access} />);

    expect(await screen.findByText(/Modificado después del cierre/)).toBeInTheDocument();
    // CC-215: a new payment after the close is the commonest change; the notice names it.
    expect(screen.getByText(/un cobro nuevo, una corrección, una cancelación o un egreso/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Reabrir corte" }));
    await user.click(screen.getByRole("button", { name: "Confirmar reapertura" }));
    expect(expenses.reopenCashCut).not.toHaveBeenCalled();
    await user.type(screen.getByRole("textbox", { name: /Motivo/ }), "pago corregido");
    await user.click(screen.getByRole("button", { name: "Confirmar reapertura" }));
    expect(expenses.reopenCashCut).toHaveBeenCalledWith(expect.any(String), "2026-10-10", "pago corregido");
  });

  it("only the director is offered the reopen", async () => {
    finance.getCashUp.mockResolvedValue(cut({ closure }));
    render(<CashUpPanel day="2026-10-10" access={{ ...access, canReopen: false }} />);
    expect(await screen.findByTestId("cash-up-closure")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Reabrir corte" })).not.toBeInTheDocument();
  });

  it("shows the server's refusal", async () => {
    expenses.closeCashCut.mockRejectedValue(new Error("el corte de ese día ya está cerrado"));
    const user = userEvent.setup();
    render(<CashUpPanel day="2026-10-10" access={access} />);
    await user.type(await screen.findByRole("textbox", { name: /Efectivo contado/ }), "2900");
    await user.click(screen.getByRole("button", { name: "Cerrar corte" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("el corte de ese día ya está cerrado");
  });
});

// Review T10: closing "today" closes the day the cut was read for, never the
// server's day at the moment of the POST (UTC rolls over at 18:00 local).
describe("CashUpPanel · el día que se cierra", () => {
  it("sends the cut's own day when the drawer asked for today", async () => {
    finance.getCashUp.mockResolvedValue(cut({ day: "2026-10-10" }));
    expenses.closeCashCut.mockResolvedValue(closure);
    const user = userEvent.setup();
    render(<CashUpPanel day="" access={access} />);
    await user.type(await screen.findByRole("textbox", { name: /Efectivo contado/ }), "2900");
    await user.click(screen.getByRole("button", { name: "Cerrar corte" }));
    expect(expenses.closeCashCut).toHaveBeenCalledWith(expect.any(String), { day: "2026-10-10", countedCashCents: 290000, note: "" });
  });
});
