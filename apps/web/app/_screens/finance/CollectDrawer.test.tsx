import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { HttpError } from "../../auth/httpError";
import * as financeApi from "../../finance/api";
import { CollectDrawer, maxPayableCents } from "./CollectDrawer";

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
    amountCents: 670000, surchargeCents: 30000, discountCents: 0, annualBalanceCents: 640000, onTimeBalanceCents: 640000, lines: [],
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
    expect(screen.getByRole("heading", { name: /Cruz Valentina/ })).toBeInTheDocument();
  });

  // Zero means "paid exactly right" — the only value the office wants to see.
  it("shows the difference against what was expected", async () => {
    const user = userEvent.setup();
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} />);
    await screen.findByTestId("amount-expected");

    // Pre-filled with the expected figure, so it starts at zero.
    expect(screen.getByTestId("difference")).toHaveTextContent("0.00");

    await user.clear(screen.getByLabelText("Monto a pagar"));
    await user.type(screen.getByLabelText("Monto a pagar"), "6000");

    expect(await screen.findByTestId("difference")).toHaveTextContent("700");
  });

  // The father who always pays on the 5th and missed by a day. The school
  // decides; the system records that somebody decided.
  it("can grant the early-payment reward by hand", async () => {
    const user = userEvent.setup();
    finance.registerDeposit.mockResolvedValue({
      deposit: {
        id: "dep-1", studentId: "s1", amountCents: 310000, method: "cash", reference: "",
        paidAt: "2026-09-20T00:00:00Z", calendarVersion: "abc", tenders: [],
      },
      allocations: [], surcharges: [], unallocatedCents: 0, replayed: false,
    });
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} canGrantDiscounts />);
    await screen.findByTestId("amount-expected");

    await user.click(screen.getByLabelText("Aplicar pronto pago"));
    await user.click(screen.getByRole("button", { name: /Cobrar/ }));

    await waitFor(() => expect(finance.registerDeposit).toHaveBeenCalled());
    expect(finance.registerDeposit.mock.calls[0]?.[1]?.grantEarlyPayment).toBe(true);
  });

  // Task 9: the late reward is a discount, finance:write's. The Cobranza role
  // takes money at the listed price, so the switch is not offered to it.
  it("does not offer the late reward to whoever cannot grant discounts", async () => {
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} />);
    await screen.findByTestId("amount-expected");

    expect(screen.queryByLabelText("Aplicar pronto pago")).toBeNull();
  });

  // Task 12: the receipt says the deposit's folio, the one the printed
  // receipt and the PDF carry — not a slice of its id.
  it("shows the deposit's folio on the receipt", async () => {
    const user = userEvent.setup();
    finance.registerDeposit.mockResolvedValue({
      deposit: {
        id: "0c4a4943-0000-0000-0000-000000000001", studentId: "s1", amountCents: 310000, method: "cash", reference: "",
        paidAt: "2026-09-20T00:00:00Z", calendarVersion: "abc", tenders: [], folio: "P-2026-000042",
      },
      allocations: [], surcharges: [], unallocatedCents: 0, replayed: false,
    });
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} />);
    await screen.findByTestId("amount-expected");

    await user.click(screen.getByRole("button", { name: /Cobrar/ }));

    expect(await screen.findByText(/P-2026-000042/)).toBeInTheDocument();

    // The ticket prints on its own 80 mm page; the PDF downloads.
    const open = vi.spyOn(window, "open").mockImplementation(() => null);
    await user.click(screen.getByRole("button", { name: "Imprimir ticket" }));
    expect(open).toHaveBeenCalledWith("/comprobantes/payment/0c4a4943-0000-0000-0000-000000000001?print=1", "_blank", "noopener");
    expect(screen.getByRole("button", { name: "Descargar PDF" })).toBeInTheDocument();
  });

  // One field. The cashier types what was handed over and the system says what
  // it covers — that is the whole interaction the school asked for.
  it("shows what the typed amount would cover, before anything is taken", async () => {
    const user = userEvent.setup();
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} />);
    await screen.findByTestId("amount-expected");

    await user.clear(screen.getByLabelText("Monto a pagar"));
    await user.type(screen.getByLabelText("Monto a pagar"), "6700");

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
        paidAt: "2026-10-10T00:00:00Z", calendarVersion: "abc", tenders: [],
      },
      allocations: [payment("p1", "ch-sep", 320000)],
      surcharges: [],
      unallocatedCents: 0,
      replayed: false,
    });
    const onCollected = vi.fn();
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={onCollected} />);
    await screen.findByTestId("amount-expected");

    await user.clear(screen.getByLabelText("Monto a pagar"));
    await user.type(screen.getByLabelText("Monto a pagar"), "6700");
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
        paidAt: "2026-10-10T00:00:00Z", calendarVersion: "abc", tenders: [],
      },
      allocations: [], surcharges: [], unallocatedCents: 0, replayed: false,
    });
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} />);
    await screen.findByTestId("amount-expected");

    await user.clear(screen.getByLabelText("Monto a pagar"));
    await user.type(screen.getByLabelText("Monto a pagar"), "3200{Enter}");

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

    await user.clear(screen.getByLabelText("Monto a pagar"));
    await user.type(screen.getByLabelText("Monto a pagar"), "6700");
    await user.click(screen.getByRole("button", { name: /Cobrar/ }));

    expect(await screen.findByRole("alert")).toHaveTextContent("la fecha del pago no puede estar en el futuro");
  });

  it("refuses to send an amount that is not money", async () => {
    const user = userEvent.setup();
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} />);
    await screen.findByTestId("amount-expected");

    await user.clear(screen.getByLabelText("Monto a pagar"));
    await user.type(screen.getByLabelText("Monto a pagar"), "0");
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
        paidAt: "2026-10-10T00:00:00Z", calendarVersion: "abc", tenders: [], cardType: "credit",
      },
      allocations: [], surcharges: [], unallocatedCents: 0, replayed: false,
    });
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} />);
    await screen.findByTestId("amount-expected");

    // Cash: nothing to ask.
    expect(screen.queryByRole("button", { name: "Débito" })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Tarjeta" }));
    await user.click(screen.getByRole("button", { name: "Crédito" }));
    await user.click(screen.getByRole("button", { name: /Cobrar/ }));

    await waitFor(() => expect(finance.registerDeposit).toHaveBeenCalled());
    expect(finance.registerDeposit.mock.calls[0]?.[1]?.cardType).toBe("credit");
    expect(finance.registerDeposit.mock.calls[0]?.[1]?.tenders).toEqual([{ method: "card", amountCents: 670000, cardType: "credit" }]);
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
      deposit: { id: "d1", studentId: "s1", amountCents: 670000, method: "cash", reference: "", paidAt: "2026-10-03T00:00:00Z", calendarVersion: "", tenders: [] },
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
      deposit: { id: "d1", studentId: "s1", amountCents: 670000, method: "cash", reference: "", paidAt: "", calendarVersion: "", tenders: [] },
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
      amountCents: on === "2026-10-03" ? 640000 : 670000, surchargeCents: 0, discountCents: 0, annualBalanceCents: 640000, onTimeBalanceCents: 640000, lines: [],
    }));
    open();
    await screen.findByTestId("amount-expected");
    const amount = screen.getByLabelText(/Monto a pagar/, { selector: "input" });
    fireEvent.blur(amount);
    fireEvent.change(screen.getByLabelText("Fecha de pago"), { target: { value: "2026-10-03" } });
    await waitFor(() => expect(amount).toHaveValue("6,400.00"));
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

