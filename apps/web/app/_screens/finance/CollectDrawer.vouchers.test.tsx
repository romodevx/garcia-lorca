import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as financeApi from "../../finance/api";
import * as vouchersApi from "../../finance/vouchers";
import { pesosToCents } from "../../_lib/money";
import { CollectDrawer } from "./CollectDrawer";

vi.mock("../../finance/api", async (importOriginal) => {
  const real = await importOriginal<typeof financeApi>();
  return {
    FinanceError: real.FinanceError,
    getAccount: vi.fn(),
    getExpected: vi.fn(),
    previewDeposit: vi.fn(),
    registerDeposit: vi.fn(),
    listDeposits: vi.fn(),
  };
});
vi.mock("../../finance/vouchers", () => ({
  listPendingVouchers: vi.fn(),
  getVoucherFile: vi.fn(),
  rejectVoucher: vi.fn(),
  VOUCHER_CODES: { alreadyApplied: "VOUCHER_ALREADY_APPLIED", rejected: "VOUCHER_REJECTED", choiceRequired: "VOUCHER_CHOICE_REQUIRED", limit: "VOUCHER_LIMIT" },
}));

const finance = vi.mocked(financeApi);
const vouchers = vi.mocked(vouchersApi);

const TODAY = "2026-10-21";
const student = { id: "s1", firstName: "Valentina", lastName: "Cruz", gradeGroup: "1° A" };

const voucher = (id: string, extra: Partial<vouchersApi.PaymentVoucher> = {}): vouchersApi.PaymentVoucher => ({
  id, studentId: "s1", status: "pending", fileContentType: "image/jpeg", fileSizeBytes: 4000,
  uploadedByName: "Rosa Pérez", createdAt: "2026-10-20T18:04:00Z", ...extra,
});

const deposit = (extra: Partial<financeApi.Deposit> = {}): financeApi.Deposit => ({
  id: "dep-1", studentId: "s1", amountCents: 320000, method: "transfer", reference: "", paidAt: "2026-10-21T16:00:00Z",
  paidOn: "2026-10-21", tenders: [], calendarVersion: "abc", folio: "P-2026-000042", ...extra,
});

const result = (extra: Partial<financeApi.DepositResult> = {}): financeApi.DepositResult => ({
  deposit: deposit(), allocations: [], surcharges: [], unallocatedCents: 0, replayed: false, ...extra,
});

function conflict(code: string, sentence: string): financeApi.FinanceError {
  return new financeApi.FinanceError(409, sentence, { serverText: sentence, errorCode: code });
}

function sent(): financeApi.RegisterDepositInput {
  const call = finance.registerDeposit.mock.calls.at(-1);
  if (call === undefined) throw new Error("registerDeposit was not called");
  return call[1];
}

beforeEach(() => {
  finance.getAccount.mockResolvedValue({ charges: [], payments: [], totalBalanceCents: 640000, unallocatedCents: 0, dueTodayCents: 0, surchargeCents: 0, earlyDiscountCents: 0, asOf: "" });
  finance.getExpected.mockResolvedValue({ amountCents: 670000, surchargeCents: 0, discountCents: 0, annualBalanceCents: 640000, onTimeBalanceCents: 640000, lines: [] });
  finance.previewDeposit.mockResolvedValue({ allocations: [], surcharges: [], unallocatedCents: 0 });
  finance.listDeposits.mockResolvedValue([]);
  vouchers.listPendingVouchers.mockResolvedValue([]);
  vouchers.getVoucherFile.mockResolvedValue(new Blob(["x"], { type: "image/jpeg" }));
  globalThis.URL.createObjectURL = vi.fn(() => "blob:voucher");
  globalThis.URL.revokeObjectURL = vi.fn();
});
afterEach(() => vi.clearAllMocks());

