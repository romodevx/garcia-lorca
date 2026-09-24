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

  // Task 7: a payment can be dated the day the family paid. The cut says who
  // keyed each one, and flags one keyed on a later day.
  it("says who took each payment, and flags one captured late", async () => {
    finance.getCashUp.mockResolvedValue({
      deposits: [
        // A back-dated or same-day payment keyed on its own day: the date of
        // the payment is a calendar date (midnight UTC), never 18:00 the day
        // before.
        { ...deposit("d1", 320000, "cash"), paidAt: "2026-10-10T00:00:00Z", receivedByName: "Laura Caja", capturedAt: "2026-10-10T17:00:00" },
        { ...deposit("d2", 150000, "cash"), receivedByName: "Pedro Caja", capturedAt: "2026-10-13T10:00:00" },
      ],
      cancelled: [],
      cashCents: 470000, cardCents: 0, transferCents: 0, totalCents: 470000,
    } as unknown as financeApi.CashUp);
    render(<CashUpPanel day="2026-10-10" />);

    const rows = await screen.findAllByTestId("cash-up-row");
    expect(rows[0]).toHaveTextContent("Laura Caja");
    expect(rows[0]).not.toHaveTextContent("capturado");
    expect(rows[1]).toHaveTextContent("Pedro Caja");
    expect(rows[1]).toHaveTextContent(/capturado el 13 oct/i);
    // A back-dated payment carries a day, not an hour: none is invented.
    expect(rows[1]).not.toHaveTextContent(/p\. ?m\.|a\. ?m\./i);
  });

  it("names the day of a past cut, and invents no hour for a cancelled back-dated payment", async () => {
    finance.getCashUp.mockResolvedValue({
      deposits: [],
      cancelled: [{ ...deposit("d9", 100000, "cash"), paidAt: "2026-10-03T00:00:00Z", capturedAt: "2026-10-12T10:00:00", voidedAt: "2026-10-12T11:00:00Z", voidReason: "doble" }],
      cashCents: 0, cardCents: 0, transferCents: 0, totalCents: 0,
    } as unknown as financeApi.CashUp);
    render(<CashUpPanel day="2026-10-03" />);

    expect(await screen.findByText("Recibido el 3 oct")).toBeInTheDocument();
    expect(screen.queryByText("Recibido hoy")).not.toBeInTheDocument();
    expect(screen.getByText(/doble/)).not.toHaveTextContent(/p\. ?m\.|a\. ?m\./i);
  });
});
