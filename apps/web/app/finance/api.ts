/**
 * Typed fetch helpers for the gateway's finance endpoints
 * (`/api/v1/finance/*`), gated behind the `module_finance` flag + cookie auth.
 * The session lives ONLY in the HttpOnly cookie the gateway set, so every call
 * uses `credentials:"include"`; nothing secret ever travels in the body.
 *
 * Reads require `finance:read`, writes `finance:write` (the owner role holds
 * both). These helpers mirror `app/grades/api.ts`: a `url()` trailing-slash
 * guard, throw on any non-2xx, and parse the gateway's single-key envelopes
 * (`{concepts:[…]}`, `{charges:[…]}`, `{overdue:[…]}`). Shapes match the gateway
 * REST contract exactly.
 *
 * MONEY IS ALWAYS INTEGER CENTS on the wire: every amount is a `…Cents` field
 * carrying a JSON integer, matching the gateway's int64. Pesos exist only in the
 * UI's formatting layer — no float ever reaches this module.
 *
 * `paidCents`/`remainingCents`/`status` are DERIVED server-side from the
 * charge's payments, so they are read-only here and never sent. This client is
 * UX only; the gateway is the security boundary.
 */

// --- entity types (match the gateway JSON contract) -----------------------

/** How often a concept is charged. */
import { authFetch } from "../auth/authFetch";

export type Recurrence = "once" | "monthly";

export interface ChargeConcept {
  id: string;
  name: string;
  defaultAmountCents: number;
  recurrence: Recurrence;
  active: boolean;
}

/** Lifecycle of a charge, recomputed server-side after every payment/discount. */
export type ChargeStatus = "pending" | "partial" | "paid" | "cancelled";

export interface Charge {
  id: string;
  studentId: string;
  cycleId: string;
  conceptId: string;
  description: string;
  amountCents: number;
  discountCents: number;
  dueDate: string;
  status: ChargeStatus;
  /** Server-derived from the charge's payments — never sent by the client. */
  paidCents: number;
  /** Server-derived (amount − discount − paid) — never sent by the client. */
  remainingCents: number;
}

/** How the money came in at the window. */
export type PaymentMethod = "cash" | "card" | "transfer";

export interface Payment {
  id: string;
  chargeId: string;
  amountCents: number;
  method: PaymentMethod;
  reference: string;
  paidAt: string;
}

/** A student's full statement: what was charged, what was paid, what is left. */
export interface AccountStatement {
  charges: Charge[];
  payments: Payment[];
  totalBalanceCents: number;
  /** Money the family has handed over that no charge has claimed. */
  unallocatedCents: number;
}

/**
 * Money as it arrived, before it was decided what it pays. The counter records
 * ONE amount; which months it covers is worked out server-side, oldest first.
 */
export interface Deposit {
  id: string;
  studentId: string;
  amountCents: number;
  method: PaymentMethod;
  reference: string;
  paidAt: string;
  /**
   * The school calendar the split was worked out against. "unavailable" means
   * the calendar could not be read and the deadline was computed without the
   * school's holidays — worth showing on a receipt somebody may dispute.
   */
  calendarVersion: string;
  voidedAt?: string;
  voidReason?: string;
}

/** What a deposit did: where the money landed and what is left over. */
export interface DepositResult {
  deposit: Deposit;
  allocations: Payment[];
  surcharges: Charge[];
  /** Money no charge claimed — the family's credit with the school. */
  unallocatedCents: number;
  /** The deposit already existed under this key; nothing was collected again. */
  replayed: boolean;
}

/** What an amount WOULD cover. No deposit, because nothing was taken. */
export interface DepositPreview {
  allocations: Payment[];
  surcharges: Charge[];
  unallocatedCents: number;
}

export interface RegisterDepositInput {
  studentId: string;
  amountCents: number;
  method: PaymentMethod;
  reference: string;
  /** The DAY the money changed hands, "2026-10-10". Omitted ⇒ the server stamps today. */
  paidAt?: string;
  /** One key per ATTEMPT: a retry returns the deposit already stored. */
  idempotencyKey?: string;
}