describe("CollectDrawer · cobrar por comprobante (ADR 0040)", () => {
  const claimed = voucher("v1", { claimedAmountCents: 320000, claimedPaidOn: "2026-10-20" });

  function open(v: vouchersApi.PaymentVoucher = claimed, extra: { onChanged?: () => void; onRejected?: () => void; onCollected?: () => void } = {}): void {
    render(
      <CollectDrawer student={student} today={TODAY} onClose={() => {}} onCollected={extra.onCollected ?? (() => {})}
        vouchers={{ byVoucher: v, onChanged: extra.onChanged ?? (() => {}), onRejected: extra.onRejected }} />,
    );
  }

  it("previews the image the family sent, read with the session, and says who sent it and what they claim", async () => {
    open();

    const panel = await screen.findByRole("region", { name: /Comprobante de la familia/ });
    const img = await within(panel).findByRole("img", { name: /Comprobante enviado por Rosa Pérez/ });
    expect(img).toHaveAttribute("src", "blob:voucher");
    expect(vouchers.getVoucherFile).toHaveBeenCalledWith(expect.any(String), claimed);
    expect(panel).toHaveTextContent("Rosa Pérez");
    expect(panel).toHaveTextContent(/Dice que pagó \$3,200\.00 el 20 oct/);
  });

  it("previews a PDF in place, never as a page of the app", async () => {
    vouchers.getVoucherFile.mockResolvedValue(new Blob(["%PDF"], { type: "application/pdf" }));
    open(voucher("v1", { fileContentType: "application/pdf" }));

    const pdf = await screen.findByTitle(/Comprobante enviado por Rosa Pérez/);
    expect(pdf.tagName).toBe("OBJECT");
    expect(pdf).toHaveAttribute("type", "application/pdf");
    expect(pdf).toHaveAttribute("data", "blob:voucher");
  });

  it("prefills the amount and the day the family claimed, paid by transfer", async () => {
    open();
    await screen.findByTestId("amount-expected");
    await waitFor(() => expect(finance.getExpected).toHaveBeenCalled());

    expect(pesosToCents((screen.getByLabelText("Monto a pagar") as HTMLInputElement).value)).toBe(320000);
    expect(screen.getByLabelText("Fecha de pago")).toHaveValue("2026-10-20");
    const tender = screen.getByRole("group", { name: "Forma de pago 1" });
    expect(within(tender).getByRole("button", { name: "Transferencia" })).toHaveAttribute("aria-pressed", "true");
  });

  it("falls back to what is owed when the family gave no amount", async () => {
    open(voucher("v1"));

    await waitFor(() => expect(pesosToCents((screen.getByLabelText("Monto a pagar") as HTMLInputElement).value)).toBe(670000));
  });

  it("registers the payment BY the voucher, with no key of its own", async () => {
    const user = userEvent.setup();
    const onChanged = vi.fn();
    finance.registerDeposit.mockResolvedValue(result({ linkedVoucher: { ...claimed, status: "applied", depositId: "dep-1" } }));
    open(claimed, { onChanged });
    await screen.findByTestId("amount-expected");

    await user.click(screen.getByRole("button", { name: "Registrar pago" }));

    await waitFor(() => expect(finance.registerDeposit).toHaveBeenCalled());
    expect(sent()).toMatchObject({ voucherId: "v1", amountCents: 320000, paidAt: "2026-10-20" });
    expect(sent().idempotencyKey).toBeUndefined();
    expect(sent().tenders?.[0]?.method).toBe("transfer");
    expect(await screen.findByTestId("receipt")).toHaveTextContent(/Comprobante de la familia ligado a este pago/);
    expect(onChanged).toHaveBeenCalled();
  });

  // Guard D: the cashier declined a link earlier, or another cashier already
  // took this money by hand. Said before the voucher is applied.
  it("shows what was already registered for the pupil since the day the family claims", async () => {
    finance.listDeposits.mockResolvedValue([
      deposit(),
      deposit({ id: "old", paidOn: "2026-10-01", folio: "P-2026-000001", amountCents: 100 }),
      deposit({ id: "void", folio: "P-2026-000050", voidedAt: "2026-10-21T17:00:00Z" }),
    ]);
    open();

    const panel = await screen.findByRole("region", { name: /Comprobante de la familia/ });
    expect(await within(panel).findByText(/Ya se registró \$3,200\.00 el 21 oct \(P-2026-000042\)/)).toBeInTheDocument();
    expect(panel).not.toHaveTextContent("P-2026-000001");
    expect(panel).not.toHaveTextContent("P-2026-000050");
    expect(finance.listDeposits).toHaveBeenCalledWith(expect.any(String), "s1");
  });

  it("rejects with a reason, asked for only when it is missing", async () => {
    const user = userEvent.setup();
    const onRejected = vi.fn();
    vouchers.rejectVoucher.mockResolvedValue({ ...claimed, status: "rejected", rejectionReason: "No se lee" });
    open(claimed, { onRejected });
    const panel = await screen.findByRole("region", { name: /Comprobante de la familia/ });

    await user.click(within(panel).getByRole("button", { name: "Rechazar comprobante" }));
    await user.click(within(panel).getByRole("button", { name: "Confirmar rechazo" }));
    expect(await within(panel).findByText("Escribe el motivo: la familia lo lee en la app.")).toBeInTheDocument();
    expect(vouchers.rejectVoucher).not.toHaveBeenCalled();

    await user.type(within(panel).getByRole("textbox", { name: /Motivo del rechazo/ }), "No se lee la referencia");
    await user.click(within(panel).getByRole("button", { name: "Confirmar rechazo" }));

    await waitFor(() => expect(vouchers.rejectVoucher).toHaveBeenCalledWith(expect.any(String), "v1", "No se lee la referencia"));
    expect(onRejected).toHaveBeenCalled();
    expect(finance.registerDeposit).not.toHaveBeenCalled();
  });

  it("Enter in the reason rejects; it never registers the payment", async () => {
    const user = userEvent.setup();
    vouchers.rejectVoucher.mockResolvedValue({ ...claimed, status: "rejected" });
    open();
    const panel = await screen.findByRole("region", { name: /Comprobante de la familia/ });
    await user.click(within(panel).getByRole("button", { name: "Rechazar comprobante" }));

    await user.type(within(panel).getByRole("textbox", { name: /Motivo del rechazo/ }), "Es de otro alumno{Enter}");

    await waitFor(() => expect(vouchers.rejectVoucher).toHaveBeenCalled());
    expect(finance.registerDeposit).not.toHaveBeenCalled();
  });

  it("says, with the folio, that the voucher was already applied", async () => {
    const user = userEvent.setup();
    const onChanged = vi.fn();
    finance.registerDeposit.mockRejectedValue(conflict("VOUCHER_ALREADY_APPLIED", "este comprobante ya se aplicó al pago P-2026-000042"));
    open(claimed, { onChanged });
    await screen.findByTestId("amount-expected");

    await user.click(screen.getByRole("button", { name: "Registrar pago" }));

    expect(await screen.findByText(/Este comprobante ya se aplicó al pago P-2026-000042/)).toBeInTheDocument();
    expect(onChanged).toHaveBeenCalled();
  });

  it("says a rejected voucher pays nothing", async () => {
    const user = userEvent.setup();
    finance.registerDeposit.mockRejectedValue(conflict("VOUCHER_REJECTED", "este comprobante fue rechazado"));
    open();
    await screen.findByTestId("amount-expected");

    await user.click(screen.getByRole("button", { name: "Registrar pago" }));

    expect(await screen.findByText(/Este comprobante ya fue rechazado: no se puede cobrar con él/)).toBeInTheDocument();
  });

  it("asks to open it again when the voucher is no longer there", async () => {
    const user = userEvent.setup();
    finance.registerDeposit.mockRejectedValue(new financeApi.FinanceError(404, "comprobante no encontrado"));
    open();
    await screen.findByTestId("amount-expected");

    await user.click(screen.getByRole("button", { name: "Registrar pago" }));

    expect(await screen.findByText(/Ciérralo y ábrelo de nuevo desde la lista/)).toBeInTheDocument();
  });
});

