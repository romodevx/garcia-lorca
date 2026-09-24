import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as financeApi from "../../finance/api";
import { CollectDrawer } from "./CollectDrawer";

vi.mock("../../finance/api", () => ({
  getAccount: vi.fn(),
  getExpected: vi.fn(),
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
    unallocatedCents: 0, dueTodayCents: 0, surchargeCents: 0, earlyDiscountCents: 0, asOf: "",
  });
  finance.getExpected.mockResolvedValue({
    amountCents: 670000, surchargeCents: 30000, discountCents: 0, annualBalanceCents: 640000, lines: [],
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
  // The counter leads with what should be paid TODAY, not with the year: that
  // is the figure the cashier reads aloud and the parent hands over.
  it("leads with the amount expected today, with the year beside it", async () => {
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} />);

    expect(await screen.findByTestId("amount-expected")).toHaveTextContent("6,700");
    expect(screen.getByTestId("amount-owed")).toHaveTextContent("6,400");
    expect(screen.getByRole("heading", { name: /Valentina Cruz/ })).toBeInTheDocument();
  });

  // Zero means "paid exactly right" — the only value the office wants to see.
  it("shows the difference against what was expected", async () => {
    const user = userEvent.setup();
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} />);
    await screen.findByTestId("amount-expected");

    // Pre-filled with the expected figure, so it starts at zero.
    expect(screen.getByTestId("difference")).toHaveTextContent("0.00");

    await user.clear(screen.getByLabelText("Monto recibido"));
    await user.type(screen.getByLabelText("Monto recibido"), "6000");

    expect(await screen.findByTestId("difference")).toHaveTextContent("700");
  });

  // The father who always pays on the 5th and missed by a day. The school
  // decides; the system records that somebody decided.
  it("can grant the early-payment reward by hand", async () => {
    const user = userEvent.setup();
    finance.registerDeposit.mockResolvedValue({
      deposit: {
        id: "dep-1", studentId: "s1", amountCents: 310000, method: "cash", reference: "",
        paidAt: "2026-09-20T00:00:00Z", calendarVersion: "abc",
      },
      allocations: [], surcharges: [], unallocatedCents: 0, replayed: false,
    });
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} />);
    await screen.findByTestId("amount-expected");

    await user.click(screen.getByLabelText("Aplicar pronto pago"));
    await user.click(screen.getByRole("button", { name: /Cobrar/ }));

    await waitFor(() => expect(finance.registerDeposit).toHaveBeenCalled());
    expect(finance.registerDeposit.mock.calls[0]?.[1]?.grantEarlyPayment).toBe(true);
  });

  // One field. The cashier types what was handed over and the system says what
  // it covers — that is the whole interaction the school asked for.
  it("shows what the typed amount would cover, before anything is taken", async () => {
    const user = userEvent.setup();
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} />);
    await screen.findByTestId("amount-expected");

    await user.clear(screen.getByLabelText("Monto recibido"));
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
    await screen.findByTestId("amount-expected");

    await user.clear(screen.getByLabelText("Monto recibido"));
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
    await screen.findByTestId("amount-expected");

    await user.clear(screen.getByLabelText("Monto recibido"));
    await user.type(screen.getByLabelText("Monto recibido"), "3200{Enter}");

    await waitFor(() => expect(finance.registerDeposit).toHaveBeenCalled());
  });

  it("closes on Escape, and does not take the money", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<CollectDrawer student={student} onClose={onClose} onCollected={() => {}} />);
    await screen.findByTestId("amount-expected");

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
    await screen.findByTestId("amount-expected");

    await user.clear(screen.getByLabelText("Monto recibido"));
    await user.type(screen.getByLabelText("Monto recibido"), "6700");
    await user.click(screen.getByRole("button", { name: /Cobrar/ }));

    expect(await screen.findByRole("alert")).toHaveTextContent("la fecha del pago no puede estar en el futuro");
  });

  it("refuses to send an amount that is not money", async () => {
    const user = userEvent.setup();
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} />);
    await screen.findByTestId("amount-expected");

    await user.clear(screen.getByLabelText("Monto recibido"));
    await user.type(screen.getByLabelText("Monto recibido"), "0");
    await user.click(screen.getByRole("button", { name: /Cobrar/ }));

    expect(finance.registerDeposit).not.toHaveBeenCalled();
  });

  // The card type is asked for only when the money is on a card, and it reaches
  // the ledger: the office reconciles debit against credit at month end.
  it("asks which kind of card, only when the money came in on one", async () => {
    const user = userEvent.setup();
    finance.registerDeposit.mockResolvedValue({
      deposit: {
        id: "dep-1", studentId: "s1", amountCents: 670000, method: "card", reference: "",
        paidAt: "2026-10-10T00:00:00Z", calendarVersion: "abc", cardType: "credit",
      },
      allocations: [], surcharges: [], unallocatedCents: 0, replayed: false,
    });
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} />);
    await screen.findByTestId("amount-expected");

    // Cash: nothing to ask.
    expect(screen.queryByLabelText("Débito")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Tarjeta" }));
    await user.click(screen.getByLabelText("Crédito"));
    await user.click(screen.getByRole("button", { name: /Cobrar/ }));

    await waitFor(() => expect(finance.registerDeposit).toHaveBeenCalled());
    expect(finance.registerDeposit.mock.calls[0]?.[1]?.cardType).toBe("credit");
  });

