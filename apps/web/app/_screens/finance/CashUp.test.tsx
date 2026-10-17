import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as financeApi from "../../finance/api";
import { CashUpPanel } from "./CashUpPanel";

vi.mock("../../finance/api", () => ({ getCashUp: vi.fn() }));
const finance = vi.mocked(financeApi);

const deposit = (id: string, cents: number, method: financeApi.PaymentMethod): financeApi.Deposit => ({
  id, studentId: "s1", amountCents: cents, method, reference: "",
  paidAt: "2026-10-10T15:00:00Z", calendarVersion: "abc",
  tenders: [{ method, amountCents: cents }],
});

beforeEach(() => {
  finance.getCashUp.mockResolvedValue({
    deposits: [deposit("d1", 320000, "cash"), deposit("d2", 150000, "transfer")],
    cancelled: [], superseded: [],
    payments: [],
    cashCents: 320000,
    cardCents: 0,
    transferCents: 150000,
    totalCents: 470000, creditNoteCents: 0,
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
      cancelled: [{ ...deposit("d2", 150000, "cash"), voidedAt: "2026-10-10T16:00:00Z", voidReason: "importe capturado mal" }], superseded: [],
      payments: [],
      cashCents: 320000, cardCents: 0, transferCents: 0, totalCents: 320000, creditNoteCents: 0,
    });

    render(<CashUpPanel day="2026-10-10" />);

    expect(await screen.findByTestId("cash-up-total")).toHaveTextContent("3,200");
    expect(screen.getByText(/importe capturado mal/)).toBeInTheDocument();
  });

  // QA 2026-09-25, ISSUE-010: a counter sale is paid outside a deposit. The cut
  // lists it with what it paid for; the total the server sends already counts it.
  it("lists the payments taken outside a deposit, with what they paid for", async () => {
    finance.getCashUp.mockResolvedValue({
      deposits: [deposit("d1", 550000, "cash")],
      cancelled: [], superseded: [],
      payments: [{
        id: "p1", chargeId: "c1", amountCents: 135000, method: "cash", reference: "",
        paidAt: "2026-10-10T17:44:00Z", description: "Uniformes · U-2026-000001",
      }],
      cashCents: 685000, cardCents: 0, transferCents: 0, totalCents: 685000, creditNoteCents: 0,
    });

    render(<CashUpPanel day="2026-10-10" />);

    expect(await screen.findByTestId("cash-up-total")).toHaveTextContent("6,850");
    const rows = screen.getAllByTestId("cash-up-row");
    expect(rows).toHaveLength(2);
    expect(rows[1]).toHaveTextContent(/Uniformes · U-2026-000001/);
    expect(rows[1]).toHaveTextContent("1,350");
  });

  it("says so plainly when nothing came in", async () => {
    finance.getCashUp.mockResolvedValue({
      deposits: [], cancelled: [], superseded: [], payments: [], cashCents: 0, cardCents: 0, transferCents: 0, totalCents: 0, creditNoteCents: 0,
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
      cancelled: [], superseded: [],
      payments: [], cashCents: 470000, cardCents: 0, transferCents: 0, totalCents: 470000, creditNoteCents: 0,
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

  // Task 12: any payment of the day can be printed again.
  it("reprints a payment's ticket", async () => {
    finance.getCashUp.mockResolvedValue({
      deposits: [{ ...deposit("d1", 320000, "cash"), folio: "P-2026-000042" }],
      cancelled: [], superseded: [],
      payments: [], cashCents: 320000, cardCents: 0, transferCents: 0, totalCents: 320000, creditNoteCents: 0,
    } as unknown as financeApi.CashUp);
    const open = vi.spyOn(window, "open").mockImplementation(() => null);
    const user = userEvent.setup();
    render(<CashUpPanel day="2026-10-10" />);

    // The folio is on the row, so a paper ticket matches its line in the cut.
    expect(await screen.findByTestId("cash-up-row")).toHaveTextContent("P-2026-000042");
    await user.click(await screen.findByRole("button", { name: "Reimprimir P-2026-000042" }));
    expect(open).toHaveBeenCalledWith("/comprobantes/payment/d1?print=1", "_blank", "noopener");
  });

  it("names the day of a past cut, and invents no hour for a cancelled back-dated payment", async () => {
    finance.getCashUp.mockResolvedValue({
      deposits: [],
      cancelled: [{ ...deposit("d9", 100000, "cash"), paidAt: "2026-10-03T00:00:00Z", capturedAt: "2026-10-12T10:00:00", voidedAt: "2026-10-12T11:00:00Z", voidReason: "doble" }], superseded: [],
      payments: [], cashCents: 0, cardCents: 0, transferCents: 0, totalCents: 0, creditNoteCents: 0,
    } as unknown as financeApi.CashUp);
    render(<CashUpPanel day="2026-10-03" />);

    expect(await screen.findByText("Recibido el 3 oct")).toBeInTheDocument();
    expect(screen.queryByText("Recibido hoy")).not.toBeInTheDocument();
    expect(screen.getByText(/doble/)).not.toHaveTextContent(/p\. ?m\.|a\. ?m\./i);
  });
});

// T8: a credit note settles debt but brings no money into the drawer.
describe("CashUpPanel · nota de crédito y pago combinado (tarea 8)", () => {
  it("shows the day's credit notes on their own row, outside the money total", async () => {
    finance.getCashUp.mockResolvedValue({
      deposits: [{
        ...deposit("d1", 150000, "cash"), method: "mixed",
        tenders: [{ method: "credit_note", amountCents: 50000, reference: "NC-12" }, { method: "cash", amountCents: 100000 }],
      }],
      cancelled: [], superseded: [], payments: [],
      cashCents: 100000, cardCents: 0, transferCents: 0, totalCents: 100000, creditNoteCents: 50000,
    });

    render(<CashUpPanel day="2026-10-10" />);

    expect(await screen.findByTestId("cash-up-total")).toHaveTextContent("$1,000.00");
    expect(screen.getByTestId("cash-up-credit-note")).toHaveTextContent(/Notas de crédito/);
    expect(screen.getByTestId("cash-up-credit-note")).toHaveTextContent(/no es ingreso/);
    expect(screen.getByTestId("cash-up-credit-note")).toHaveTextContent("$500.00");
  });

  it("lists a split deposit by its tenders, not by a single method", async () => {
    finance.getCashUp.mockResolvedValue({
      deposits: [{
        ...deposit("d1", 150000, "cash"), method: "mixed",
        tenders: [{ method: "credit_note", amountCents: 50000 }, { method: "cash", amountCents: 100000 }],
      }],
      cancelled: [], superseded: [], payments: [],
      cashCents: 100000, cardCents: 0, transferCents: 0, totalCents: 100000, creditNoteCents: 50000,
    });

    render(<CashUpPanel day="2026-10-10" />);

    const row = await screen.findByTestId("cash-up-row");
    expect(row).toHaveTextContent("Nota de crédito $500.00 + Efectivo $1,000.00");
    expect(row).not.toHaveTextContent("Combinado");
  });

  it("names a credit-note payment taken outside a deposit", async () => {
    finance.getCashUp.mockResolvedValue({
      deposits: [], cancelled: [], superseded: [],
      payments: [{ id: "p1", chargeId: "c1", amountCents: 30000, method: "credit_note", reference: "", paidAt: "2026-10-10T17:44:00Z", description: "Uniformes · U-2026-000002" }],
      cashCents: 0, cardCents: 0, transferCents: 0, totalCents: 0, creditNoteCents: 30000,
    });

    render(<CashUpPanel day="2026-10-10" />);

    expect(await screen.findByTestId("cash-up-row")).toHaveTextContent("Nota de crédito");
  });

  it("does not show the credit-note row on a day without one", async () => {
    render(<CashUpPanel day="2026-10-10" />);
    await screen.findByTestId("cash-up-total");
    expect(screen.queryByTestId("cash-up-credit-note")).toBeNull();
  });

  // T9: a receipt replaced by its correction is listed apart, counting for
  // nothing, with the folio that replaced it.
  it("lists what was corrected that day, counted as nothing", async () => {
    finance.getCashUp.mockResolvedValue({
      deposits: [], cancelled: [], payments: [],
      superseded: [{ ...deposit("d3", 50000, "cash"), folio: "P-2026-000001", supersededBy: "d4", supersededByFolio: "P-2026-000002" }],
      cashCents: 0, cardCents: 0, transferCents: 0, totalCents: 0, creditNoteCents: 0,
    });

    render(<CashUpPanel day="2026-10-10" />);

    expect(await screen.findByText(/Sustituidos · no cuentan/)).toBeInTheDocument();
    expect(screen.getByText(/P-2026-000001 · sustituido por P-2026-000002/)).toBeInTheDocument();
    expect(screen.queryByText("No se recibió nada este día.")).toBeNull();
  });

  it("offers to correct a payment only to whoever may", async () => {
    const user = userEvent.setup();
    const onCorrect = vi.fn();
    const { unmount } = render(<CashUpPanel day="2026-10-10" />);
    await screen.findByTestId("cash-up-total");
    expect(screen.queryByRole("button", { name: /Corregir/ })).toBeNull();
    unmount();

    render(<CashUpPanel day="2026-10-10" onCorrect={onCorrect} />);
    await screen.findByTestId("cash-up-total");
    const buttons = screen.getAllByRole("button", { name: /Corregir/ });
    expect(buttons).toHaveLength(2);
    await user.click(buttons[0] as HTMLElement);
    expect(onCorrect).toHaveBeenCalledWith(expect.objectContaining({ id: "d1" }));
  });

  // ISSUE-T14-206: deposits came first and loose payments after, so 09:53 sat
  // above 08:35. The cut reads in the order the money came in; same instant, by folio.
  it("lists deposits and loose payments together, in the order they came in", async () => {
    const loose = (id: string, paidAt: string, description: string): financeApi.CashUpPayment => ({
      id, chargeId: "c1", amountCents: 10000, method: "cash", reference: "", paidAt, description,
    });
    finance.getCashUp.mockResolvedValue({
      deposits: [
        { ...deposit("d5", 200000, "cash"), paidAt: "2026-10-10T15:53:00Z", folio: "P-2026-000005" },
        { ...deposit("d4", 200000, "cash"), paidAt: "2026-10-10T15:53:00Z", folio: "P-2026-000004" },
      ],
      cancelled: [], superseded: [],
      payments: [loose("p2", "2026-10-10T14:45:00Z", "Uniformes · U-2026-000001"), loose("p1", "2026-10-10T14:35:00Z", "INSCRIPCIÓN")],
      cashCents: 420000, cardCents: 0, transferCents: 0, totalCents: 420000, creditNoteCents: 0,
    });

    render(<CashUpPanel day="2026-10-10" />);
    await screen.findByTestId("cash-up-total");

    const rows = screen.getAllByTestId("cash-up-row").map((r) => r.textContent ?? "");
    expect(rows).toHaveLength(4);
    expect(rows[0]).toMatch(/INSCRIPCIÓN/);
    expect(rows[1]).toMatch(/U-2026-000001/);
    expect(rows[2]).toMatch(/P-2026-000004/);
    expect(rows[3]).toMatch(/P-2026-000005/);
  });
});