export interface PreviewDepositInput {
  studentId: string;
  amountCents: number;
  paidAt?: string;
}

/** The payment plus the charge with its recomputed paid/remaining/status. */
export interface PaymentResult {
  payment: Payment;
  charge: Charge;
}

/** Bulk-generation summary; `skipped` counts the idempotent re-run. */
export interface GenerateResult {
  created: number;
  skipped: number;
}

// --- input types ----------------------------------------------------------

export interface CreateConceptInput {
  name: string;
  defaultAmountCents: number;
  recurrence: Recurrence;
}

export interface UpdateConceptInput extends CreateConceptInput {
  active: boolean;
}

export interface CreateChargeInput {
  studentId: string;
  cycleId: string;
  conceptId: string;
  /** 0 ⇒ the concept's default amount. */
  amountCentsOverride: number;
  /** "" ⇒ the concept's name. */
  descriptionOverride: string;
  dueDate: string;
}

export interface GenerateChargesInput {
  conceptId: string;
  cycleId: string;
  /** "" ⇒ the whole cycle. */
  groupId: string;
  /** 0 ⇒ the concept's default amount. */
  amountCentsOverride: number;
  dueDate: string;
}

export interface RegisterPaymentInput {
  chargeId: string;
  amountCents: number;
  method: PaymentMethod;
  reference: string;
  /**
   * Optional handle on the payment ATTEMPT: the gateway returns the original
   * payment for a repeat under the same key instead of collecting again, so a
   * double-click or a retried request cannot charge the family twice. Omitted ⇒
   * every call posts a new payment.
   */
  idempotencyKey?: string;
  /**
   * The DAY the money changed hands, "2026-09-04". Omitted ⇒ the server stamps
   * now, which is right when the payment is captured as it is received and
   * wrong when a cashier is catching up on Friday's cash. Every collection rule
   * reads this date.
   */
  paidAt?: string;
}

/**
 * Charge list filters; empty values are dropped from the query string. Each is
 * independent, but the gateway REQUIRES at least one — an unscoped read would be
 * the school's whole ledger — and answers 400 when both are missing.
 */
export interface ChargeFilters {
  studentId?: string;
  cycleId?: string;
}

// --- the parse boundary ---------------------------------------------------

/**
 * On the wire an enum is just a string, so the union types above are a CLAIM
 * about the response, not a fact. `oneOf` is the single place that claim is
 * checked: every entity crossing into the app passes through one of the `to…`
 * mappers below, so no unchecked string is ever widened into a union by a cast.
 */
type Wire<T, K extends keyof T> = Omit<T, K> & { [P in K]: string };

const RECURRENCES: readonly Recurrence[] = ["once", "monthly"];
const CHARGE_STATUSES: readonly ChargeStatus[] = ["pending", "partial", "paid", "cancelled"];
const PAYMENT_METHODS: readonly PaymentMethod[] = ["cash", "card", "transfer"];

/** Narrow a wire string to its union; an unrecognized value degrades to `fallback`. */
function oneOf<T extends string>(allowed: readonly T[], value: string, fallback: T): T {
  return (allowed as readonly string[]).includes(value) ? (value as T) : fallback;
}

const toConcept = (raw: Wire<ChargeConcept, "recurrence">): ChargeConcept => ({
  ...raw,
  recurrence: oneOf(RECURRENCES, raw.recurrence, "monthly"),
});

/**
 * A status this build does not know means a gateway newer than the client. It
 * degrades to `pending`, the neutral lifecycle state: the row stays readable and
 * never claims the charge was voided, while the collect action remains gated on
 * `remainingCents` — so an unknown status can neither hide nor invent a debt.
 */
const toCharge = (raw: Wire<Charge, "status">): Charge => ({
  ...raw,
  status: oneOf(CHARGE_STATUSES, raw.status, "pending"),
});

const toPayment = (raw: Wire<Payment, "method">): Payment => ({
  ...raw,
  method: oneOf(PAYMENT_METHODS, raw.method, "cash"),
});

