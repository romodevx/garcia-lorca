import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
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
vi.mock("../../students/api", () => ({ listStudents: vi.fn(), listNotes: vi.fn(), addNote: vi.fn(), editNote: vi.fn(), voidNote: vi.fn(), StudentsError: class extends Error {} }));

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
  id, curp: `C${id}`, firstName: first, lastName: last, level: "primaria", levelId: "", gradeGroup: "1° A", guardianName: "Tutor", admissionStatus: "enrolled",
});

beforeEach(() => {
  config.listCycles.mockResolvedValue([{ id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" }]);
  students.listStudents.mockResolvedValue([student("s1", "Valentina", "Cruz"), student("s2", "Mateo", "Sánchez")]);
  finance.listCharges.mockResolvedValue([charge("ch1", "s1", 100000), charge("ch2", "s2", 0)]);
  finance.getOverdue.mockResolvedValue([]);
  finance.getAccount.mockResolvedValue({ charges: [], payments: [], totalBalanceCents: 0, unallocatedCents: 0, dueTodayCents: 0, surchargeCents: 0, earlyDiscountCents: 0, asOf: "" });
  finance.previewDeposit.mockResolvedValue({ allocations: [], surcharges: [], unallocatedCents: 0 });
  finance.getCashUp.mockResolvedValue({ deposits: [], cancelled: [], payments: [], cashCents: 0, cardCents: 0, transferCents: 0, totalCents: 0, creditNoteCents: 0 });
  finance.getIncome.mockResolvedValue({ lines: [], totalCents: 0, creditNoteCents: 0 });
});
afterEach(() => vi.clearAllMocks());

describe("FinanceScreen", () => {
  it("resolves student names and shows charge status", async () => {
    render(<FinanceScreen onBack={() => {}} />);
    expect(await screen.findByRole("heading", { name: "Finanzas" })).toBeInTheDocument();
    expect(await screen.findByText("Cruz Valentina")).toBeInTheDocument();
    expect(screen.getByText("Sánchez Mateo")).toBeInTheDocument();
    // ch2 is fully paid → "Al corriente"; ch1 has a balance → a "Registrar pago" action
    expect(screen.getByText("Al corriente")).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: /Registrar pago/ }).length).toBeGreaterThan(0);
  });

  it("registers a payment for a charge from the drawer", async () => {
    const user = userEvent.setup();
    finance.registerPayment.mockResolvedValue({} as financeApi.PaymentResult);
    render(<FinanceScreen onBack={() => {}} />);
    await screen.findByText("Cruz Valentina");

    await user.click(screen.getAllByRole("button", { name: /Registrar pago/ })[0] as HTMLElement);
    const dialog = await screen.findByRole("dialog", { name: "Registrar pago" });
    await user.clear(within(dialog).getByLabelText("Monto a pagar", { selector: "input" }));
    await user.clear(within(dialog).getByLabelText("Monto a pagar", { selector: "input" }));
    await user.type(within(dialog).getByLabelText("Monto a pagar", { selector: "input" }), "500");
    await user.click(within(dialog).getByRole("button", { name: /Registrar pago/ }));

    await waitFor(() =>
      expect(finance.registerPayment).toHaveBeenCalledWith(expect.any(String), expect.objectContaining({ chargeId: "ch1", amountCents: 50000, method: "cash" })),
    );
  });
});