describe("CollectDrawer · saldo a favor (T5)", () => {
  // The balance already nets the money no charge claimed. Below zero the
  // school owes the family: credit, as a positive amount, never "-$500.00".
  it("names a balance below zero as credit", async () => {
    finance.getAccount.mockResolvedValue({
      charges: [], payments: [], totalBalanceCents: -50000,
      unallocatedCents: 50000, dueTodayCents: 0, surchargeCents: 0, earlyDiscountCents: 0, asOf: "",
    });
    finance.getExpected.mockResolvedValue({ amountCents: 0, surchargeCents: 0, discountCents: 0, annualBalanceCents: -50000, onTimeBalanceCents: -50000, lines: [] });
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} />);

    expect(await screen.findByTestId("amount-owed")).toHaveTextContent(/^\$500\.00$/);
    expect(screen.getByTestId("amount-owed").parentElement).toHaveTextContent(/Tiene saldo a favor/);
    expect(screen.queryByText(/Del año debe/)).toBeNull();
  });
});

describe("CollectDrawer · el adeudo, concepto por concepto (tarea 8)", () => {
  const lines: financeApi.ExpectedLine[] = [
    { chargeId: "ch-sep", description: "Colegiatura septiembre", dueDate: "2026-09-10", owedCents: 320000, surchargeCents: 30000, discountCents: 0, scholarshipCents: 0, totalCents: 350000, due: true, inBalance: true },
    { chargeId: "ch-oct", description: "Colegiatura octubre", dueDate: "2026-10-10", owedCents: 320000, surchargeCents: 0, discountCents: 0, scholarshipCents: 0, totalCents: 320000, due: true, inBalance: true },
    { chargeId: "ch-nov", description: "Colegiatura noviembre", dueDate: "2026-11-10", owedCents: 320000, surchargeCents: 0, discountCents: 0, scholarshipCents: 0, totalCents: 320000, due: false, inBalance: true },
  ];
  beforeEach(() => {
    finance.getExpected.mockResolvedValue({
      amountCents: 670000, surchargeCents: 30000, discountCents: 0, annualBalanceCents: 990000, onTimeBalanceCents: 990000, lines,
    });
  });
  const open = () => render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} today="2026-10-12" />);

  // What is due is chosen by default — what the counter asks for today, the
  // rule as it always was. A month not yet due is offered, unchosen: choosing
  // it is paying ahead.
  it("lists what is owed, the due concepts chosen, with its surcharge and the two totals", async () => {
    open();
    const sep = await screen.findByRole("checkbox", { name: "Colegiatura septiembre" });
    expect(sep).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Colegiatura octubre" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Colegiatura noviembre" })).not.toBeChecked();
    expect(screen.getByTestId("debt-row-ch-sep")).toHaveTextContent("3,500");
    expect(screen.getByTestId("debt-row-ch-sep")).toHaveTextContent(/recargo \$300/i);
    expect(screen.getByTestId("debt-total")).toHaveTextContent("9,900");
    expect(screen.getByTestId("debt-selected")).toHaveTextContent("6,700");
  });

  // T5 (ISSUE-006): ONE balance. The debt's total is the year's balance —
  // the same figure as "Del año debe" — and says it carries the surcharge.
  it("reads the debt's total as the one balance, surcharge included", async () => {
    open();
    await screen.findByRole("checkbox", { name: "Colegiatura septiembre" });
    expect(screen.getByTestId("amount-owed")).toHaveTextContent("9,900");
    expect(screen.getByTestId("debt-total")).toHaveTextContent("9,900");
    expect(screen.getByTestId("amount-owed").parentElement).toHaveTextContent(/con el recargo incluido/);
    expect(screen.queryByTestId("debt-on-time")).toBeNull();
  });

  // A month not yet due quoted with its early-payment price is a projection,
  // not a second debt total: it is said apart, as what paying on time costs.
  it("says the pronto-pago projection apart from the debt", async () => {
    const withReward: financeApi.ExpectedLine[] = [
      lines[0] as financeApi.ExpectedLine,
      lines[1] as financeApi.ExpectedLine,
      { chargeId: "ch-nov", description: "Colegiatura noviembre", dueDate: "2026-11-10", owedCents: 320000, surchargeCents: 0, discountCents: 20000, scholarshipCents: 0, totalCents: 300000, due: false, inBalance: true },
    ];
    finance.getExpected.mockResolvedValue({ amountCents: 670000, surchargeCents: 30000, discountCents: 0, annualBalanceCents: 990000, onTimeBalanceCents: 970000, lines: withReward });
    open();
    await screen.findByRole("checkbox", { name: "Colegiatura noviembre" });

    expect(screen.getByTestId("debt-total")).toHaveTextContent("9,900");
    expect(screen.getByTestId("debt-on-time")).toHaveTextContent(/Si paga cada mes a tiempo/);
    expect(screen.getByTestId("debt-on-time")).toHaveTextContent("9,700");
  });

  it("follows the choice: paying ahead, and back", async () => {
    const user = userEvent.setup();
    open();
    const amount = () => screen.getByLabelText(/Monto a pagar/, { selector: "input" });
    await user.click(await screen.findByRole("checkbox", { name: "Colegiatura noviembre" }));

    expect(screen.getByTestId("debt-selected")).toHaveTextContent("9,900");
    expect(amount()).toHaveValue("9,900.00");
    await waitFor(() =>
      expect(finance.previewDeposit).toHaveBeenLastCalledWith(expect.any(String), expect.objectContaining({ chargeIds: ["ch-sep", "ch-oct", "ch-nov"] })),
    );

    // Back to what is due: the amount returns to it, and no choice is sent.
    await user.click(screen.getByRole("checkbox", { name: "Colegiatura noviembre" }));
    expect(amount()).toHaveValue("6,700.00");
    expect(screen.getByTestId("difference")).toHaveTextContent("$0.00");
    await waitFor(() => expect(finance.previewDeposit).toHaveBeenLastCalledWith(expect.any(String), expect.not.objectContaining({ chargeIds: expect.anything() })));
  });

  it("takes the money against the chosen charges only", async () => {
    const user = userEvent.setup();
    finance.registerDeposit.mockResolvedValue({
      deposit: { id: "d1", studentId: "s1", amountCents: 320000, method: "cash", reference: "", paidAt: "", calendarVersion: "", tenders: [] },
      allocations: [], surcharges: [], unallocatedCents: 0, replayed: false,
    } as unknown as financeApi.DepositResult);
    open();
    await user.click(await screen.findByRole("checkbox", { name: "Colegiatura septiembre" }));
    expect(screen.getByLabelText(/Monto a pagar/, { selector: "input" })).toHaveValue("3,200.00");
    await user.click(screen.getByRole("button", { name: "Cobrar" }));

    await waitFor(() => expect(finance.registerDeposit).toHaveBeenCalled());
    expect(finance.registerDeposit.mock.calls[0]?.[1]).toMatchObject({ chargeIds: ["ch-oct"], amountCents: 320000 });
  });

  // A new quote (another date) is a new list: the choice starts over.
  it("starts the choice over with a new quote", async () => {
    const user = userEvent.setup();
    open();
    await user.click(await screen.findByRole("checkbox", { name: "Colegiatura noviembre" }));
    fireEvent.change(screen.getByLabelText("Fecha de pago"), { target: { value: "2026-10-03" } });
    await waitFor(() => expect(screen.getByRole("checkbox", { name: "Colegiatura noviembre" })).not.toBeChecked());
    expect(screen.getByTestId("debt-selected")).toHaveTextContent("6,700");
  });

  it("sends no choice when what is due is what is chosen: the oldest debt first, as always", async () => {
    const user = userEvent.setup();
    finance.registerDeposit.mockResolvedValue({
      deposit: { id: "d1", studentId: "s1", amountCents: 670000, method: "cash", reference: "", paidAt: "", calendarVersion: "", tenders: [] },
      allocations: [], surcharges: [], unallocatedCents: 0, replayed: false,
    } as unknown as financeApi.DepositResult);
    open();
    await screen.findByRole("checkbox", { name: "Colegiatura septiembre" });
    await user.click(screen.getByRole("button", { name: "Cobrar" }));
    await waitFor(() => expect(finance.registerDeposit).toHaveBeenCalled());
    expect(finance.registerDeposit.mock.calls[0]?.[1]).not.toHaveProperty("chargeIds");
  });
});