// --- transport helpers ----------------------------------------------------

const BASE_PATH = "/api/v1/finance";

function url(baseURL: string, path: string): string {
  return `${baseURL.replace(/\/+$/, "")}${path}`;
}

/**
 * The gateway's failure text (e.g. an overpay rejection), so the ventanilla can
 * show WHY the payment bounced instead of a generic message. Falls back to the
 * status when the body is empty.
 */
async function failure(res: Response): Promise<Error> {
  const body = await res.text().catch(() => "");
  const detail = body.trim();
  return new Error(detail !== "" ? detail : `HTTP ${res.status}`);
}

/** Send a JSON body (POST/PUT) and return the parsed entity. Throws on non-2xx. */
async function sendJSON<T>(baseURL: string, path: string, method: "POST" | "PUT", body: unknown): Promise<T> {
  const res = await authFetch(url(baseURL, `${BASE_PATH}${path}`), {
    method,
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw await failure(res);
  }
  return (await res.json()) as T;
}

/** POST a bodyless action (the id in the path is the whole request). */
async function postAction<T>(baseURL: string, path: string): Promise<T> {
  const res = await authFetch(url(baseURL, `${BASE_PATH}${path}`), { method: "POST", credentials: "include" });
  if (!res.ok) {
    throw await failure(res);
  }
  return (await res.json()) as T;
}

/** GET and return the parsed payload. Throws on any non-2xx. */
async function getJSON<T>(baseURL: string, path: string): Promise<T> {
  const res = await authFetch(url(baseURL, `${BASE_PATH}${path}`), { credentials: "include" });
  if (!res.ok) {
    throw await failure(res);
  }
  return (await res.json()) as T;
}

/** GET a list endpoint and unwrap the single-key envelope (`{ <key>: [...] }`). */
async function getList<T>(baseURL: string, path: string, key: string): Promise<T[]> {
  const data = await getJSON<Record<string, T[] | undefined>>(baseURL, path);
  return data[key] ?? [];
}

/** Builds `?a=1&b=2`, skipping empty values, so optional filters simply drop. */
function query(params: Record<string, string | undefined>): string {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== "") {
      search.set(key, value);
    }
  }
  const qs = search.toString();
  return qs === "" ? "" : `?${qs}`;
}

// --- concepts -------------------------------------------------------------

export async function createConcept(baseURL: string, input: CreateConceptInput): Promise<ChargeConcept> {
  const body = {
    name: input.name,
    defaultAmountCents: input.defaultAmountCents,
    recurrence: input.recurrence,
  };
  return toConcept(await sendJSON<Wire<ChargeConcept, "recurrence">>(baseURL, "/concepts", "POST", body));
}

/** Full replace of a concept's mutable fields, `active` included. */
export async function updateConcept(baseURL: string, id: string, input: UpdateConceptInput): Promise<ChargeConcept> {
  const body = {
    name: input.name,
    defaultAmountCents: input.defaultAmountCents,
    recurrence: input.recurrence,
    active: input.active,
  };
  return toConcept(await sendJSON<Wire<ChargeConcept, "recurrence">>(baseURL, `/concepts/${id}`, "PUT", body));
}

/** Active concepts by default; `includeInactive` adds the archived ones. */
export async function listConcepts(baseURL: string, includeInactive = false): Promise<ChargeConcept[]> {
  const qs = query({ includeInactive: includeInactive ? "true" : undefined });
  const rows = await getList<Wire<ChargeConcept, "recurrence">>(baseURL, `/concepts${qs}`, "concepts");
  return rows.map(toConcept);
}

// --- charges --------------------------------------------------------------

export async function createCharge(baseURL: string, input: CreateChargeInput): Promise<Charge> {
  const body = {
    studentId: input.studentId,
    cycleId: input.cycleId,
    conceptId: input.conceptId,
    amountCentsOverride: input.amountCentsOverride,
    descriptionOverride: input.descriptionOverride,
    dueDate: input.dueDate,
  };
  return toCharge(await sendJSON<Wire<Charge, "status">>(baseURL, "/charges", "POST", body));
}

