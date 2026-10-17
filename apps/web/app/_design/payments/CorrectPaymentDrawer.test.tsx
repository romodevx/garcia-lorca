import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as financeApi from "../../finance/api";
import { CorrectPaymentDrawer } from "./CorrectPaymentDrawer";

vi.mock("../../finance/api", () => ({
  listDeposits: vi.fn(),
  getAccount: vi.fn(),
  listDepositCorrections: vi.fn(),
  correctDeposit: vi.fn(),
}));
const api = vi.mocked(financeApi);

const DEP = "62000000-0000-0000-0000-000000000001";

const deposit: financeApi.Deposit = {
  id: DEP, studentId: "st-1", amountCents: 50000, method: "cash", reference: "",
  paidAt: "2026-09-09T00:00:00Z", calendarVersion: "", folio: "P-2026-000001",
  tenders: [{ method: "cash", amountCents: 50000 }],
};

const charge = (id: string, description: string, remaining: number): financeApi.Charge => ({
  id, studentId: "st-1", cycleId: "c", conceptId: "k", description, amountCents: 320000, discountCents: 0,
  dueDate: "2026-09-10", status: remaining === 0 ? "paid" : "partial", paidCents: 320000 - remaining, remainingCents: remaining,
});

const correction: financeApi.DepositCorrection = {
  id: "c-1", originalDepositId: "61000000-0000-0000-0000-000000000009", correctedDepositId: DEP,
  reason: "Fecha equivocada", correctedByName: "Directora Ruiz", correctedAt: "2026-09-10T15:00:00Z",
  before: { depositId: "x", folio: "P-2026-000000", amountCents: 50000, paidAt: "2026-09-08T00:00:00Z", tenders: [{ method: "transfer", amountCents: 50000 }], lines: [] },
  after: { depositId: DEP, folio: "P-2026-000001", amountCents: 50000, paidAt: "2026-09-09T00:00:00Z", tenders: [{ method: "cash", amountCents: 50000 }], lines: [] },
};

beforeEach(() => {
  api.listDeposits.mockResolvedValue([deposit]);
  api.getAccount.mockResolvedValue({
    charges: [charge("ch-sep", "Colegiatura septiembre", 270000), charge("ch-oct", "Colegiatura octubre", 320000), charge("ch-ins", "Inscripción", 0)],
    payments: [{ id: "p1", chargeId: "ch-sep", amountCents: 50000, method: "cash", reference: "", paidAt: deposit.paidAt, depositId: DEP }],
    totalBalanceCents: 0, unallocatedCents: 0, dueTodayCents: 0, surchargeCents: 0, earlyDiscountCents: 0, asOf: "",
  } as financeApi.AccountStatement);
  api.listDepositCorrections.mockResolvedValue([correction]);
  api.correctDeposit.mockResolvedValue({
    original: { ...deposit, supersededBy: "dep-2", supersededByFolio: "P-2026-000002" },
    deposit: { ...deposit, id: "62000000-0000-0000-0000-000000000002", amountCents: 100000, folio: "P-2026-000002", correctsFolio: "P-2026-000001" },
    allocations: [], unallocatedCents: 0, replayed: false, correction,
  });
});
afterEach(() => vi.clearAllMocks());

async function open(): Promise<HTMLElement> {
  render(<CorrectPaymentDrawer depositId={DEP} studentId="st-1" today="2026-10-01" onClose={() => {}} />);
  const dialog = await screen.findByRole("dialog", { name: /Corregir pago P-2026-000001/ });
  await within(dialog).findByLabelText(/Monto recibido/);
  return dialog;
}

