/**
 * Typed fetch helpers for the Egresos module (T10): `/api/v1/expenses*`,
 * `/api/v1/expense-categories*` and closing the day's cash cut. Behind
 * `module_expenses` (and `module_finance`) + cookie auth; reads need
 * `expenses:read`, writes `expenses:write`, reopening a closed cut
 * `finance:write`. This client is UX only — the gateway is the boundary.
 *
 * MONEY IS ALWAYS INTEGER CENTS on the wire. Who records, closes or reopens is
 * the session, never a field sent from here.
 */
import { authFetch } from "../auth/authFetch";

export type ExpenseMethod = "cash" | "card" | "transfer";
export type ExpenseStatus = "active" | "cancelled";
export type ReportPeriod = "daily" | "weekly" | "monthly" | "yearly";

const METHODS: readonly ExpenseMethod[] = ["cash", "card", "transfer"];
const STATUSES: readonly ExpenseStatus[] = ["active", "cancelled"];

/** The largest receipt the gateway takes (5 MB). */
export const MAX_RECEIPT_BYTES = 5 * 1024 * 1024;

export interface ExpenseCategory {
  id: string;
  name: string;
  active: boolean;
}

export interface ExpenseReceipt {
  contentType: string;
  sizeBytes: number;
}

export interface Expense {
  id: string;
  spentOn: string;
  categoryId: string;
  categoryName: string;
  amountCents: number;
  method: ExpenseMethod;
  description: string;
  status: ExpenseStatus;
  cancelReason?: string;
  cancelledAt?: string;
  receipt?: ExpenseReceipt;
  createdByName?: string;
  createdAt?: string;
  updatedAt?: string;
}

/** One side of a history entry: the expense as it stood. */
export interface ExpenseSnapshot {
  spentOn?: string;
  categoryName?: string;
  amountCents?: number;
  method?: string;
  description?: string;
  status?: string;
  hasReceipt?: boolean;
}

export type ExpenseEventKind = "created" | "edited" | "cancelled" | "receipt_attached" | "receipt_removed";

export interface ExpenseEvent {
  id: string;
  kind: ExpenseEventKind;
  before: ExpenseSnapshot;
  after: ExpenseSnapshot;
  reason?: string;
  actorName?: string;
  at: string;
}

export interface ExpenseInput {
  spentOn: string;
  categoryId: string;
  amountCents: number;
  method: ExpenseMethod;
  description: string;
}

/** An amount per method and their sum; a balance may be negative. */
export interface MethodTotals {
  cashCents: number;
  cardCents: number;
  transferCents: number;
  totalCents: number;
}

export interface CategoryTotal {
  categoryId: string;
  categoryName: string;
  amountCents: number;
  count: number;
}

export interface FinanceSummary {
  income: MethodTotals;
  /** What credit notes settled. NOT income: outside every figure here. */
  creditNoteCents: number;
  expenses: MethodTotals;
  balance: MethodTotals;
  expensesByCategory: CategoryTotal[];
}

export interface FinanceReport {
  period: ReportPeriod;
  from: string;
  to: string;
  label: string;
  summary: FinanceSummary;
  incomeByConcept: { lines: { conceptId: string; concept: string; amountCents: number }[]; totalCents: number; creditNoteCents: number };
  expenses: Expense[];
}

/** One closure of a day's cut: the snapshot taken when it was closed. */
export interface CashCutClosure {
  id: string;
  day: string;
  expectedCashCents: number;
  countedCashCents: number;
  /** counted − expected: negative is money missing from the drawer. */
  differenceCents: number;
  income: MethodTotals;
  expenses: MethodTotals;
  creditNoteCents: number;
  note?: string;
  closedByName?: string;
  closedAt: string;
  reopenedAt?: string;
  reopenedByName?: string;
  reopenReason?: string;
}

// --- transport -------------------------------------------------------------

const ID = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;

function url(baseURL: string, path: string): string {
  return `${baseURL.replace(/\/+$/, "")}${path}`;
}

/** An id that is not one never reaches the network (it would be a path). */
function idPath(prefix: string, id: string, suffix = ""): string {
  if (!ID.test(id)) {
    throw new Error("Ese registro no existe.");
  }
  return `${prefix}/${encodeURIComponent(id)}${suffix}`;
}