/**
 * Bulk-issue a concept's charge across a cycle (optionally one group). The
 * response is a summary, not the charges — a re-run is idempotent and simply
 * reports the already-charged students as `skipped`.
 */
export async function generateCharges(baseURL: string, input: GenerateChargesInput): Promise<GenerateResult> {
  const body = {
    conceptId: input.conceptId,
    cycleId: input.cycleId,
    groupId: input.groupId,
    amountCentsOverride: input.amountCentsOverride,
    dueDate: input.dueDate,
  };
  const res = await sendJSON<Partial<GenerateResult>>(baseURL, "/charges/generate", "POST", body);
  return { created: res.created ?? 0, skipped: res.skipped ?? 0 };
}

/** Pass a student and/or a cycle: the gateway rejects an unscoped read with 400. */
export async function listCharges(baseURL: string, filters: ChargeFilters = {}): Promise<Charge[]> {
  const qs = query({ studentId: filters.studentId, cycleId: filters.cycleId });
  const rows = await getList<Wire<Charge, "status">>(baseURL, `/charges${qs}`, "charges");
  return rows.map(toCharge);
}

/**
 * Forgive part of a charge (a beca or an adjustment). `discountCents` is integer
 * cents and `reason` is the audit trail the gateway records; the response is the
 * charge with its recomputed remaining balance and status.
 */
export async function applyDiscount(baseURL: string, chargeId: string, discountCents: number, reason: string): Promise<Charge> {
  const body = { discountCents, reason };
  return toCharge(await sendJSON<Wire<Charge, "status">>(baseURL, `/charges/${chargeId}/discount`, "POST", body));
}

/** Void a charge. Returns it with `status: "cancelled"` — no longer collectable. */
export async function cancelCharge(baseURL: string, chargeId: string): Promise<Charge> {
  return toCharge(await postAction<Wire<Charge, "status">>(baseURL, `/charges/${chargeId}/cancel`));
}

/** Charges past their due date with a remaining balance. */
export async function getOverdue(baseURL: string, cycleId?: string): Promise<Charge[]> {
  const rows = await getList<Wire<Charge, "status">>(baseURL, `/overdue${query({ cycleId })}`, "overdue");
  return rows.map(toCharge);
}

// --- payments + account ---------------------------------------------------

/**
 * Register a window payment. The response carries the recomputed charge too, so
 * the caller needs no follow-up read.
 *
 * An `idempotencyKey` makes the POST safe to retry: the gateway replies with the
 * payment already stored rather than collecting a second time. The field is
 * dropped when absent, so a caller that does not deduplicate sends exactly the
 * body it always did.
 */
export async function registerPayment(baseURL: string, input: RegisterPaymentInput): Promise<PaymentResult> {
  const body = {
    chargeId: input.chargeId,
    amountCents: input.amountCents,
    method: input.method,
    reference: input.reference,
    ...(input.idempotencyKey !== undefined && input.idempotencyKey !== ""
      ? { idempotencyKey: input.idempotencyKey }
      : {}),
    // A date like "2026-09-04", omitted when the payment is being captured the
    // same day it was received. The gateway reads an absent paidAt as "now", so
    // leaving it out keeps the old behaviour exactly.
    ...(input.paidAt !== undefined && input.paidAt !== "" ? { paidAt: input.paidAt } : {}),
  };
  const res = await sendJSON<{ payment: Wire<Payment, "method">; charge: Wire<Charge, "status"> }>(
    baseURL,
    "/payments",
    "POST",
    body,
  );
  return { payment: toPayment(res.payment), charge: toCharge(res.charge) };
}

/** A student's statement: charges, payments and the outstanding balance. */
export async function getAccount(baseURL: string, studentId: string): Promise<AccountStatement> {
  const res = await getJSON<{
    charges?: Wire<Charge, "status">[];
    payments?: Wire<Payment, "method">[];
    totalBalanceCents?: number;
    unallocatedCents?: number;
  }>(baseURL, `/accounts/${studentId}`);
  return {
    charges: (res.charges ?? []).map(toCharge),
    payments: (res.payments ?? []).map(toPayment),
    totalBalanceCents: res.totalBalanceCents ?? 0,
    unallocatedCents: res.unallocatedCents ?? 0,
  };
}