// T8: a credit note is a way to pay, typed by hand, and any method can be
// completed with another.
describe("CollectDrawer · nota de crédito + pago combinado (tarea 8)", () => {
  const open = () => render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} today="2026-10-12" />);
  const tenderRow = (n: number): HTMLElement => screen.getByRole("group", { name: `Forma de pago ${n}` });
  const result = (tenders: financeApi.Tender[], allocations: financeApi.Payment[] = []): financeApi.DepositResult => ({
    deposit: {
      id: "d1", studentId: "s1", amountCents: 670000, method: tenders.length > 1 ? "mixed" : "credit_note",
      reference: "", paidAt: "", calendarVersion: "", tenders, folio: "P-2026-000050",
    },
    allocations, surcharges: [], unallocatedCents: 0, replayed: false,
  });

  it("asks for the amount to pay, prefilled with what is expected", async () => {
    open();
    await screen.findByTestId("amount-expected");
    expect(screen.getByLabelText("Monto a pagar", { selector: "input" })).toHaveValue("6,700.00");
    expect(screen.queryByLabelText("Monto recibido")).toBeNull();
    expect(screen.getByTestId("tender-total")).toHaveTextContent("$6,700.00");
  });

  it("takes a credit note that covers the whole amount", async () => {
    const user = userEvent.setup();
    finance.registerDeposit.mockResolvedValue(result([{ method: "credit_note", amountCents: 670000 }]));
    open();
    await screen.findByTestId("amount-expected");

    await user.click(within(tenderRow(1)).getByRole("button", { name: "Nota de crédito" }));
    await user.type(within(tenderRow(1)).getByLabelText("Referencia"), "NC-7");
    await user.click(screen.getByRole("button", { name: "Cobrar" }));

    await waitFor(() => expect(finance.registerDeposit).toHaveBeenCalled());
    expect(finance.registerDeposit.mock.calls[0]?.[1]).toMatchObject({
      amountCents: 670000,
      method: "credit_note",
      tenders: [{ method: "credit_note", amountCents: 670000, reference: "NC-7" }],
    });
  });

  it("completes a credit note with cash, and shows both on the receipt", async () => {
    const user = userEvent.setup();
    const tenders: financeApi.Tender[] = [
      { method: "credit_note", amountCents: 50000, reference: "NC-7" },
      { method: "cash", amountCents: 620000 },
    ];
    finance.registerDeposit.mockResolvedValue(result(tenders, [
      payment("p1", "ch-sep", 50000),
      payment("p2", "ch-sep", 270000),
      payment("p3", "ch-oct", 350000),
    ]));
    open();
    await screen.findByTestId("amount-expected");

    await user.click(within(tenderRow(1)).getByRole("button", { name: "Nota de crédito" }));
    await user.clear(within(tenderRow(1)).getByLabelText("Monto"));
    await user.type(within(tenderRow(1)).getByLabelText("Monto"), "500");
    await user.type(within(tenderRow(1)).getByLabelText("Referencia"), "NC-7");
    await user.click(screen.getByRole("button", { name: "Agregar método" }));
    expect(within(tenderRow(2)).getByLabelText("Monto")).toHaveValue("6,200.00");
    await user.click(screen.getByRole("button", { name: "Cobrar" }));

    await waitFor(() => expect(finance.registerDeposit).toHaveBeenCalled());
    expect(finance.registerDeposit.mock.calls[0]?.[1]?.tenders).toEqual(tenders);

    const receipt = await screen.findByTestId("receipt");
    expect(within(receipt).getByTestId("receipt-tenders")).toHaveTextContent(/Nota de crédito.*NC-7.*\$500\.00/);
    expect(within(receipt).getByTestId("receipt-tenders")).toHaveTextContent(/Efectivo.*\$6,200\.00/);
    // The same month paid by two methods is read once, with its total.
    expect(within(receipt).getAllByText("Colegiatura septiembre")).toHaveLength(1);
    expect(within(receipt).getByText("$3,200.00")).toBeInTheDocument();
  });

  it("refuses a split that does not add up to the amount to pay, and goes to it", async () => {
    const user = userEvent.setup();
    open();
    await screen.findByTestId("amount-expected");

    await user.clear(within(tenderRow(1)).getByLabelText("Monto"));
    await user.type(within(tenderRow(1)).getByLabelText("Monto"), "500");
    await user.click(screen.getByRole("button", { name: "Cobrar" }));

    expect(await screen.findByText("La suma de las formas de pago debe ser igual al monto a pagar.")).toBeInTheDocument();
    await waitFor(() => expect(document.activeElement).toBe(within(tenderRow(1)).getByLabelText("Monto")));
    expect(finance.registerDeposit).not.toHaveBeenCalled();
  });

  it("refuses an amount to pay above what the family owes, and goes to it", async () => {
    const user = userEvent.setup();
    open();
    await screen.findByTestId("amount-expected");
    const amount = screen.getByLabelText("Monto a pagar", { selector: "input" });

    await user.clear(amount);
    await user.type(amount, "9000");
    await user.click(screen.getByRole("button", { name: "Cobrar" }));

    expect(await screen.findByText(/No puede ser mayor que lo que se puede cobrar hoy/)).toBeInTheDocument();
    await waitFor(() => expect(document.activeElement).toBe(amount));
    expect(finance.registerDeposit).not.toHaveBeenCalled();
  });

  it("lists a charge once in the preview when the planner splits it", async () => {
    finance.previewDeposit.mockResolvedValue({
      allocations: [payment("p1", "ch-sep", 50000), payment("p2", "ch-sep", 270000)],
      surcharges: [], unallocatedCents: 0,
    });
    open();
    const preview = await screen.findByTestId("preview");
    await waitFor(() => expect(within(preview).getAllByText("Colegiatura septiembre")).toHaveLength(1));
    expect(within(preview).getByText("$3,200.00")).toBeInTheDocument();
  });
});