describe("CollectDrawer · fecha de pago (tarea 7)", () => {
  const TODAY = "2026-10-12";
  const open = () => render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} today={TODAY} />);

  it("dates the payment today unless told otherwise, and never in the future", async () => {
    open();
    const date = await screen.findByLabelText("Fecha de pago");
    expect(date).toHaveValue(TODAY);
    expect(date).toHaveAttribute("max", TODAY);
  });

  // Every rule is a function of the day the family paid: the quote and the
  // preview are asked again for that day.
  it("asks for the quote and the preview of the day the family paid", async () => {
    open();
    const date = await screen.findByLabelText("Fecha de pago");
    fireEvent.change(date, { target: { value: "2026-10-03" } });

    await waitFor(() => expect(finance.getExpected).toHaveBeenLastCalledWith(expect.any(String), "s1", "2026-10-03"));
    await waitFor(() =>
      expect(finance.previewDeposit).toHaveBeenLastCalledWith(expect.any(String), expect.objectContaining({ paidAt: "2026-10-03" })),
    );
  });

  it("takes the money with the day the family paid", async () => {
    const user = userEvent.setup();
    finance.registerDeposit.mockResolvedValue({
      deposit: { id: "d1", studentId: "s1", amountCents: 670000, method: "cash", reference: "", paidAt: "2026-10-03T00:00:00Z", calendarVersion: "" },
      allocations: [], surcharges: [], unallocatedCents: 0, replayed: false,
    } as unknown as financeApi.DepositResult);
    open();
    fireEvent.change(await screen.findByLabelText("Fecha de pago"), { target: { value: "2026-10-03" } });
    await screen.findByTestId("amount-expected");
    await user.click(screen.getByRole("button", { name: "Cobrar" }));

    await waitFor(() => expect(finance.registerDeposit).toHaveBeenCalled());
    expect(finance.registerDeposit.mock.calls[0]?.[1]).toMatchObject({ paidAt: "2026-10-03" });
  });

  // Today is not sent: the server stamps its own clock, which keeps the hour
  // of the payment (a date alone is read as midnight UTC — 18:00 the day
  // before in Mexico City).
  it("sends no date for a payment made today", async () => {
    const user = userEvent.setup();
    finance.registerDeposit.mockResolvedValue({
      deposit: { id: "d1", studentId: "s1", amountCents: 670000, method: "cash", reference: "", paidAt: "", calendarVersion: "" },
      allocations: [], surcharges: [], unallocatedCents: 0, replayed: false,
    } as unknown as financeApi.DepositResult);
    open();
    await screen.findByTestId("amount-expected");
    await user.click(screen.getByRole("button", { name: "Cobrar" }));

    await waitFor(() => expect(finance.registerDeposit).toHaveBeenCalled());
    expect(finance.registerDeposit.mock.calls[0]?.[1]).not.toHaveProperty("paidAt");
    expect(finance.getExpected).toHaveBeenCalledWith(expect.any(String), "s1", "");
  });

  // Formatting the amount on the way out of the field is not the cashier
  // choosing another amount: a new date still brings its own quote.
  it("follows the quote of a new date when the amount was only reformatted", async () => {
    finance.getExpected.mockImplementation(async (_b, _s, on) => ({
      amountCents: on === "2026-10-03" ? 640000 : 670000, surchargeCents: 0, discountCents: 0, annualBalanceCents: 640000, lines: [],
    }));
    open();
    await screen.findByTestId("amount-expected");
    const amount = screen.getByLabelText(/Monto recibido/, { selector: "input" });
    fireEvent.blur(amount);
    fireEvent.change(screen.getByLabelText("Fecha de pago"), { target: { value: "2026-10-03" } });
    await waitFor(() => expect(amount).toHaveValue("6400.00"));
  });

  it("takes the focus to a date it cannot use, and says what is wrong", async () => {
    const user = userEvent.setup();
    open();
    const date = await screen.findByLabelText("Fecha de pago");
    fireEvent.change(date, { target: { value: "" } });
    await user.click(screen.getByRole("button", { name: "Cobrar" }));
    expect(await screen.findByText("Falta la fecha de pago.")).toBeInTheDocument();
    await waitFor(() => expect(document.activeElement).toBe(date));
    expect(finance.registerDeposit).not.toHaveBeenCalled();
  });

  it("says which day the expected amount is for", async () => {
    open();
    expect(await screen.findByText("Monto esperado hoy")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Fecha de pago"), { target: { value: "2026-10-03" } });
    expect(await screen.findByText("Monto esperado al 3 oct")).toBeInTheDocument();
  });

  // The label names the day of the quote ON SCREEN: a date typed that brought
  // no quote (invalid, or the request failed) does not relabel the old one.
  it("keeps the label on the day of the quote it shows", async () => {
    open();
    await screen.findByText("Monto esperado hoy");
    finance.getExpected.mockRejectedValueOnce(new Error("down"));
    fireEvent.change(screen.getByLabelText("Fecha de pago"), { target: { value: "2026-10-03" } });
    await waitFor(() => expect(finance.getExpected).toHaveBeenLastCalledWith(expect.any(String), "s1", "2026-10-03"));
    expect(screen.getByText("Monto esperado hoy")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Fecha de pago"), { target: { value: "2026-10-20" } });
    expect(screen.getByText("Monto esperado hoy")).toBeInTheDocument();
  });

  it("refuses a future date on the field, and takes nothing", async () => {
    const user = userEvent.setup();
    open();
    const date = await screen.findByLabelText("Fecha de pago");
    fireEvent.change(date, { target: { value: "2026-10-20" } });
    fireEvent.blur(date);
    expect(await screen.findByText("La fecha de pago no puede ser futura.")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Cobrar" }));
    expect(finance.registerDeposit).not.toHaveBeenCalled();
  });
});
});