// --- deposits (M16) ---------------------------------------------------------

/**
 * Take money without being told what it pays for. The response carries the
 * whole split, so a receipt can be printed without a second call while the
 * parent is still at the counter.
 *
 * Send an `idempotencyKey`: a double click or a retried request must not cost
 * the family twice.
 */
export async function registerDeposit(baseURL: string, input: RegisterDepositInput): Promise<DepositResult> {
  const res = await sendJSON<{
    deposit: Wire<Deposit, "method">;
    allocations?: Wire<Payment, "method">[];
    surcharges?: Wire<Charge, "status">[];
    unallocatedCents?: number;
    replayed?: boolean;
  }>(baseURL, "/deposits", "POST", depositBody(input));
  return {
    deposit: toDeposit(res.deposit),
    allocations: (res.allocations ?? []).map(toPayment),
    surcharges: (res.surcharges ?? []).map(toCharge),
    unallocatedCents: res.unallocatedCents ?? 0,
    replayed: res.replayed ?? false,
  };
}

/**
 * What an amount WOULD cover. Same rule as taking it, and it writes nothing, so
 * the cashier can say it out loud before the money changes hands.
 */
export async function previewDeposit(baseURL: string, input: PreviewDepositInput): Promise<DepositPreview> {
  const res = await sendJSON<{
    allocations?: Wire<Payment, "method">[];
    surcharges?: Wire<Charge, "status">[];
    unallocatedCents?: number;
  }>(baseURL, "/deposits/preview", "POST", {
    studentId: input.studentId,
    amountCents: input.amountCents,
    ...(input.paidAt !== undefined && input.paidAt !== "" ? { paidAt: input.paidAt } : {}),
  });
  return {
    allocations: (res.allocations ?? []).map(toPayment),
    surcharges: (res.surcharges ?? []).map(toCharge),
    unallocatedCents: res.unallocatedCents ?? 0,
  };
}

/** A family's deposits, newest first. */
export async function listDeposits(baseURL: string, studentId: string): Promise<Deposit[]> {
  const rows = await getList<Wire<Deposit, "method">>(baseURL, `/deposits?studentId=${encodeURIComponent(studentId)}`, "deposits");
  return rows.map(toDeposit);
}

/**
 * Cancel a deposit and release what it settled. The reason is required — a
 * cancelled deposit without one is an unexplained hole in the day's takings.
 */
export async function voidDeposit(baseURL: string, id: string, reason: string): Promise<void> {
  const res = await authFetch(url(baseURL, `${BASE_PATH}/deposits/${id}/void`), {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ reason }),
  });
  if (!res.ok) {
    throw await failure(res);
  }
}

/** One day's takings, split by how the money arrived. */
export interface CashUp {
  deposits: Deposit[];
  /** Voided that day. They count for nothing and are shown anyway. */
  cancelled: Deposit[];
  cashCents: number;
  cardCents: number;
  transferCents: number;
  totalCents: number;
}

/**
 * What came in on one day. This is what a school closes the day with, instead
 * of adding up a notebook. An empty `day` means today.
 */
export async function getCashUp(baseURL: string, day: string): Promise<CashUp> {
  const res = await getJSON<{
    deposits?: Wire<Deposit, "method">[];
    cancelled?: Wire<Deposit, "method">[];
    cashCents?: number;
    cardCents?: number;
    transferCents?: number;
    totalCents?: number;
  }>(baseURL, day === "" ? "/cash-up" : `/cash-up?day=${encodeURIComponent(day)}`);
  return {
    deposits: (res.deposits ?? []).map(toDeposit),
    cancelled: (res.cancelled ?? []).map(toDeposit),
    cashCents: res.cashCents ?? 0,
    cardCents: res.cardCents ?? 0,
    transferCents: res.transferCents ?? 0,
    totalCents: res.totalCents ?? 0,
  };
}