// D2 (T5 review): the cap on what may be taken is the gross debt the quote
// lists — every line, the ones not yet open included — never the net
// balance, which leaves those out and nets the credit (it can be negative).
describe("CollectDrawer · pagar por adelantado lo que aún no abre (T5)", () => {
  it("accepts an unmarked advance payment covering a charge not yet open", async () => {
    const user = userEvent.setup();
    finance.getAccount.mockResolvedValue({
      charges: [], payments: [], totalBalanceCents: 0,
      unallocatedCents: 0, dueTodayCents: 0, surchargeCents: 0, earlyDiscountCents: 0, asOf: "",
    });
    finance.getExpected.mockResolvedValue({
      amountCents: 0, surchargeCents: 0, discountCents: 0, annualBalanceCents: 0, onTimeBalanceCents: 0,
      lines: [{ chargeId: "ch-ins", description: "Inscripción", dueDate: "2026-06-30", owedCents: 50000, surchargeCents: 0, discountCents: 0, scholarshipCents: 0, totalCents: 50000, due: false, inBalance: false }],
    });
    finance.registerDeposit.mockResolvedValue({
      deposit: { id: "d1", studentId: "s1", amountCents: 50000, method: "cash", reference: "", paidAt: "", calendarVersion: "", tenders: [] },
      allocations: [], surcharges: [], unallocatedCents: 0, replayed: false,
    } as unknown as financeApi.DepositResult);
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} today="2026-05-20" />);
    await screen.findByRole("checkbox", { name: "Inscripción" });

    const amount = screen.getByLabelText(/Monto a pagar/, { selector: "input" });
    await user.clear(amount);
    await user.type(amount, "500");
    await user.click(screen.getByRole("button", { name: "Cobrar" }));

    await waitFor(() => expect(finance.registerDeposit).toHaveBeenCalled());
    expect(finance.registerDeposit.mock.calls[0]?.[1]).toMatchObject({ amountCents: 50000 });
    expect(screen.queryByText(/No puede ser mayor que lo que se puede cobrar hoy/)).toBeNull();
  });

  it("still refuses more than every line listed adds up to", async () => {
    const user = userEvent.setup();
    finance.getAccount.mockResolvedValue({
      charges: [], payments: [], totalBalanceCents: 0,
      unallocatedCents: 0, dueTodayCents: 0, surchargeCents: 0, earlyDiscountCents: 0, asOf: "",
    });
    finance.getExpected.mockResolvedValue({
      amountCents: 0, surchargeCents: 0, discountCents: 0, annualBalanceCents: 0, onTimeBalanceCents: 0,
      lines: [{ chargeId: "ch-ins", description: "Inscripción", dueDate: "2026-06-30", owedCents: 50000, surchargeCents: 0, discountCents: 0, scholarshipCents: 0, totalCents: 50000, due: false, inBalance: false }],
    });
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} today="2026-05-20" />);
    await screen.findByRole("checkbox", { name: "Inscripción" });

    const amount = screen.getByLabelText(/Monto a pagar/, { selector: "input" });
    await user.clear(amount);
    await user.type(amount, "600");
    await user.click(screen.getByRole("button", { name: "Cobrar" }));

    expect(await screen.findAllByText(/No puede ser mayor que lo que se puede cobrar hoy \(\$500\.00\)/)).not.toHaveLength(0);
    expect(finance.registerDeposit).not.toHaveBeenCalled();
  });
});