/** The gateway's sentence, so the screen can say WHY it was refused. */
async function failure(res: Response): Promise<Error> {
  const body = (await res.text().catch(() => "")).trim();
  return new Error(body !== "" ? body : `HTTP ${res.status}`);
}

async function request<T>(baseURL: string, path: string, init: RequestInit = {}): Promise<T> {
  const res = await authFetch(url(baseURL, path), { credentials: "include", ...init });
  if (!res.ok) {
    throw await failure(res);
  }
  return (await res.json()) as T;
}

function send<T>(baseURL: string, path: string, method: "POST" | "PUT", body: unknown): Promise<T> {
  return request<T>(baseURL, path, { method, headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
}

function query(params: Record<string, string | undefined>): string {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== "") search.set(key, value);
  }
  const qs = search.toString();
  return qs === "" ? "" : `?${qs}`;
}

function oneOf<T extends string>(allowed: readonly T[], value: unknown, fallback: T): T {
  return typeof value === "string" && (allowed as readonly string[]).includes(value) ? (value as T) : fallback;
}

type WireExpense = Omit<Expense, "method" | "status"> & { method: string; status: string };

const toExpense = (raw: WireExpense): Expense => ({
  ...raw,
  method: oneOf(METHODS, raw.method, "cash"),
  status: oneOf(STATUSES, raw.status, "active"),
});

const ZERO: MethodTotals = { cashCents: 0, cardCents: 0, transferCents: 0, totalCents: 0 };

const totals = (raw: Partial<MethodTotals> | undefined): MethodTotals => ({ ...ZERO, ...(raw ?? {}) });

type WireSummary = Partial<Omit<FinanceSummary, "income" | "expenses" | "balance">> & {
  income?: Partial<MethodTotals>;
  expenses?: Partial<MethodTotals>;
  balance?: Partial<MethodTotals>;
};

const toSummary = (raw: WireSummary): FinanceSummary => ({
  income: totals(raw.income),
  creditNoteCents: raw.creditNoteCents ?? 0,
  expenses: totals(raw.expenses),
  balance: totals(raw.balance),
  expensesByCategory: raw.expensesByCategory ?? [],
});

/** A closure from the wire, totals filled in. */
export function toClosure(raw: Partial<CashCutClosure> & { id: string; day: string }): CashCutClosure {
  return {
    ...raw,
    expectedCashCents: raw.expectedCashCents ?? 0,
    countedCashCents: raw.countedCashCents ?? 0,
    differenceCents: raw.differenceCents ?? 0,
    income: totals(raw.income),
    expenses: totals(raw.expenses),
    creditNoteCents: raw.creditNoteCents ?? 0,
    closedAt: raw.closedAt ?? "",
  };
}

export { toExpense as fromWireExpense, totals as fromWireTotals };
export type { WireExpense };

// --- categories ------------------------------------------------------------

const CATEGORIES = "/api/v1/expense-categories";

export async function listCategories(baseURL: string, includeInactive = false): Promise<ExpenseCategory[]> {
  const res = await request<{ categories?: ExpenseCategory[] }>(baseURL, `${CATEGORIES}${query({ includeInactive: includeInactive ? "true" : undefined })}`);
  return res.categories ?? [];
}

export function createCategory(baseURL: string, name: string): Promise<ExpenseCategory> {
  return send<ExpenseCategory>(baseURL, CATEGORIES, "POST", { name });
}

export function updateCategory(baseURL: string, id: string, input: { name: string; active: boolean }): Promise<ExpenseCategory> {
  return send<ExpenseCategory>(baseURL, idPath(CATEGORIES, id), "PUT", input);
}

// --- expenses --------------------------------------------------------------

const EXPENSES = "/api/v1/expenses";

export interface ExpenseQuery {
  from: string;
  to: string;
  categoryId?: string;
  includeCancelled?: boolean;
}

export async function listExpenses(baseURL: string, q: ExpenseQuery): Promise<Expense[]> {
  const res = await request<{ expenses?: WireExpense[] }>(
    baseURL,
    `${EXPENSES}${query({ from: q.from, to: q.to, categoryId: q.categoryId, includeCancelled: q.includeCancelled === true ? "true" : undefined })}`,
  );
  return (res.expenses ?? []).map(toExpense);
}

