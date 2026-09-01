import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as financeApi from "../../finance/api";
import { CollectDrawer } from "./CollectDrawer";

vi.mock("../../finance/api", () => ({
  getAccount: vi.fn(),
  previewDeposit: vi.fn(),
  registerDeposit: vi.fn(),
}));

const finance = vi.mocked(financeApi);

const student = { id: "s1", firstName: "Valentina", lastName: "Cruz", gradeGroup: "1° A" };

const payment = (id: string, chargeId: string, cents: number): financeApi.Payment => ({
  id, chargeId, amountCents: cents, method: "cash", reference: "", paidAt: "2026-10-10T00:00:00Z",
});

beforeEach(() => {
  finance.getAccount.mockResolvedValue({
    charges: [
      {
        id: "ch-sep", studentId: "s1", cycleId: "c1", conceptId: "k1", description: "Colegiatura septiembre",
        amountCents: 320000, discountCents: 0, dueDate: "2026-09-10", status: "pending",
        paidCents: 0, remainingCents: 320000,
      },
    ],
    payments: [],
    totalBalanceCents: 640000,
    unallocatedCents: 0,
  });
  finance.previewDeposit.mockResolvedValue({
    allocations: [payment("p1", "ch-sep", 320000)],
    surcharges: [],
    unallocatedCents: 350000,
  });
});
afterEach(() => vi.clearAllMocks());

describe("CollectDrawer", () => {
  // The counter shows ONE number: what the family owes. Everything else is
  // detail the cashier does not have to read out.
  it("leads with what the family owes", async () => {
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} />);

    expect(await screen.findByTestId("amount-owed")).toHaveTextContent("6,400");
    expect(screen.getByRole("heading", { name: /Valentina Cruz/ })).toBeInTheDocument();
  });

  // One field. The cashier types what was handed over and the system says what
  // it covers — that is the whole interaction the school asked for.
  it("shows what the typed amount would cover, before anything is taken", async () => {
    const user = userEvent.setup();
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} />);
    await screen.findByTestId("amount-owed");

    await user.type(screen.getByLabelText("Monto recibido"), "6700");

    await waitFor(() => expect(finance.previewDeposit).toHaveBeenCalled());
    expect(await screen.findByText(/Colegiatura septiembre/)).toBeInTheDocument();
    // The leftover is the family's credit, and the counter has to say so out
    // loud — invisible credit is how a parent gets asked for it twice.
    expect(await screen.findByTestId("credit")).toHaveTextContent("3,500");
    expect(finance.registerDeposit).not.toHaveBeenCalled();
  });

  it("takes the money and reports what it covered", async () => {
    const user = userEvent.setup();
    finance.registerDeposit.mockResolvedValue({
      deposit: {
        id: "dep-1", studentId: "s1", amountCents: 670000, method: "cash", reference: "",
        paidAt: "2026-10-10T00:00:00Z", calendarVersion: "abc",
      },
      allocations: [payment("p1", "ch-sep", 320000)],
      surcharges: [],
      unallocatedCents: 0,
      replayed: false,
    });
    const onCollected = vi.fn();
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={onCollected} />);
    await screen.findByTestId("amount-owed");

    await user.type(screen.getByLabelText("Monto recibido"), "6700");
    await user.click(screen.getByRole("button", { name: /Cobrar/ }));

    await waitFor(() => expect(finance.registerDeposit).toHaveBeenCalled());
    const sent = finance.registerDeposit.mock.calls[0]?.[1];
    expect(sent?.amountCents).toBe(670000);
    expect(sent?.studentId).toBe("s1");
    // One key per attempt: a double click must not cost the family twice.
    expect(sent?.idempotencyKey).toBeTruthy();
    expect(onCollected).toHaveBeenCalled();
    expect(await screen.findByTestId("receipt")).toBeInTheDocument();
  });

  // Enter is how a counter works. Reaching for the mouse on every payment is
  // the difference between the system being used and the notebook winning.
  it("takes the money on Enter", async () => {
    const user = userEvent.setup();
    finance.registerDeposit.mockResolvedValue({
      deposit: {
        id: "dep-1", studentId: "s1", amountCents: 320000, method: "cash", reference: "",
        paidAt: "2026-10-10T00:00:00Z", calendarVersion: "abc",
      },
      allocations: [], surcharges: [], unallocatedCents: 0, replayed: false,
    });
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} />);
    await screen.findByTestId("amount-owed");

    await user.type(screen.getByLabelText("Monto recibido"), "3200{Enter}");

    await waitFor(() => expect(finance.registerDeposit).toHaveBeenCalled());
  });

  it("closes on Escape, and does not take the money", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<CollectDrawer student={student} onClose={onClose} onCollected={() => {}} />);
    await screen.findByTestId("amount-owed");

    await user.keyboard("{Escape}");

    expect(onClose).toHaveBeenCalled();
    expect(finance.registerDeposit).not.toHaveBeenCalled();
  });

  // A refusal has to be readable. "No se pudo" tells a cashier nothing they can
  // act on with a parent standing there.
  it("shows the gateway's own words when the money is refused", async () => {
    const user = userEvent.setup();
    finance.registerDeposit.mockRejectedValue(new Error("la fecha del pago no puede estar en el futuro"));
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} />);
    await screen.findByTestId("amount-owed");

    await user.type(screen.getByLabelText("Monto recibido"), "6700");
    await user.click(screen.getByRole("button", { name: /Cobrar/ }));

    expect(await screen.findByRole("alert")).toHaveTextContent("la fecha del pago no puede estar en el futuro");
  });

  it("refuses to send an amount that is not money", async () => {
    const user = userEvent.setup();
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} />);
    await screen.findByTestId("amount-owed");

    await user.type(screen.getByLabelText("Monto recibido"), "0");
    await user.click(screen.getByRole("button", { name: /Cobrar/ }));

    expect(finance.registerDeposit).not.toHaveBeenCalled();
  });
});