/** What one concept earned in a period. */
export interface IncomeLine {
  conceptId: string;
  concept: string;
  amountCents: number;
}

/** A period's income, biggest line first. */
export interface Income {
  lines: IncomeLine[];
  totalCents: number;
}

/**
 * What the school earned between two days, by what it charged for.
 *
 * Counted on the day the money changed hands, not the day the charge was
 * issued: for anyone paying late the two are months apart, and reporting on the
 * charge date credits October with September's arrears.
 */
export async function getIncome(baseURL: string, from: string, to: string): Promise<Income> {
  const res = await getJSON<{ lines?: IncomeLine[]; totalCents?: number }>(
    baseURL,
    `/income?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`,
  );
  return { lines: res.lines ?? [], totalCents: res.totalCents ?? 0 };
}

function depositBody(input: RegisterDepositInput): Record<string, unknown> {
  return {
    studentId: input.studentId,
    amountCents: input.amountCents,
    method: input.method,
    reference: input.reference,
    // Omitted when the money is captured the day it arrived: the gateway reads
    // an absent paidAt as "today".
    ...(input.paidAt !== undefined && input.paidAt !== "" ? { paidAt: input.paidAt } : {}),
    ...(input.idempotencyKey !== undefined && input.idempotencyKey !== ""
      ? { idempotencyKey: input.idempotencyKey }
      : {}),
  };
}

function toDeposit(d: Wire<Deposit, "method">): Deposit {
  return {
    id: d.id,
    studentId: d.studentId,
    amountCents: d.amountCents,
    method: d.method as PaymentMethod,
    reference: d.reference ?? "",
    paidAt: d.paidAt,
    calendarVersion: d.calendarVersion ?? "",
    ...(d.voidedAt !== undefined ? { voidedAt: d.voidedAt } : {}),
    ...(d.voidReason !== undefined ? { voidReason: d.voidReason } : {}),
  };
}

// --- tuition (M15) ----------------------------------------------------------

/** What one level costs in one cycle. Every amount is integer cents. */
export interface TuitionPlan {
  id: string;
  cycleId: string;
  levelId: string;
  enrollmentCents: number;
  monthlyCents: number;
  /** How many monthly payments the year has, and which month the first falls in. */
  months: number;
  firstBilledMonth: number;
  /** Day of the month a payment falls due. 1-28, so it exists in February too. */
  dueDay: number;
  /** Fixed surcharge added once per completed overdue month. Zero disables it. */
  lateFeeCents: number;
  active: boolean;
}

export type TuitionPlanInput = Omit<TuitionPlan, "id">;

/** What a billing run did, as the person who pressed the button reads it. */
export interface BillingRun {
  tuitionCreated: number;
  enrollmentCreated: number;
  lateFeesCreated: number;
  /** Already existed. On a second press this is everything — that IS the proof. */
  skipped: number;
  /** Levels nobody has priced. Reported, never billed at zero. */
  levelsWithoutPlan: string[];
}

/** Sets what one level of one cycle costs, replacing whatever was there. */
export async function upsertTuitionPlan(baseURL: string, input: TuitionPlanInput): Promise<TuitionPlan> {
  return sendJSON<TuitionPlan>(baseURL, "/tuition-plans", "PUT", input);
}

/** The whole tuition table of a cycle. */
export async function listTuitionPlans(baseURL: string, cycleId: string): Promise<TuitionPlan[]> {
  return getList<TuitionPlan>(baseURL, `/tuition-plans?cycleId=${encodeURIComponent(cycleId)}`, "plans");
}

/**
 * Generates what the school is owed as of a date.
 *
 * Safe to press twice: the run is idempotent downstream, so a second press
 * reports everything as skipped rather than charging anybody again.
 */
export async function runBilling(baseURL: string, cycleId: string, asOfDate = ""): Promise<BillingRun> {
  return sendJSON<BillingRun>(baseURL, "/billing-runs", "POST", { cycleId, asOfDate });
}