// CC-202: the server refuses money that does not land in full on the charges
// it may pay. With concepts chosen, it may pay only those: the cap is what
// they add up to, not every line listed.
describe("maxPayableCents", () => {
  const line = (chargeId: string, totalCents: number) => ({
    chargeId, description: chargeId, dueDate: "2026-10-10", owedCents: totalCents, surchargeCents: 0,
    discountCents: 0, scholarshipCents: 0, totalCents, due: true, inBalance: true,
  });
  const expected = { amountCents: 350000, lines: [line("sep", 350000), line("oct", 320000)] };

  it("is every line listed when the counter pays by its own rule", () => {
    expect(maxPayableCents(expected, 0, false)).toBe(670000);
  });

  it("is what the chosen concepts add up to when the cashier chose", () => {
    expect(maxPayableCents(expected, 320000, true)).toBe(320000);
  });
});

// Review of PR #144 (N7): another cashier took the money first and the server
// answered 409 (CC-202). The counter says why and reads the account again by
// itself, so the cashier sees what is left without closing the drawer.
describe("CollectDrawer · cobrado por otra caja (CC-202)", () => {
  it("reloads the account and the quote after a 409, and says why", async () => {
    const user = userEvent.setup();
    const paid = "lo elegido ya está pagado: actualiza la cuenta del alumno antes de cobrar";
    finance.registerDeposit.mockRejectedValue(new HttpError(409, paid, { serverText: paid }));
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} today="2026-10-12" />);
    await screen.findByTestId("amount-expected");
    expect(finance.getExpected).toHaveBeenCalledTimes(1);
    expect(finance.getAccount).toHaveBeenCalledTimes(1);

    await user.click(screen.getByRole("button", { name: "Cobrar" }));

    expect(await screen.findByRole("group", { name: /Revisa esto/ })).toHaveTextContent(paid);
    await waitFor(() => expect(finance.getExpected).toHaveBeenCalledTimes(2));
    expect(finance.getAccount).toHaveBeenCalledTimes(2);
  });

  // CC-402 (browser check of PR #144): after the 409 the reload chose ANOTHER
  // charge (the inscription), filled its amount and left focus on Cobrar — one
  // more click collected what the family did not come to pay. After a conflict
  // nothing is chosen or filled again, and focus goes to the notice at the top.
  it("after a 409 chooses nothing, fills nothing, and takes focus to the notice", async () => {
    const user = userEvent.setup();
    const paid = "lo elegido ya está pagado: actualiza la cuenta del alumno antes de cobrar";
    const line = (chargeId: string, description: string, totalCents: number) => ({
      chargeId, description, dueDate: "2026-10-10", owedCents: totalCents, surchargeCents: 0, discountCents: 0,
      scholarshipCents: 0, totalCents, due: true, inBalance: true,
    });
    finance.getExpected
      .mockResolvedValueOnce({ amountCents: 25000, surchargeCents: 0, discountCents: 0, annualBalanceCents: 475000,
        onTimeBalanceCents: 475000, lines: [line("ch-nat", "Natación", 25000), line("ch-ins", "Inscripción", 450000)].map((l, i) => ({ ...l, due: i === 0 })) })
      .mockResolvedValue({ amountCents: 450000, surchargeCents: 0, discountCents: 0, annualBalanceCents: 450000,
        onTimeBalanceCents: 450000, lines: [line("ch-ins", "Inscripción", 450000)] });
    finance.registerDeposit.mockRejectedValue(new HttpError(409, paid, { serverText: paid }));
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} today="2026-10-12" />);
    expect(await screen.findByRole("checkbox", { name: "Natación" })).toBeChecked();

    await user.click(screen.getByRole("button", { name: "Cobrar" }));

    const notice = await screen.findByRole("group", { name: /Revisa esto/ });
    expect(notice).toHaveTextContent(paid);
    await waitFor(() => expect(finance.getExpected).toHaveBeenCalledTimes(2));
    const inscription = await screen.findByRole("checkbox", { name: "Inscripción" });
    expect(inscription).not.toBeChecked();
    expect(screen.getByLabelText("Monto a pagar", { selector: "input" })).toHaveValue("");
    expect(document.activeElement).toBe(notice);

    // One more click on Cobrar takes nothing.
    await user.click(screen.getByRole("button", { name: "Cobrar" }));
    expect(finance.registerDeposit).toHaveBeenCalledTimes(1);
  });

  it("does not reload on any other failure", async () => {
    const user = userEvent.setup();
    finance.registerDeposit.mockRejectedValue(new HttpError(500, "HTTP 500"));
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} today="2026-10-12" />);
    await screen.findByTestId("amount-expected");

    await user.click(screen.getByRole("button", { name: "Cobrar" }));

    expect(await screen.findByText("HTTP 500")).toBeInTheDocument();
    expect(finance.getExpected).toHaveBeenCalledTimes(1);
  });
});

