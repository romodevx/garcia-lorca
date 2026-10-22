import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as financeApi from "../../finance/api";
import * as studentsApi from "../../students/api";
import * as vouchersApi from "../../finance/vouchers";
import { PaymentsScreen } from "./PaymentsScreen";

vi.mock("../../finance/api", () => ({
  getExpected: vi.fn(),
  getAccount: vi.fn(),
  previewDeposit: vi.fn(),
  registerDeposit: vi.fn(),
  getCashUp: vi.fn(),
  listDeposits: vi.fn(),
}));
vi.mock("../../finance/vouchers", () => ({
  listPendingVouchers: vi.fn(),
  getVoucherFile: vi.fn(),
  rejectVoucher: vi.fn(),
  VOUCHER_CODES: { alreadyApplied: "VOUCHER_ALREADY_APPLIED", rejected: "VOUCHER_REJECTED", choiceRequired: "VOUCHER_CHOICE_REQUIRED", limit: "VOUCHER_LIMIT" },
}));
vi.mock("../../students/api", () => ({ listStudentsWithRooms: vi.fn(), listEnrollments: vi.fn() }));

const finance = vi.mocked(financeApi);
const students = vi.mocked(studentsApi);
const vouchers = vi.mocked(vouchersApi);

const student = (id: string, first: string, last: string): studentsApi.Student => ({
  id, curp: `C${id}`, firstName: first, lastName: last, level: "primaria", levelId: "",
  gradeGroup: "1° A", groupLabel: "1° PRIMARIA · A", guardianName: "Tutor", admissionStatus: "enrolled",
});

const voucher = (id: string, studentId: string, extra: Partial<vouchersApi.PaymentVoucher> = {}): vouchersApi.PaymentVoucher => ({
  id, studentId, status: "pending", fileContentType: "image/jpeg", fileSizeBytes: 4000,
  uploadedByName: "Rosa Pérez", createdAt: "2026-10-20T18:04:00Z", ...extra,
});

beforeEach(() => {
  students.listStudentsWithRooms.mockResolvedValue([student("s1", "Valentina", "Cruz"), student("s2", "Mateo", "Sánchez")]);
  finance.getExpected.mockResolvedValue({ amountCents: 670000, surchargeCents: 0, discountCents: 0, annualBalanceCents: 1300000, onTimeBalanceCents: 1300000, lines: [] });
  finance.getAccount.mockResolvedValue({ charges: [], payments: [], totalBalanceCents: 1300000, unallocatedCents: 0, dueTodayCents: 0, surchargeCents: 0, earlyDiscountCents: 0, asOf: "" });
  finance.previewDeposit.mockResolvedValue({ allocations: [], surcharges: [], unallocatedCents: 0 });
  finance.listDeposits.mockResolvedValue([]);
  vouchers.listPendingVouchers.mockResolvedValue([voucher("v1", "s2", { claimedAmountCents: 320000, claimedPaidOn: "2026-10-20" })]);
  vouchers.getVoucherFile.mockResolvedValue(new Blob(["x"], { type: "image/jpeg" }));
  globalThis.URL.createObjectURL = vi.fn(() => "blob:voucher");
  globalThis.URL.revokeObjectURL = vi.fn();
});
afterEach(() => vi.clearAllMocks());

