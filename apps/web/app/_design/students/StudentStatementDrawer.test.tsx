import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as statementApi from "../../finance/statement";
import { StudentStatementDrawer } from "./StudentStatementDrawer";

vi.mock("../payments/CorrectPaymentDrawer", () => ({
  CorrectPaymentDrawer: ({ depositId, onClose }: { depositId: string; onClose: () => void }) => (
    <div role="dialog" aria-label={`Corregir ${depositId}`}><button type="button" onClick={onClose}>Cerrar corrección</button></div>
  ),
}));

vi.mock("../../finance/statement", async (importOriginal) => ({
  ...(await importOriginal<typeof statementApi>()),
  getStatement: vi.fn(),
  downloadStatementPdf: vi.fn(),
}));

const api = vi.mocked(statementApi);

const ledger: statementApi.Statement = {
  movements: [
    { kind: "charge", date: "2026-09-10", description: "Colegiatura septiembre", amountCents: 320000, balanceCents: 320000, accrued: false, notYetDue: false, voided: false },
    { kind: "discount", date: "2026-09-10", description: "Pronto pago · Colegiatura septiembre", amountCents: 10000, balanceCents: 310000, accrued: false, notYetDue: false, voided: false },
    { kind: "payment", date: "2026-09-12", description: "Pago", amountCents: 100000, balanceCents: 210000, method: "card", cardType: "debit", folio: "P-2026-000042", accrued: false, notYetDue: false, voided: false },
    { kind: "payment", date: "2026-09-13", description: "Pago", amountCents: 50000, balanceCents: 210000, method: "cash", accrued: false, notYetDue: false, voided: true, voidReason: "dos veces" },
    { kind: "surcharge", date: "2026-10-01", description: "Recargo por atraso · Colegiatura septiembre", amountCents: 20000, balanceCents: 230000, accrued: true, notYetDue: false, voided: false },
    { kind: "charge", date: "2027-01-10", description: "Inscripción 2027", amountCents: 50000, balanceCents: 230000, accrued: false, notYetDue: true, voided: false },
  ],
  totals: { chargedCents: 340000, discountCents: 10000, paidCents: 100000, balanceCents: 230000, notYetDueCents: 50000 },
  dueTodayCents: 230000, surchargeCents: 20000, earlyDiscountCents: 0, unallocatedCents: 0, asOf: "2026-10-01",
};

beforeEach(() => {
  api.getStatement.mockResolvedValue(ledger);
  api.downloadStatementPdf.mockResolvedValue();
});
afterEach(() => vi.clearAllMocks());

function row(rows: HTMLElement[], i: number): HTMLElement {
  const found = rows[i];
  if (found === undefined) throw new Error(`no row ${i}`);
  return found;
}

function money(text: string): RegExp {
  return new RegExp(text.replace(/[$.,]/g, (c) => `\\${c}`));
}