describe("CorrectPaymentDrawer (T9)", () => {
  it("opens with the payment as it was recorded: amount, day, method and what it paid", async () => {
    const dialog = await open();

    expect(within(dialog).getByLabelText(/Monto recibido/)).toHaveValue("500.00");
    expect(within(dialog).getByLabelText(/Fecha de pago/)).toHaveValue("2026-09-09");
    expect(within(dialog).getByRole("checkbox", { name: /Colegiatura septiembre/ })).toBeChecked();
    expect(within(dialog).getByRole("checkbox", { name: /Colegiatura octubre/ })).not.toBeChecked();
    // A month already paid by other money is not offered.
    expect(within(dialog).queryByRole("checkbox", { name: /Inscripción/ })).toBeNull();
  });

  it("will not save without a reason, and says so where the reason goes", async () => {
    const user = userEvent.setup();
    const dialog = await open();

    await user.click(within(dialog).getByRole("button", { name: "Guardar corrección" }));

    expect(api.correctDeposit).not.toHaveBeenCalled();
    const reason = within(dialog).getByRole("textbox", { name: /Motivo de la corrección/ });
    expect(reason).toHaveAttribute("aria-invalid", "true");
    await waitFor(() => expect(reason).toHaveFocus());
  });

  it("saves the corrected amount with the reason, and offers the new receipt", async () => {
    const user = userEvent.setup();
    render(<CorrectPaymentDrawer depositId={DEP} studentId="st-1" today="2026-10-01" onClose={() => {}} />);
    const dialog = await screen.findByRole("dialog", { name: /Corregir pago P-2026-000001/ });
    const amount = await within(dialog).findByLabelText(/Monto recibido/);

    await user.clear(amount);
    await user.type(amount, "1000");
    await user.type(within(dialog).getByRole("textbox", { name: /Motivo de la corrección/ }), "Se capturó $500 y fueron $1,000");
    await user.click(within(dialog).getByRole("button", { name: "Guardar corrección" }));

    await waitFor(() => expect(api.correctDeposit).toHaveBeenCalledTimes(1));
    const [, id, input] = api.correctDeposit.mock.calls[0] ?? [];
    expect(id).toBe(DEP);
    expect(input).toMatchObject({
      // The day was not touched: the original's instant is kept (review T9).
      amountCents: 100000, paidAt: "", reason: "Se capturó $500 y fueron $1,000",
      tenders: [{ method: "cash", amountCents: 100000 }], chargeIds: ["ch-sep"],
    });
    expect(input?.idempotencyKey).not.toBe("");
    // The form gives way to the new receipt, in the same drawer.
    const saved = await screen.findByRole("dialog", { name: "Pago corregido" });
    expect(within(saved).getAllByText(/P-2026-000002/).length).toBeGreaterThan(0);
    expect(within(saved).getByRole("button", { name: /Reimprimir comprobante/ })).toBeInTheDocument();
  });

  it("shows the history: before, after, who, when and why", async () => {
    const user = userEvent.setup();
    const dialog = await open();

    await user.click(within(dialog).getByText(/Historial de cambios \(1\)/));

    const entry = within(dialog).getByRole("listitem");
    expect(within(entry).getByText(/Fecha equivocada/)).toBeInTheDocument();
    expect(within(entry).getByText(/Directora Ruiz/)).toBeInTheDocument();
    // Before: by transfer, on the 8th; after: cash on the 9th.
    expect(within(entry).getByText(/Antes:/).parentElement).toHaveTextContent(/P-2026-000000.*Transferencia/);
    expect(within(entry).getByText(/Después:/).parentElement).toHaveTextContent(/P-2026-000001.*Efectivo/);
  });

  it("shows why the server refused, and keeps the form", async () => {
    const user = userEvent.setup();
    api.correctDeposit.mockRejectedValue(new Error("este pago ya fue corregido"));
    const dialog = await open();

    await user.type(within(dialog).getByRole("textbox", { name: /Motivo de la corrección/ }), "x");
    await user.click(within(dialog).getByRole("button", { name: "Guardar corrección" }));

    expect(await within(dialog).findByRole("alert")).toHaveTextContent("este pago ya fue corregido");
    expect(within(dialog).getByLabelText(/Monto recibido/)).toBeInTheDocument();
  });

  it("a receipt already corrected is not offered for correction again", async () => {
    api.listDeposits.mockResolvedValue([{ ...deposit, supersededBy: "d2", supersededByFolio: "P-2026-000009" }]);
    render(<CorrectPaymentDrawer depositId={DEP} studentId="st-1" today="2026-10-01" onClose={() => {}} />);
    const dialog = await screen.findByRole("dialog", { name: /Corregir pago P-2026-000001/ });

    expect(await within(dialog).findByText(/ya fue corregido por P-2026-000009/)).toBeInTheDocument();
    expect(within(dialog).queryByRole("button", { name: "Guardar corrección" })).toBeNull();
  });

  it("sends the day only when it was changed", async () => {
    const user = userEvent.setup();
    const dialog = await open();
    const day = within(dialog).getByLabelText(/Fecha de pago/);

    await user.clear(day);
    await user.type(day, "2026-09-05");
    await user.type(within(dialog).getByRole("textbox", { name: /Motivo de la corrección/ }), "fecha");
    await user.click(within(dialog).getByRole("button", { name: "Guardar corrección" }));

    await waitFor(() => expect(api.correctDeposit).toHaveBeenCalledTimes(1));
    expect(api.correctDeposit.mock.calls[0]?.[2]).toMatchObject({ paidAt: "2026-09-05" });
  });

  // Review T9: a payment taken after 18:00 local is stored on the next UTC
  // day; opening it the same evening must not call its own day "future".
  it("does not refuse the recorded day as future", async () => {
    const user = userEvent.setup();
    api.listDeposits.mockResolvedValue([{ ...deposit, paidAt: "2026-10-02T01:30:00Z" }]);
    const dialog = await open();

    await user.type(within(dialog).getByRole("textbox", { name: /Motivo de la corrección/ }), "monto");
    await user.click(within(dialog).getByRole("button", { name: "Guardar corrección" }));

    await waitFor(() => expect(api.correctDeposit).toHaveBeenCalledTimes(1));
  });

  // ADR 0035: the recorded day is the deposit's school-local day (paidOn), not
  // the UTC date of its instant — 19:30 on 1 Oct in Morelia is 2 Oct UTC.
  it("opens on the deposit's own day", async () => {
    api.listDeposits.mockResolvedValue([{ ...deposit, paidAt: "2026-10-02T01:30:00Z", paidOn: "2026-10-01" }]);
    const dialog = await open();
    expect(within(dialog).getByLabelText(/Fecha de pago/)).toHaveValue("2026-10-01");
  });

  // T12 (from T7's review): the amount opened as "1850.00" and only took its
  // thousands separator once the cursor left it.
  it("opens the amount with its thousands separator", async () => {
    api.listDeposits.mockResolvedValue([{ ...deposit, amountCents: 185000, tenders: [{ method: "cash", amountCents: 185000 }] }]);
    const dialog = await open();

    expect(within(dialog).getByLabelText(/Monto recibido/)).toHaveValue("1,850.00");
  });
});