describe("CollectDrawer · cobro a mano con comprobantes pendientes (ADR 0040)", () => {
  function open(onChanged: () => void = () => {}): void {
    render(<CollectDrawer student={student} today={TODAY} onClose={() => {}} onCollected={() => {}} vouchers={{ onChanged }} />);
  }

  it("says the payment will be linked to the family's only voucher, and sends nothing extra", async () => {
    const user = userEvent.setup();
    vouchers.listPendingVouchers.mockResolvedValue([voucher("v1")]);
    finance.registerDeposit.mockResolvedValue(result());
    open();

    expect(await screen.findByText("Este pago se ligará al comprobante que subió la familia el 20 oct")).toBeInTheDocument();
    expect(vouchers.listPendingVouchers).toHaveBeenCalledWith(expect.any(String), "s1");
    await screen.findByTestId("amount-expected");
    await user.click(screen.getByRole("button", { name: "Cobrar" }));

    await waitFor(() => expect(finance.registerDeposit).toHaveBeenCalled());
    expect(sent().declinedVoucherIds).toBeUndefined();
    expect(sent().voucherId).toBeUndefined();
  });

  it("declines it when the cashier says it is a different payment", async () => {
    const user = userEvent.setup();
    vouchers.listPendingVouchers.mockResolvedValue([voucher("v1")]);
    finance.registerDeposit.mockResolvedValue(result());
    open();
    await screen.findByTestId("amount-expected");

    await user.click(await screen.findByRole("checkbox", { name: /Es otro pago/ }));
    await user.click(screen.getByRole("button", { name: "Cobrar" }));

    await waitFor(() => expect(finance.registerDeposit).toHaveBeenCalled());
    expect(sent().declinedVoucherIds).toEqual(["v1"]);
  });

  it("with two waiting, it does not collect until the cashier says which one", async () => {
    const user = userEvent.setup();
    vouchers.listPendingVouchers.mockResolvedValue([voucher("v1", { claimedAmountCents: 320000 }), voucher("v2", { createdAt: "2026-10-21T15:00:00Z" })]);
    finance.registerDeposit.mockResolvedValue(result());
    open();
    await screen.findByTestId("amount-expected");
    const group = await screen.findByRole("radiogroup", { name: /¿Cuál comprobante es este pago\?/ });

    await user.click(screen.getByRole("button", { name: "Cobrar" }));
    expect(await screen.findByText("Elige cuál comprobante es este pago, o marca que es otro.")).toBeInTheDocument();
    expect(finance.registerDeposit).not.toHaveBeenCalled();

    await user.click(within(group).getByRole("radio", { name: /20 oct.*\$3,200\.00/ }));
    await user.click(screen.getByRole("button", { name: "Cobrar" }));

    await waitFor(() => expect(finance.registerDeposit).toHaveBeenCalled());
    expect(sent().voucherId).toBe("v1");
    expect(sent().idempotencyKey).toBeUndefined();
  });

  it("with two waiting, 'ninguno' declines them all", async () => {
    const user = userEvent.setup();
    vouchers.listPendingVouchers.mockResolvedValue([voucher("v1"), voucher("v2")]);
    finance.registerDeposit.mockResolvedValue(result());
    open();
    await screen.findByTestId("amount-expected");

    await user.click(await screen.findByRole("radio", { name: /Ninguno: es otro pago/ }));
    await user.click(screen.getByRole("button", { name: "Cobrar" }));

    await waitFor(() => expect(finance.registerDeposit).toHaveBeenCalled());
    expect(sent().declinedVoucherIds).toEqual(["v1", "v2"]);
    expect(sent().voucherId).toBeUndefined();
  });

  // The queue was read before a family sent a second voucher: the server says
  // so, nothing was taken, and the counter asks which.
  it("on VOUCHER_CHOICE_REQUIRED, reads the vouchers again and asks which", async () => {
    const user = userEvent.setup();
    const onChanged = vi.fn();
    vouchers.listPendingVouchers.mockResolvedValueOnce([]).mockResolvedValue([voucher("v1"), voucher("v2")]);
    finance.registerDeposit.mockRejectedValue(conflict("VOUCHER_CHOICE_REQUIRED", "elige"));
    open(onChanged);
    await screen.findByTestId("amount-expected");
    await waitFor(() => expect(vouchers.listPendingVouchers).toHaveBeenCalledTimes(1));

    await user.click(screen.getByRole("button", { name: "Cobrar" }));

    expect(await screen.findByRole("radiogroup", { name: /¿Cuál comprobante es este pago\?/ })).toBeInTheDocument();
    expect(screen.getAllByText(/varios comprobantes por revisar/).length).toBeGreaterThan(0);
    // The amount the cashier was about to take is still there.
    expect(pesosToCents((screen.getByLabelText("Monto a pagar") as HTMLInputElement).value)).toBe(670000);
    expect(onChanged).toHaveBeenCalled();
  });

  it("shows the voucher a hand-registered payment linked on its receipt", async () => {
    const user = userEvent.setup();
    vouchers.listPendingVouchers.mockResolvedValue([voucher("v1")]);
    finance.registerDeposit.mockResolvedValue(result({ linkedVoucher: voucher("v1", { status: "applied" }) }));
    open();
    await screen.findByTestId("amount-expected");

    await user.click(screen.getByRole("button", { name: "Cobrar" }));

    expect(await screen.findByTestId("receipt")).toHaveTextContent(/Comprobante de la familia ligado a este pago.*Rosa Pérez/);
  });

  it("without the module, it never asks about vouchers", async () => {
    render(<CollectDrawer student={student} today={TODAY} onClose={() => {}} onCollected={() => {}} />);
    await screen.findByTestId("amount-expected");

    expect(vouchers.listPendingVouchers).not.toHaveBeenCalled();
    expect(screen.queryByText(/comprobante/i)).not.toBeInTheDocument();
  });
});
