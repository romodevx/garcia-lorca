import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as financeApi from "../../finance/api";
import * as configApi from "../../config/api";
import * as studentsApi from "../../students/api";
import { FinanceScreen } from "./FinanceScreen";

vi.mock("../../finance/api", () => ({
  listCharges: vi.fn(),
  getOverdue: vi.fn(),
  registerPayment: vi.fn(),
  getAccount: vi.fn(),
  previewDeposit: vi.fn(),
  registerDeposit: vi.fn(),
  getCashUp: vi.fn(),
  getIncome: vi.fn(),
}));
vi.mock("../../config/api", () => ({ listCycles: vi.fn() }));
vi.mock("../../students/api", () => ({ listStudents: vi.fn() }));

const finance = vi.mocked(financeApi);
const config = vi.mocked(configApi);
const students = vi.mocked(studentsApi);

const charge = (id: string, studentId: string, remaining: number): financeApi.Charge => ({
  id,
  studentId,
  cycleId: "c1",
  conceptId: "k1",
  description: "Colegiatura",
  amountCents: 150000,
  discountCents: 0,
  dueDate: "2026-09-05",
  status: "pending",
  paidCents: 150000 - remaining,
  remainingCents: remaining,
});
const student = (id: string, first: string, last: string): studentsApi.Student => ({
  id, curp: `C${id}`, firstName: first, lastName: last, level: "primaria", gradeGroup: "1° A", guardianName: "Tutor", admissionStatus: "enrolled",
});

beforeEach(() => {
  config.listCycles.mockResolvedValue([{ id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" }]);
  students.listStudents.mockResolvedValue([student("s1", "Valentina", "Cruz"), student("s2", "Mateo", "Sánchez")]);
  finance.listCharges.mockResolvedValue([charge("ch1", "s1", 100000), charge("ch2", "s2", 0)]);
  finance.getOverdue.mockResolvedValue([]);
  finance.getAccount.mockResolvedValue({ charges: [], payments: [], totalBalanceCents: 0, unallocatedCents: 0 });
  finance.previewDeposit.mockResolvedValue({ allocations: [], surcharges: [], unallocatedCents: 0 });
  finance.getCashUp.mockResolvedValue({ deposits: [], cancelled: [], cashCents: 0, cardCents: 0, transferCents: 0, totalCents: 0 });
  finance.getIncome.mockResolvedValue({ lines: [], totalCents: 0 });
});
afterEach(() => vi.clearAllMocks());

describe("FinanceScreen", () => {
  it("resolves student names and shows charge status", async () => {
    render(<FinanceScreen onBack={() => {}} />);
    expect(await screen.findByRole("heading", { name: "Finanzas y Adeudos" })).toBeInTheDocument();
    expect(await screen.findByText("Valentina Cruz")).toBeInTheDocument();
    expect(screen.getByText("Mateo Sánchez")).toBeInTheDocument();
    // ch2 is fully paid → "Al corriente"; ch1 has a balance → a "Registrar pago" action
    expect(screen.getByText("Al corriente")).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: /Registrar pago/ }).length).toBeGreaterThan(0);
  });

  it("registers a payment for a charge from the drawer", async () => {
    const user = userEvent.setup();
    finance.registerPayment.mockResolvedValue({} as financeApi.PaymentResult);
    render(<FinanceScreen onBack={() => {}} />);
    await screen.findByText("Valentina Cruz");

    await user.click(screen.getAllByRole("button", { name: /Registrar pago/ })[0] as HTMLElement);
    const dialog = await screen.findByRole("dialog", { name: "Registrar pago" });
    await user.clear(within(dialog).getByLabelText("Monto"));
    await user.clear(within(dialog).getByLabelText("Monto"));
    await user.type(within(dialog).getByLabelText("Monto"), "500");
    await user.click(within(dialog).getByRole("button", { name: /Registrar pago/ }));

    await waitFor(() =>
      expect(finance.registerPayment).toHaveBeenCalledWith(expect.any(String), expect.objectContaining({ chargeId: "ch1", amountCents: 50000, method: "cash" })),
    );
  });
});

/**
 * The payment drawer moves money. These two were reachable in production.
 */