export async function createExpense(baseURL: string, input: ExpenseInput): Promise<Expense> {
  return toExpense(await send<WireExpense>(baseURL, EXPENSES, "POST", input));
}

export async function updateExpense(baseURL: string, id: string, input: ExpenseInput & { reason: string }): Promise<Expense> {
  return toExpense(await send<WireExpense>(baseURL, idPath(EXPENSES, id), "PUT", input));
}

export async function cancelExpense(baseURL: string, id: string, reason: string): Promise<Expense> {
  return toExpense(await send<WireExpense>(baseURL, idPath(EXPENSES, id, "/cancel"), "POST", { reason }));
}

export async function getExpense(baseURL: string, id: string): Promise<{ expense: Expense; events: ExpenseEvent[] }> {
  const path = idPath(EXPENSES, id);
  const res = await request<{ expense: WireExpense; events?: ExpenseEvent[] }>(baseURL, path);
  return { expense: toExpense(res.expense), events: res.events ?? [] };
}

/** Attaches (or replaces) the receipt: the raw file, at most 5 MB. */
export async function uploadReceipt(baseURL: string, id: string, file: Blob): Promise<Expense> {
  const path = idPath(EXPENSES, id, "/receipt");
  if (file.size > MAX_RECEIPT_BYTES) {
    throw new Error("El comprobante pesa más de 5 MB.");
  }
  return toExpense(
    await request<WireExpense>(baseURL, path, {
      method: "PUT",
      headers: { "Content-Type": file.type === "" ? "application/octet-stream" : file.type },
      body: file,
    }),
  );
}

/** The receipt's bytes, read with the session — there is no public URL. */
export async function getReceiptBlob(baseURL: string, id: string): Promise<Blob> {
  const res = await authFetch(url(baseURL, idPath(EXPENSES, id, "/receipt")), { credentials: "include" });
  if (!res.ok) {
    throw await failure(res);
  }
  return res.blob();
}

export async function deleteReceipt(baseURL: string, id: string): Promise<Expense> {
  return toExpense(await request<WireExpense>(baseURL, idPath(EXPENSES, id, "/receipt"), { method: "DELETE" }));
}

// --- the period's money and reports ----------------------------------------

export async function getSummary(baseURL: string, from: string, to: string): Promise<FinanceSummary> {
  return toSummary(await request<WireSummary>(baseURL, `${EXPENSES}/summary${query({ from, to })}`));
}

export async function getReport(baseURL: string, period: ReportPeriod, date: string): Promise<FinanceReport> {
  const raw = await request<Omit<FinanceReport, "summary" | "expenses"> & { summary?: WireSummary; expenses?: WireExpense[] }>(
    baseURL,
    `${EXPENSES}/reports${query({ period, date })}`,
  );
  return {
    ...raw,
    summary: toSummary(raw.summary ?? {}),
    incomeByConcept: { lines: raw.incomeByConcept?.lines ?? [], totalCents: raw.incomeByConcept?.totalCents ?? 0, creditNoteCents: raw.incomeByConcept?.creditNoteCents ?? 0 },
    expenses: (raw.expenses ?? []).map(toExpense),
  };
}

const FILENAME = /filename="?([^";]+)"?/;

/** An export (PDF or Excel), fetched with the session, ready to save. */
export async function downloadReport(baseURL: string, period: ReportPeriod, date: string, format: "pdf" | "xlsx"): Promise<{ blob: Blob; filename: string }> {
  const res = await authFetch(url(baseURL, `${EXPENSES}/reports/${format}${query({ period, date })}`), { credentials: "include" });
  if (!res.ok) {
    throw await failure(res);
  }
  const named = FILENAME.exec(res.headers.get("Content-Disposition") ?? "");
  return { blob: await res.blob(), filename: named?.[1] ?? `reporte-finanzas.${format}` };
}

// --- the cash cut ----------------------------------------------------------

export async function closeCashCut(baseURL: string, input: { day: string; countedCashCents: number; note: string }): Promise<CashCutClosure> {
  return toClosure(await send<CashCutClosure>(baseURL, "/api/v1/finance/cash-up/close", "POST", input));
}

export async function reopenCashCut(baseURL: string, day: string, reason: string): Promise<CashCutClosure> {
  return toClosure(await send<CashCutClosure>(baseURL, "/api/v1/finance/cash-up/reopen", "POST", { day, reason }));
}