describe("PaymentsScreen · comprobantes por revisar (ADR 0040)", () => {
  it("puts the pending vouchers above the search, named after the pupil the roll already has", async () => {
    render(<PaymentsScreen onBack={() => {}} paymentVouchers />);

    const queue = await screen.findByRole("region", { name: /Comprobantes por revisar \(1\)/ });
    expect(await within(queue).findByText("Sánchez Mateo")).toBeInTheDocument();
    expect(within(queue).getByText("1° PRIMARIA · A")).toBeInTheDocument();
    expect(within(queue).getByText("$3,200.00")).toBeInTheDocument();
    // FIRST: the queue sits before the search in reading order.
    const search = screen.getByLabelText("Buscar alumno");
    expect(queue.compareDocumentPosition(search) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    // One read for the queue; nothing per row.
    expect(vouchers.listPendingVouchers).toHaveBeenCalledTimes(1);
    expect(vouchers.listPendingVouchers).toHaveBeenCalledWith(expect.any(String));
  });

  it("says when the family gave no amount, instead of showing zero", async () => {
    vouchers.listPendingVouchers.mockResolvedValue([voucher("v1", "s2")]);
    render(<PaymentsScreen onBack={() => {}} paymentVouchers />);

    const queue = await screen.findByRole("region", { name: /Comprobantes por revisar/ });
    expect(await within(queue).findByText("Sin importe")).toBeInTheDocument();
  });

  it("opens the counter for that pupil, on that voucher", async () => {
    const user = userEvent.setup();
    render(<PaymentsScreen onBack={() => {}} paymentVouchers />);

    const queue = await screen.findByRole("region", { name: /Comprobantes por revisar/ });
    await user.click(await within(queue).findByRole("button", { name: /Sánchez Mateo/ }));

    const dialog = await screen.findByRole("dialog", { name: /Cobrar · Sánchez Mateo/ });
    expect(await within(dialog).findByRole("region", { name: /Comprobante de la familia/ })).toBeInTheDocument();
  });

  it("rejecting closes the counter, says so and reads the queue again", async () => {
    const user = userEvent.setup();
    vouchers.rejectVoucher.mockResolvedValue(voucher("v1", "s2", { status: "rejected" }));
    render(<PaymentsScreen onBack={() => {}} paymentVouchers />);
    const queue = await screen.findByRole("region", { name: /Comprobantes por revisar/ });
    await user.click(await within(queue).findByRole("button", { name: /Sánchez Mateo/ }));
    const dialog = await screen.findByRole("dialog", { name: /Cobrar · Sánchez Mateo/ });

    vouchers.listPendingVouchers.mockResolvedValue([]);
    await user.click(within(dialog).getByRole("button", { name: "Rechazar comprobante" }));
    await user.type(within(dialog).getByRole("textbox", { name: /Motivo del rechazo/ }), "No se lee{Enter}");

    expect(await screen.findByText("Comprobante rechazado")).toBeInTheDocument();
    expect(screen.queryByRole("dialog", { name: /Cobrar/ })).not.toBeInTheDocument();
    await waitFor(() => expect(screen.queryByRole("region", { name: /Comprobantes por revisar/ })).not.toBeInTheDocument());
  });

  it("marks a searched pupil who sent a voucher, from the queue already read", async () => {
    const user = userEvent.setup();
    render(<PaymentsScreen onBack={() => {}} paymentVouchers />);
    await screen.findByRole("region", { name: /Comprobantes por revisar/ });

    await user.type(screen.getByLabelText("Buscar alumno"), "Mateo");

    const row = await screen.findByRole("button", { name: /Sánchez Mateo.*Comprobante/ });
    expect(row).toBeInTheDocument();
    expect(vouchers.listPendingVouchers).toHaveBeenCalledTimes(1);
  });

  it("shows nothing of it when no voucher is waiting", async () => {
    vouchers.listPendingVouchers.mockResolvedValue([]);
    render(<PaymentsScreen onBack={() => {}} paymentVouchers />);
    await waitFor(() => expect(vouchers.listPendingVouchers).toHaveBeenCalled());

    expect(screen.queryByRole("region", { name: /Comprobantes por revisar/ })).not.toBeInTheDocument();
  });

  it("says the queue could not be read, and can read it again", async () => {
    const user = userEvent.setup();
    vouchers.listPendingVouchers.mockRejectedValueOnce(new Error("down"));
    render(<PaymentsScreen onBack={() => {}} paymentVouchers />);

    expect(await screen.findByText(/No pudimos leer los comprobantes/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /Reintentar comprobantes/ }));

    expect(await screen.findByRole("region", { name: /Comprobantes por revisar \(1\)/ })).toBeInTheDocument();
  });

  it("with module_payment_vouchers off, it never asks and never shows", async () => {
    const user = userEvent.setup();
    render(<PaymentsScreen onBack={() => {}} />);
    await screen.findByLabelText("Buscar alumno");

    await user.type(screen.getByLabelText("Buscar alumno"), "Mateo");
    await screen.findByText("Sánchez Mateo");

    expect(vouchers.listPendingVouchers).not.toHaveBeenCalled();
    expect(screen.queryByText(/Comprobante/)).not.toBeInTheDocument();
  });
});
