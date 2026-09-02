import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as financeApi from "../../finance/api";
import { CashUpPanel } from "./CashUpPanel";

vi.mock("../../finance/api", () => ({ getCashUp: vi.fn() }));
const finance = vi.mocked(financeApi);

const deposit = (id: string, cents: number, method: financeApi.PaymentMethod): financeApi.Deposit => ({
  id, studentId: "s1", amountCents: cents, method, reference: "",
  paidAt: "2026-10-10T15:00:00Z", calendarVersion: "abc",
});

beforeEach(() => {
  finance.getCashUp.mockResolvedValue({
    deposits: [deposit("d1", 320000, "cash"), deposit("d2", 150000, "transfer")],
    cancelled: [],
    cashCents: 320000,
    cardCents: 0,
    transferCents: 150000,
    totalCents: 470000,
  });
});
afterEach(() => vi.clearAllMocks());

describe("CashUpPanel", () => {
  // The number the office closes the day with. If it is not the first thing on
  // the screen, someone is still adding up a notebook.
  it("leads with the day's total and the split by method", async () => {
    render(<CashUpPanel day="2026-10-10" />);

    expect(await screen.findByTestId("cash-up-total")).toHaveTextContent("4,700");
    expect(screen.getByTestId("cash-up-cash")).toHaveTextContent("3,200");
    expect(screen.getByTestId("cash-up-transfer")).toHaveTextContent("1,500");
  });

  // Hiding a cancelled deposit leaves a gap between the receipts and the drawer
  // that nobody can account for.
  it("shows what was cancelled, counted as nothing", async () => {
    finance.getCashUp.mockResolvedValue({
      deposits: [deposit("d1", 320000, "cash")],
      cancelled: [{ ...deposit("d2", 150000, "cash"), voidedAt: "2026-10-10T16:00:00Z", voidReason: "importe capturado mal" }],
      cashCents: 320000, cardCents: 0, transferCents: 0, totalCents: 320000,
    });

    render(<CashUpPanel day="2026-10-10" />);

    expect(await screen.findByTestId("cash-up-total")).toHaveTextContent("3,200");
    expect(screen.getByText(/importe capturado mal/)).toBeInTheDocument();
  });

  it("says so plainly when nothing came in", async () => {
    finance.getCashUp.mockResolvedValue({
      deposits: [], cancelled: [], cashCents: 0, cardCents: 0, transferCents: 0, totalCents: 0,
    });

    render(<CashUpPanel day="2026-10-10" />);

    expect(await screen.findByText(/No se recibió nada/)).toBeInTheDocument();
  });
});