describe("PayDrawer — money safety", () => {
  async function openDrawer(): Promise<ReturnType<typeof userEvent.setup>> {
    const user = userEvent.setup();
    render(<FinanceScreen onBack={() => {}} />);
    // Several rows expose a "Registrar pago" button; take the first and then
    // scope every later query to the dialog so the row buttons never match.
    const rows = await screen.findAllByRole("button", { name: /Registrar pago/i });
    await user.click(rows[0]!);
    await screen.findByRole("dialog", { name: /Registrar pago/i });
    return user;
  }

  it("sends a non-empty idempotency key", async () => {
    // The proto documents the consequence of an empty key: "Empty ⇒ no
    // deduplication (every call posts a new payment)". The drawer shipped with
    // idempotencyKey: "", so a double click double-charged a family.
    finance.registerPayment.mockResolvedValue(undefined as never);
    const user = await openDrawer();

    const dialog = screen.getByRole("dialog", { name: /Registrar pago/i });
    await user.clear(within(dialog).getByLabelText("Monto"));
    await user.type(within(dialog).getByLabelText("Monto"), "500");
    await user.click(within(dialog).getByRole("button", { name: /^Registrar pago$/i }));

    await waitFor(() => expect(finance.registerPayment).toHaveBeenCalled());
    const sent = finance.registerPayment.mock.calls[0]?.[1];
    expect(sent?.idempotencyKey).toBeTruthy();
    expect(sent?.idempotencyKey).not.toBe("");
  });

  it("reuses the SAME key when a failed attempt is retried", async () => {
    // A retry after a network error must not post a second payment. Minting a
    // fresh key per click would defeat the whole mechanism.
    finance.registerPayment.mockRejectedValueOnce(new Error("network"));
    const user = await openDrawer();

    const dialog = screen.getByRole("dialog", { name: /Registrar pago/i });
    await user.clear(within(dialog).getByLabelText("Monto"));
    await user.type(within(dialog).getByLabelText("Monto"), "500");
    const save = within(dialog).getByRole("button", { name: /^Registrar pago$/i });
    await user.click(save);
    await screen.findByText(/network|No se pudo/i);

    finance.registerPayment.mockResolvedValue(undefined as never);
    await user.click(save);

    await waitFor(() => expect(finance.registerPayment).toHaveBeenCalledTimes(2));
    const first = finance.registerPayment.mock.calls[0]?.[1]?.idempotencyKey;
    const second = finance.registerPayment.mock.calls[1]?.[1]?.idempotencyKey;
    expect(second).toBe(first);
  });

  it("does not discard a half-captured payment when the backdrop is clicked", async () => {
    // At counter speed, with a parent waiting, a stray click outside the panel
    // silently threw away a typed amount.
    const user = await openDrawer();
    const dialog = screen.getByRole("dialog", { name: /Registrar pago/i });
    await user.clear(within(dialog).getByLabelText("Monto"));
    await user.type(within(dialog).getByLabelText("Monto"), "500");

    await user.click(screen.getByTestId("pay-drawer-backdrop"));

    expect(screen.getByRole("dialog", { name: /Registrar pago/i })).toBeInTheDocument();
    expect(within(dialog).getByLabelText("Monto")).toHaveValue("500");
  });

  it("still closes on the backdrop when nothing has been typed", async () => {
    const user = await openDrawer();
    // The amount arrives pre-filled with the charge balance, so "untouched"
    // means clearing it — otherwise every drawer is dirty from the first paint.
    await user.clear(screen.getByRole("dialog", { name: /Registrar pago/i }).querySelector("input")!);
    await user.click(screen.getByTestId("pay-drawer-backdrop"));
    await waitFor(() => expect(screen.queryByRole("dialog", { name: /Registrar pago/i })).not.toBeInTheDocument());
  });
});

describe("FinanceScreen — cobrar", () => {
  // The counter's entry point is a pupil, not a charge. A cashier is handed
  // money by a parent who says a name; making them find the right row of the
  // right month first is what sent the office back to the notebook.
  it("opens the counter by finding the pupil", async () => {
    const user = userEvent.setup();
    finance.getAccount.mockResolvedValue({ charges: [], payments: [], totalBalanceCents: 640000, unallocatedCents: 0 });
    render(<FinanceScreen onBack={() => {}} />);
    await screen.findByText("Valentina Cruz");

    await user.click(screen.getByRole("button", { name: /^Cobrar$/ }));
    await user.type(screen.getByLabelText("Buscar alumno"), "Valen");
    await user.click(await screen.findByRole("button", { name: /Valentina Cruz/ }));

    expect(await screen.findByRole("dialog", { name: "Cobrar" })).toBeInTheDocument();
    expect(await screen.findByTestId("amount-owed")).toHaveTextContent("6,400");
  });
});

describe("FinanceScreen — corte de caja", () => {
  // The number the office closes the day with, one click from the counter.
  it("opens the day's takings", async () => {
    const user = userEvent.setup();
    finance.getCashUp.mockResolvedValue({
      deposits: [], cancelled: [], cashCents: 320000, cardCents: 0, transferCents: 150000, totalCents: 470000,
    });
    render(<FinanceScreen onBack={() => {}} />);
    await screen.findByText("Valentina Cruz");

    await user.click(screen.getByRole("button", { name: /Corte de caja/ }));

    expect(await screen.findByTestId("cash-up-total")).toHaveTextContent("4,700");
  });
});

describe("FinanceScreen — exportar", () => {
  // "Exportar esta vista" has to mean the view: the tab picked and the search
  // typed. A file that quietly holds everything is worse than no file, because
  // it looks right.
  it("exports the rows on screen, not the whole ledger", async () => {
    const user = userEvent.setup();
    render(<FinanceScreen onBack={() => {}} />);
    await screen.findByText("Valentina Cruz");
    await user.type(screen.getByLabelText("Buscar"), "Valentina");

    // The spy goes on AFTER the render: React creates elements too, and a
    // blanket stub on createElement takes the whole tree down with it.
    const click = vi.fn();
    const anchor = { click, href: "", download: "", style: {} } as unknown as HTMLAnchorElement;
    const realCreate = document.createElement.bind(document);
    vi.spyOn(document, "createElement").mockImplementation((tag: string) =>
      (tag === "a" ? anchor : realCreate(tag)) as HTMLElement);
    vi.spyOn(document.body, "appendChild").mockImplementation((n) => n);
    vi.spyOn(document.body, "removeChild").mockImplementation((n) => n);
    vi.stubGlobal("URL", { createObjectURL: vi.fn().mockReturnValue("blob:x"), revokeObjectURL: vi.fn() });

    await user.click(screen.getByRole("button", { name: /Exportar/ }));

    expect(click).toHaveBeenCalledTimes(1);
    expect(anchor.download).toMatch(/^adeudos-\d{4}-\d{2}-\d{2}\.csv$/);
    expect(await screen.findByRole("status")).toHaveTextContent("1 renglones exportados");

    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });
});