// CC-214: "Del año debe $35,000" and a cap of $33,400 on the same screen read
// as two debts. The cap is what may be taken today: the months not yet due
// carry their early-payment discount. The screen says so, before and after.
describe("CollectDrawer · lo cobrable hoy frente a lo del año (CC-214)", () => {
  const line = (id: string, cents: number): financeApi.ExpectedLine => ({
    chargeId: id, description: id, dueDate: "2026-11-10", owedCents: cents, surchargeCents: 0, discountCents: 0, scholarshipCents: 0, totalCents: cents, due: false, inBalance: true,
  });

  beforeEach(() => {
    finance.getExpected.mockResolvedValue({
      amountCents: 0, surchargeCents: 0, discountCents: 0, annualBalanceCents: 3500000, onTimeBalanceCents: 3340000,
      lines: [line("Colegiatura noviembre", 1670000), line("Colegiatura diciembre", 1670000)],
    });
  });

  it("names both figures in the error and says why they differ", async () => {
    const user = userEvent.setup();
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} today="2026-10-17" />);
    await screen.findByRole("checkbox", { name: "Colegiatura noviembre" });
    const amount = screen.getByLabelText(/Monto a pagar/, { selector: "input" });
    await user.clear(amount);
    await user.type(amount, "35000");
    await user.click(screen.getByRole("button", { name: "Cobrar" }));

    expect(await screen.findAllByText(
      "No puede ser mayor que lo que se puede cobrar hoy ($33,400.00). Es menos que lo que debe del año ($35,000.00) porque los meses que aún no vencen llevan su descuento por pronto pago.",
    )).not.toHaveLength(0);
    expect(finance.registerDeposit).not.toHaveBeenCalled();
  });

  it("says the cap beside the field before anyone goes over it", async () => {
    render(<CollectDrawer student={student} onClose={() => {}} onCollected={() => {}} today="2026-10-17" />);
    expect(await screen.findByText(/Hoy se puede cobrar hasta \$33,400\.00: los meses que aún no vencen llevan su descuento por pronto pago\./)).toBeInTheDocument();
  });
});