// T6: Registrar pago of one charge shows that pupil's notes on its left.
describe("FinanceScreen · notas al registrar pago", () => {
  it("shows the charge's pupil's notes beside the payment, in a wider drawer", async () => {
    const user = userEvent.setup();
    students.listNotes.mockResolvedValue([{
      id: "n-1", studentId: "s1", body: "Pagan con transferencia del abuelo", authorUserId: "u-dir", authorName: "Dirección",
      createdAt: "2026-09-20T15:00:00Z", voided: false,
    }]);
    render(<FinanceScreen onBack={() => {}} notes={{ userId: "u-me", canRead: true, canWrite: true, canVoidAny: false }} />);
    await screen.findByText("Cruz Valentina");
    await user.click(screen.getAllByRole("button", { name: /Registrar pago/ })[0] as HTMLElement);
    const dialog = await screen.findByRole("dialog", { name: "Registrar pago" });
    const panel = await within(dialog).findByRole("complementary", { name: "Notas del alumno" });
    expect(panel).toHaveTextContent("Pagan con transferencia del abuelo");
    expect(students.listNotes).toHaveBeenCalledWith(expect.any(String), "s1");
    expect(dialog.className).toContain("drawer-lg");
  });

  it("reads no notes without access to them", async () => {
    const user = userEvent.setup();
    render(<FinanceScreen onBack={() => {}} />);
    await screen.findByText("Cruz Valentina");
    await user.click(screen.getAllByRole("button", { name: /Registrar pago/ })[0] as HTMLElement);
    const dialog = await screen.findByRole("dialog", { name: "Registrar pago" });
    expect(students.listNotes).not.toHaveBeenCalled();
    expect(dialog.className).toContain("drawer-md");
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
    await user.clear(within(dialog).getByLabelText("Monto a pagar", { selector: "input" }));
    await user.type(within(dialog).getByLabelText("Monto a pagar", { selector: "input" }), "500");
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
    await user.clear(within(dialog).getByLabelText("Monto a pagar", { selector: "input" }));
    await user.type(within(dialog).getByLabelText("Monto a pagar", { selector: "input" }), "500");
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

  // Task 7: a payment captured late carries the day the family paid; it starts
  // on today and cannot be moved into the future.
  it("registers a payment with the day the family paid", async () => {
    const user = userEvent.setup();
    finance.registerPayment.mockResolvedValue({} as financeApi.PaymentResult);
    render(<FinanceScreen onBack={() => {}} today="2026-10-12" />);
    await screen.findByText("Cruz Valentina");
    await user.click(screen.getAllByRole("button", { name: /Registrar pago/ })[0] as HTMLElement);
    const dialog = await screen.findByRole("dialog", { name: "Registrar pago" });

    const date = within(dialog).getByLabelText("Fecha de pago");
    expect(date).toHaveValue("2026-10-12");
    expect(date).toHaveAttribute("max", "2026-10-12");
    fireEvent.change(date, { target: { value: "2026-01-05" } });
    await user.click(within(dialog).getByRole("button", { name: /Registrar pago/ }));

    await waitFor(() =>
      expect(finance.registerPayment).toHaveBeenCalledWith(expect.any(String), expect.objectContaining({ paidAt: "2026-01-05" })),
    );
  });

  it("does not discard a half-captured payment when the backdrop is clicked", async () => {
    // At counter speed, with a parent waiting, a stray click outside the panel
    // silently threw away a typed amount.
    const user = await openDrawer();
    const dialog = screen.getByRole("dialog", { name: /Registrar pago/i });
    await user.clear(within(dialog).getByLabelText("Monto a pagar", { selector: "input" }));
    await user.type(within(dialog).getByLabelText("Monto a pagar", { selector: "input" }), "500");

    await user.click(screen.getByTestId("drawer-backdrop"));

    expect(screen.getByRole("dialog", { name: /Registrar pago/i })).toBeInTheDocument();
    // Leaving the field formats it (MoneyField); the amount itself survives.
    expect(within(dialog).getByLabelText("Monto a pagar", { selector: "input" })).toHaveValue("500.00");
  });

  it("still closes on the backdrop when nothing has been typed", async () => {
    const user = await openDrawer();
    // The amount arrives pre-filled with the charge balance, so "untouched"
    // means clearing it — otherwise every drawer is dirty from the first paint.
    await user.clear(screen.getByRole("dialog", { name: /Registrar pago/i }).querySelector("input")!);
    await user.click(screen.getByTestId("drawer-backdrop"));
    await waitFor(() => expect(screen.queryByRole("dialog", { name: /Registrar pago/i })).not.toBeInTheDocument());
  });
});

describe("FinanceScreen — corte de caja", () => {
  // The number the office closes the day with, one click from the counter.
  it("opens the day's takings", async () => {
    const user = userEvent.setup();
    finance.getCashUp.mockResolvedValue({
      deposits: [], cancelled: [], payments: [], cashCents: 320000, cardCents: 0, transferCents: 150000, totalCents: 470000, creditNoteCents: 0,
    });
    render(<FinanceScreen onBack={() => {}} />);
    await screen.findByText("Cruz Valentina");

    await user.click(screen.getByRole("button", { name: /Corte de caja/ }));

    expect(await screen.findByTestId("cash-up-total")).toHaveTextContent("4,700");
  });
});

describe("FinanceScreen — el corte de otro día (tarea 7)", () => {
  // A payment captured late is dated the day the family paid, so it lands on
  // that day's cut. The office has to be able to open that cut to see it —
  // with who keyed it and the "capturado el …" mark.
  it("opens the cut of a day the office chooses", async () => {
    const user = userEvent.setup();
    finance.getCashUp.mockResolvedValue({
      deposits: [], cancelled: [], payments: [], cashCents: 0, cardCents: 0, transferCents: 0, totalCents: 0, creditNoteCents: 0,
    });
    render(<FinanceScreen onBack={() => {}} today="2026-10-12" />);
    await screen.findByText("Cruz Valentina");
    await user.click(screen.getByRole("button", { name: /Corte de caja/ }));
    await screen.findByTestId("cash-up-total");
    expect(finance.getCashUp).toHaveBeenLastCalledWith(expect.any(String), "");

    const day = screen.getByLabelText("Día del corte");
    expect(day).toHaveValue("2026-10-12");
    expect(day).toHaveAttribute("max", "2026-10-12");
    fireEvent.change(day, { target: { value: "2026-10-03" } });
    await waitFor(() => expect(finance.getCashUp).toHaveBeenLastCalledWith(expect.any(String), "2026-10-03"));
 
    // Closing and reopening is closing the day again: today, not the day last
    // looked at.
    await user.click(screen.getByRole("button", { name: "Cerrar" }));
    await user.click(screen.getByRole("button", { name: /Corte de caja/ }));
    expect(await screen.findByLabelText("Día del corte")).toHaveValue("2026-10-12");
  });
});

describe("FinanceScreen — exportar", () => {
  // "Exportar esta vista" has to mean the view: the tab picked and the search
  // typed. A file that quietly holds everything is worse than no file, because
  // it looks right.
  it("exports the rows on screen, not the whole ledger", async () => {
    const user = userEvent.setup();
    render(<FinanceScreen onBack={() => {}} />);
    await screen.findByText("Cruz Valentina");
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

describe("FinanceScreen — filtro por mes", () => {
  // "¿Quién debe septiembre?" is how a school reads its debt. A year of rows on
  // one screen is not an answer to that question.
  it("narrows the debt to one month", async () => {
    const user = userEvent.setup();
    finance.listCharges.mockResolvedValue([
      { ...charge("ch1", "s1", 100000), dueDate: "2026-09-10", description: "Colegiatura 2026-09" },
      { ...charge("ch2", "s2", 100000), dueDate: "2026-10-10", description: "Colegiatura 2026-10" },
    ]);
    render(<FinanceScreen onBack={() => {}} />);
    await screen.findByText("Cruz Valentina");

    await user.selectOptions(screen.getByLabelText("Mes"), "2026-10");

    expect(await screen.findByText("Sánchez Mateo")).toBeInTheDocument();
    expect(screen.queryByText("Cruz Valentina")).not.toBeInTheDocument();
  });
});

// T8: one charge, paid with a credit note and whatever completes it.
describe("PayDrawer · nota de crédito + pago combinado (tarea 8)", () => {
  async function openDrawer(): Promise<{ user: ReturnType<typeof userEvent.setup>; dialog: HTMLElement }> {
    const user = userEvent.setup();
    render(<FinanceScreen onBack={() => {}} />);
    const rows = await screen.findAllByRole("button", { name: /Registrar pago/i });
    await user.click(rows[0]!);
    const dialog = await screen.findByRole("dialog", { name: /Registrar pago/i });
    return { user, dialog };
  }
  const tenderRow = (dialog: HTMLElement, n: number): HTMLElement => within(dialog).getByRole("group", { name: `Forma de pago ${n}` });
  const submit = (user: ReturnType<typeof userEvent.setup>, dialog: HTMLElement): Promise<void> =>
    user.click(within(dialog).getByRole("button", { name: /^Registrar pago$/i }));

  it("sends one cash tender for the charge's balance on the happy path", async () => {
    finance.registerPayment.mockResolvedValue({} as financeApi.PaymentResult);
    const { user, dialog } = await openDrawer();

    await submit(user, dialog);

    await waitFor(() => expect(finance.registerPayment).toHaveBeenCalled());
    expect(finance.registerPayment.mock.calls[0]?.[1]).toMatchObject({
      amountCents: 100000, method: "cash", tenders: [{ method: "cash", amountCents: 100000 }],
    });
  });

  it("completes a credit note with a transfer", async () => {
    finance.registerPayment.mockResolvedValue({} as financeApi.PaymentResult);
    const { user, dialog } = await openDrawer();

    await user.click(within(tenderRow(dialog, 1)).getByRole("button", { name: "Nota de crédito" }));
    await user.clear(within(tenderRow(dialog, 1)).getByLabelText("Monto"));
    await user.type(within(tenderRow(dialog, 1)).getByLabelText("Monto"), "300");
    await user.click(within(dialog).getByRole("button", { name: "Agregar método" }));
    await user.click(within(tenderRow(dialog, 2)).getByRole("button", { name: "Transferencia" }));
    await submit(user, dialog);

    await waitFor(() => expect(finance.registerPayment).toHaveBeenCalled());
    expect(finance.registerPayment.mock.calls[0]?.[1]?.tenders).toEqual([
      { method: "credit_note", amountCents: 30000 },
      { method: "transfer", amountCents: 70000 },
    ]);
  });

  it("refuses more than the charge's balance, and goes to the amount", async () => {
    const { user, dialog } = await openDrawer();
    const amount = within(dialog).getByLabelText("Monto a pagar", { selector: "input" });

    await user.clear(amount);
    await user.type(amount, "1500");
    await submit(user, dialog);

    expect(await within(dialog).findByText(/No puede ser mayor que el saldo del cargo/)).toBeInTheDocument();
    await waitFor(() => expect(document.activeElement).toBe(amount));
    expect(finance.registerPayment).not.toHaveBeenCalled();
  });

  it("refuses tenders that do not add up to the amount to pay", async () => {
    const { user, dialog } = await openDrawer();

    await user.clear(within(tenderRow(dialog, 1)).getByLabelText("Monto"));
    await user.type(within(tenderRow(dialog, 1)).getByLabelText("Monto"), "200");
    await submit(user, dialog);

    expect(await within(dialog).findByText("La suma de las formas de pago debe ser igual al monto a pagar.")).toBeInTheDocument();
    expect(finance.registerPayment).not.toHaveBeenCalled();
  });
});
