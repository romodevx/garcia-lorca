import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as financeApi from "../../finance/api";
import { CollectDrawer } from "./CollectDrawer";

vi.mock("../../finance/api", () => ({
  getAccount: vi.fn(),
  getExpected: vi.fn(),
  previewDeposit: vi.fn(),
  registerDeposit: vi.fn(),
  getCashUp: vi.fn(),
}));
const finance = vi.mocked(financeApi);

const TODAY = "2026-10-15";
const student = { id: "s1", firstName: "Valentina", lastName: "Cruz", gradeGroup: "1° A" };
const CLOSED_TODAY = /El corte de hoy ya se cerró/;

const dayCut = (closed: boolean): financeApi.CashUp => ({
  deposits: [], cancelled: [], superseded: [], payments: [],
  cashCents: 0, cardCents: 0, transferCents: 0, totalCents: 0, creditNoteCents: 0,
  ...(closed
    ? {
        closure: {
          id: "cut-1", day: TODAY, expectedCashCents: 0, countedCashCents: 0, differenceCents: 0,
          income: { cashCents: 0, cardCents: 0, transferCents: 0, totalCents: 0 },
          expenses: { cashCents: 0, cardCents: 0, transferCents: 0, totalCents: 0 },
          creditNoteCents: 0, closedAt: "2026-10-15T20:00:00Z",
        },
      }
    : {}),
});

beforeEach(() => {
  finance.getAccount.mockResolvedValue({
    charges: [], payments: [], totalBalanceCents: 320000,
    unallocatedCents: 0, dueTodayCents: 0, surchargeCents: 0, earlyDiscountCents: 0, asOf: "",
  });
  finance.getExpected.mockResolvedValue({
    amountCents: 320000, surchargeCents: 0, discountCents: 0, annualBalanceCents: 320000, onTimeBalanceCents: 320000, lines: [],
  });
  finance.previewDeposit.mockResolvedValue({ allocations: [], surcharges: [], unallocatedCents: 0 });
});
afterEach(() => vi.clearAllMocks());

const open = (): void => {
  render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} today={TODAY} />);
};

// QA 2026-10-15, ISSUE-026: collecting after the day's cut was closed said
// nothing at the time; only later the cut read "Modificado después del
// cierre". The cashier is told before confirming — and may still collect.
describe("CollectDrawer — the day's cut already closed (ISSUE-026)", () => {
  it("warns before collecting when today's cut is closed, without blocking it", async () => {
    finance.getCashUp.mockResolvedValue(dayCut(true));
    finance.registerDeposit.mockResolvedValue({
      deposit: { id: "d1", studentId: "s1", amountCents: 320000, method: "cash", reference: "", paidAt: "", calendarVersion: "", tenders: [] },
      allocations: [], surcharges: [], unallocatedCents: 0, replayed: false,
    } as unknown as financeApi.DepositResult);
    const user = userEvent.setup();
    open();

    const warning = await screen.findByText(CLOSED_TODAY);
    expect(warning).toHaveTextContent("Este cobro lo dejará marcado como modificado después del cierre.");
    expect(finance.getCashUp).toHaveBeenCalledWith(expect.any(String), "");

    await screen.findAllByDisplayValue("3,200.00");
    await user.click(screen.getByRole("button", { name: "Cobrar" }));
    await waitFor(() => expect(finance.registerDeposit).toHaveBeenCalledTimes(1));
  });

  // Tech-lead: a live region announces a change only when it is already in
  // the page; mounting it with its text reads as nothing on some readers.
  it("keeps the status region mounted and only changes its text", async () => {
    const answer = { resolve: (_c: financeApi.CashUp): void => {} };
    finance.getCashUp.mockReturnValue(new Promise<financeApi.CashUp>((r) => { answer.resolve = r; }));
    open();

    const region = screen.getByTestId("closed-cut-status");
    expect(region).toHaveAttribute("role", "status");
    expect(region).toHaveTextContent("");
    await act(async () => answer.resolve(dayCut(true)));

    expect(screen.getByTestId("closed-cut-status")).toBe(region);
    expect(region).toHaveTextContent(CLOSED_TODAY);
  });

  it("says nothing while the day is open", async () => {
    finance.getCashUp.mockResolvedValue(dayCut(false));
    open();

    await waitFor(() => expect(finance.getCashUp).toHaveBeenCalled());
    await screen.findAllByDisplayValue("3,200.00");
    expect(screen.queryByText(/ya se cerró/)).toBeNull();
  });

  it("asks about the cut of the day the family paid, when it is an earlier one", async () => {
    finance.getCashUp.mockImplementation(async (_base, day) => dayCut(day === "2026-10-13"));
    open();

    fireEvent.change(screen.getByLabelText("Fecha de pago"), { target: { value: "2026-10-13" } });

    expect(await screen.findByText(/El corte del 13 oct ya se cerró/)).toBeInTheDocument();
  });

  it("collects as before when the cut cannot be read", async () => {
    finance.getCashUp.mockRejectedValue(new Error("403"));
    open();

    await screen.findAllByDisplayValue("3,200.00");
    expect(screen.queryByText(/ya se cerró/)).toBeNull();
  });
});

describe("CollectDrawer — Cobrar while saving", () => {
  it("is busy, not disabled, and a double click sends one payment", async () => {
    finance.getCashUp.mockResolvedValue(dayCut(false));
    finance.registerDeposit.mockReturnValue(new Promise<financeApi.DepositResult>(() => {}));
    open();
    await screen.findAllByDisplayValue("3,200.00");

    // Two presses inside one render: state cannot see the first, a ref can.
    const cobrar = screen.getByRole("button", { name: "Cobrar" });
    act(() => {
      cobrar.click();
      cobrar.click();
    });

    const busy = await screen.findByRole("button", { name: "Cobrando…" });
    expect(busy).toBeEnabled();
    expect(busy).toHaveAttribute("aria-busy", "true");
    expect(finance.registerDeposit).toHaveBeenCalledTimes(1);
  });
});