describe("StudentStatementDrawer (T7)", () => {
  it("shows every movement with its running balance, and the totals", async () => {
    render(<StudentStatementDrawer studentId="st-1" name="Cruz Valentina" onClose={() => {}} />);
    const dialog = await screen.findByRole("dialog", { name: "Estado de cuenta · Cruz Valentina" });

    expect(await within(dialog).findByText("Colegiatura septiembre")).toBeInTheDocument();
    expect(api.getStatement).toHaveBeenCalledWith(expect.any(String), "st-1");
    const rows = within(dialog).getAllByRole("row");
    // Head + six movements.
    expect(rows).toHaveLength(7);
    // The charge and the running balance after it.
    expect(within(row(rows, 1)).getAllByText(money("$3,200.00"))).toHaveLength(2);
    expect(within(row(rows, 3)).getByText(money("$2,100.00"))).toBeInTheDocument();
    expect(within(row(rows, 3)).getByText(/P-2026-000042/)).toBeInTheDocument();
    expect(within(row(rows, 3)).getByText(/Tarjeta de débito/)).toBeInTheDocument();

    expect(within(dialog).getByText("Total cargado").nextSibling).toHaveTextContent(money("$3,400.00"));
    expect(within(dialog).getByText("Total pagado").nextSibling).toHaveTextContent(money("$1,000.00"));
    expect(within(dialog).getByText("Saldo pendiente").nextSibling).toHaveTextContent(money("$2,300.00"));
    expect(within(dialog).getByText(/A pagar hoy/).nextSibling).toHaveTextContent(money("$2,300.00"));
  });

  it("strikes a voided deposit through and marks what is not yet due and what is accruing", async () => {
    render(<StudentStatementDrawer studentId="st-1" name="Cruz Valentina" onClose={() => {}} />);
    const dialog = await screen.findByRole("dialog", { name: "Estado de cuenta · Cruz Valentina" });
    await within(dialog).findByText("Colegiatura septiembre");
    const rows = within(dialog).getAllByRole("row");

    expect(within(row(rows, 4)).getByText(/Cancelado: dos veces/)).toBeInTheDocument();
    expect(row(rows, 4)).toHaveStyle({ textDecoration: "line-through" });
    expect(within(row(rows, 5)).getByText("Acumulado a hoy")).toBeInTheDocument();
    expect(within(row(rows, 6)).getByText("Aún no exigible")).toBeInTheDocument();
    expect(within(dialog).getByText(/Aún no exigible \(fuera del saldo\)/)).toBeInTheDocument();
  });

  it("downloads the PDF", async () => {
    const user = userEvent.setup();
    render(<StudentStatementDrawer studentId="st-1" name="Cruz Valentina" onClose={() => {}} />);
    const dialog = await screen.findByRole("dialog", { name: "Estado de cuenta · Cruz Valentina" });
    await within(dialog).findByText("Colegiatura septiembre");

    await user.click(within(dialog).getByRole("button", { name: "Descargar PDF" }));

    await waitFor(() => expect(api.downloadStatementPdf).toHaveBeenCalledWith(expect.any(String), "st-1"));
  });

  it("says when the statement cannot be read, and retries", async () => {
    const user = userEvent.setup();
    api.getStatement.mockRejectedValueOnce(new statementApi.StatementError(500, "No se pudo leer el estado de cuenta."));
    render(<StudentStatementDrawer studentId="st-1" name="Cruz Valentina" onClose={() => {}} />);
    const dialog = await screen.findByRole("dialog", { name: "Estado de cuenta · Cruz Valentina" });

    expect(await within(dialog).findByText("No se pudo leer el estado de cuenta.")).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Reintentar" }));
    expect(await within(dialog).findByText("Colegiatura septiembre")).toBeInTheDocument();
  });

  // T5: below zero the school owes the family. Said as credit, with a positive
  // amount — in the totals and in the running balance — never as "-$500.00".
  it("names a balance below zero as credit, never with a minus sign", async () => {
    api.getStatement.mockResolvedValue({
      ...ledger,
      movements: [{ kind: "payment", date: "2026-09-12", description: "Pago", amountCents: 50000, balanceCents: -50000, method: "cash", accrued: false, notYetDue: false, voided: false }],
      totals: { chargedCents: 0, discountCents: 0, paidCents: 50000, balanceCents: -50000, notYetDueCents: 0 },
      dueTodayCents: 0,
    });
    render(<StudentStatementDrawer studentId="st-1" name="Cruz Valentina" onClose={() => {}} />);
    const dialog = await screen.findByRole("dialog", { name: "Estado de cuenta · Cruz Valentina" });

    expect((await within(dialog).findByText("Saldo a favor")).nextSibling).toHaveTextContent(money("$500.00"));
    expect(within(dialog).getByTestId("running-balance-0")).toHaveTextContent(/^A favor \$500\.00$/);
    expect(dialog).not.toHaveTextContent(/-\s?\$/);
    expect(within(dialog).queryByText("Saldo pendiente")).toBeNull();
  });

  it("says a pupil with no movements has none", async () => {
    api.getStatement.mockResolvedValue({ ...ledger, movements: [], totals: { chargedCents: 0, discountCents: 0, paidCents: 0, balanceCents: 0, notYetDueCents: 0 } });
    render(<StudentStatementDrawer studentId="st-1" name="Cruz Valentina" onClose={() => {}} />);
    const dialog = await screen.findByRole("dialog", { name: "Estado de cuenta · Cruz Valentina" });

    expect(await within(dialog).findByText("Sin movimientos")).toBeInTheDocument();
  });

  // T8: a credit note reaches the statement as its own kind, with its method in words.
  it("names a credit note's method in words", async () => {
    api.getStatement.mockResolvedValue({
      ...ledger,
      movements: [{ kind: "credit_note", date: "2026-10-12", description: "Nota de crédito", amountCents: 50000, balanceCents: 0, method: "credit_note", reference: "NC-7", folio: "P-2026-000050", accrued: false, notYetDue: false, voided: false }],
    });
    render(<StudentStatementDrawer studentId="st-1" name="Cruz Valentina" onClose={() => {}} />);
    const dialog = await screen.findByRole("dialog", { name: "Estado de cuenta · Cruz Valentina" });

    const detail = await within(dialog).findByText(/P-2026-000050/);
    expect(detail).toHaveTextContent("P-2026-000050 · Nota de crédito · ref. NC-7");
    expect(detail).not.toHaveTextContent("credit_note");
  });

  // T9: the original of a correction stays, struck through and named by its
  // replacement; the correction names what it corrects.
  it("shows a corrected payment and its replacement", async () => {
    api.getStatement.mockResolvedValue({
      ...ledger,
      movements: [
        { kind: "payment", date: "2026-10-10", description: "Pago", amountCents: 50000, balanceCents: 0, depositId: "d1", folio: "P-2026-000001", accrued: false, notYetDue: false, voided: false, superseded: true, supersededByFolio: "P-2026-000002" },
        { kind: "payment", date: "2026-10-10", description: "Pago", amountCents: 100000, balanceCents: 0, depositId: "d2", folio: "P-2026-000002", correctsFolio: "P-2026-000001", accrued: false, notYetDue: false, voided: false },
      ],
    });
    render(<StudentStatementDrawer studentId="st-1" name="Cruz Valentina" onClose={() => {}} />);
    const dialog = await screen.findByRole("dialog", { name: "Estado de cuenta · Cruz Valentina" });
    await within(dialog).findByText(/Sustituido por P-2026-000002/);
    const rows = within(dialog).getAllByRole("row");

    expect(row(rows, 1)).toHaveStyle({ textDecoration: "line-through" });
    expect(within(row(rows, 2)).getByText(/Corrige a P-2026-000001/)).toBeInTheDocument();
    // Nobody here may correct: no button.
    expect(within(dialog).queryByRole("button", { name: /Corregir pago/ })).toBeNull();
  });

  it("offers to correct each live payment once, and comes back to the statement", async () => {
    const user = userEvent.setup();
    api.getStatement.mockResolvedValue({
      ...ledger,
      movements: [
        { kind: "credit_note", date: "2026-10-10", description: "Nota de crédito", amountCents: 20000, balanceCents: 0, depositId: "d1", folio: "P-2026-000001", accrued: false, notYetDue: false, voided: false },
        { kind: "payment", date: "2026-10-10", description: "Pago", amountCents: 30000, balanceCents: 0, depositId: "d1", folio: "P-2026-000001", accrued: false, notYetDue: false, voided: false },
        { kind: "payment", date: "2026-10-09", description: "Pago", amountCents: 30000, balanceCents: 0, depositId: "d0", folio: "P-2026-000000", accrued: false, notYetDue: false, voided: false, superseded: true, supersededByFolio: "P-2026-000001" },
      ],
    });
    render(<StudentStatementDrawer studentId="st-1" name="Cruz Valentina" canCorrect onClose={() => {}} />);
    const dialog = await screen.findByRole("dialog", { name: "Estado de cuenta · Cruz Valentina" });
    await within(dialog).findAllByText(/P-2026-000001/);

    const buttons = within(dialog).getAllByRole("button", { name: /Corregir pago/ });
    expect(buttons).toHaveLength(1);
    await user.click(buttons[0] as HTMLElement);

    // One dialog at a time: the statement gives way to the correction.
    expect(await screen.findByRole("dialog", { name: "Corregir d1" })).toBeInTheDocument();
    expect(screen.queryByRole("dialog", { name: "Estado de cuenta · Cruz Valentina" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Cerrar corrección" }));
    expect(await screen.findByRole("dialog", { name: "Estado de cuenta · Cruz Valentina" })).toBeInTheDocument();
    expect(api.getStatement).toHaveBeenCalledTimes(2);
  });
});

describe("StudentStatementDrawer · scholarships (T18)", () => {
  it("shows a scholarship as a credit line labelled with its description", async () => {
    api.getStatement.mockResolvedValue({
      ...ledger,
      movements: [
        { kind: "charge", date: "2026-09-10", description: "Colegiatura septiembre", amountCents: 320000, balanceCents: 320000, accrued: false, notYetDue: false, voided: false },
        { kind: "scholarship", date: "2026-09-10", description: "Beca Hermanos · Colegiatura septiembre", amountCents: 160000, balanceCents: 160000, chargeId: "ch1", accrued: false, notYetDue: false, voided: false },
      ],
      totals: { chargedCents: 320000, discountCents: 160000, paidCents: 0, balanceCents: 160000, notYetDueCents: 0 },
    });
    render(<StudentStatementDrawer studentId="st-1" name="Cruz Valentina" onClose={() => {}} />);
    const dialog = await screen.findByRole("dialog", { name: "Estado de cuenta · Cruz Valentina" });

    const line = (await within(dialog).findByText("Beca Hermanos · Colegiatura septiembre")).closest("tr") as HTMLElement;
    const cells = within(line).getAllByRole("cell");
    // Fecha · Concepto · Cargo · Abono · Saldo: the scholarship lowers the balance.
    expect(cells[2]?.textContent).toBe("");
    expect(cells[3]).toHaveTextContent(money("$1,600.00"));
    expect(within(dialog).getByText("Descuentos y becas").nextSibling).toHaveTextContent(money("$1,600.00"));
  });
});
