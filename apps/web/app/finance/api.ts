/**
 * Typed fetch helpers for the gateway's finance endpoints
 * (`/api/v1/finance/*`), gated behind the `module_finance` flag + cookie auth.
 * The session lives ONLY in the HttpOnly cookie the gateway set, so every call
 * uses `credentials:"include"`; nothing secret ever travels in the body.
 *
 * Reads require `finance:read`, writes `finance:write` (the owner role holds
 * both); the counter's routes also answer to `collections:read` and
 * `payments:write`, the Cobranza role (task 9, ADR 0015). These helpers mirror `app/grades/api.ts`: a `url()` trailing-slash
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
import {
  fromWireExpense,
  fromWireTotals,
  toClosure,
  type CashCutClosure,
  type Expense,
  type MethodTotals,
  type WireExpense,
} from "../expenses/api";

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

/**
 * How the money came in at the window. A credit note (T8) is not money — the
 * school honouring a balance it owes the family — but it settles debt the same
 * way, so it is taken as one more method and kept apart in every money total.
 */
export type PaymentMethod = "cash" | "card" | "transfer" | "credit_note";

/** What a deposit records as its method: one of the above, or `mixed` when it was split. */
export type DepositMethod = PaymentMethod | "mixed";

/**
 * Which kind of card. Not a payment method — the money still arrives by card —
 * but the terminal the office reconciles against at month end knows the
 * difference, so the ledger records it.
 */
export type CardType = "debit" | "credit";

/**
 * One part of a payment (T8): what one method covered. A payment split across
 * a credit note and cash travels as two tenders whose amounts add up to it.
 */
export interface Tender {
  method: PaymentMethod;
  amountCents: number;
  /** The credit note's folio, the voucher, the transfer id. Optional. */
  reference?: string;
  /** Only on a card. */
  cardType?: CardType;
}

export interface Payment {
  id: string;
  chargeId: string;
  amountCents: number;
  method: PaymentMethod;
  reference: string;
  paidAt: string;
  /** The receipt this payment is a slice of (T9); absent when paid straight against its charge. */
  depositId?: string;
}

/** A student's full statement: what was charged, what was paid, what is left. */
export interface AccountStatement {
  charges: Charge[];
  payments: Payment[];
  totalBalanceCents: number;
  /** Money the family has handed over that no charge has claimed. */
  unallocatedCents: number;
  /**
   * What the school is asking for TODAY: the months that have come due, already
   * carrying the surcharge that has accrued and net of the early-payment reward.
   *
   * `totalBalanceCents` above is what the YEAR still owes. Both are true at once,
   * and showing only the first tells a family a number nobody is asking them for.
   */
  dueTodayCents: number;
  /** Its parts, so the screen can explain its own figure. */
  surchargeCents: number;
  earlyDiscountCents: number;
  /** The day those three were worked out for. They move overnight. */
  asOf: string;
}

/**
 * Money as it arrived, before it was decided what it pays. The counter records
 * ONE amount; which months it covers is worked out server-side, oldest first.
 */
export interface Deposit {
  id: string;
  studentId: string;
  amountCents: number;
  /** `mixed` when it was paid with several methods: read `tenders`, not this. */
  method: DepositMethod;
  reference: string;
  paidAt: string;
  /** How it was paid, method by method (T8). Always at least one. */
  tenders: Tender[];
  /**
   * The school calendar the split was worked out against. "unavailable" means
   * the calendar could not be read and the deadline was computed without the
   * school's holidays — worth showing on a receipt somebody may dispute.
   */
  calendarVersion: string;
  /** debit or credit on a card payment; absent otherwise. */
  cardType?: CardType;
  voidedAt?: string;
  voidReason?: string;
  /** The receipt number, P-<year>-<six digits> (task 12). */
  folio?: string;
  /** Who took the money, as the name read then (task 7). Absent on older deposits. */
  receivedByName?: string;
  /** When it was keyed; a later day than paidAt means it was captured late. */
  capturedAt?: string;
  /** T9: a correction replaced it — kept, counting for nothing. */
  supersededAt?: string;
  supersededBy?: string;
  supersededByFolio?: string;
  /** T9: on a correction, the receipt it replaces. */
  correctsDepositId?: string;
  correctsFolio?: string;
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
  /**
   * Honours the early-payment reward after the window closed, because the
   * school decided to — the parent who is always punctual and slipped once.
   * Recorded under its own discount reason, so a report can always tell a
   * decision from the rule firing.
   */
  grantEarlyPayment?: boolean;
  /** debit or credit. Only meaningful when the method is card. */
  cardType?: CardType;
  /** The charges the money may pay, chosen at the counter (task 8); absent means all. */
  chargeIds?: string[];
  /** The payment split by method (T8); when present the server ignores method/reference/cardType. */
  tenders?: Tender[];
}

/** What a family must hand over today to be current. */
export interface Expected {
  amountCents: number;
  surchargeCents: number;
  discountCents: number;
  /** The whole year still owed — not the same question as what is due today. */
  annualBalanceCents: number;
  /** The debt concept by concept, oldest due date first (task 8). */
  lines: ExpectedLine[];
}

/** One charge still owed, as of the day asked about. */
export interface ExpectedLine {
  chargeId: string;
  description: string;
  /** "2026-09-10" */
  dueDate: string;
  owedCents: number;
  /** What being late has cost it that day. */
  surchargeCents: number;
  /** The early-payment reward settling it that day earns. */
  discountCents: number;
  totalCents: number;
  /** Asked for that day: due, or falling due later that month. */
  due: boolean;
}

export interface PreviewDepositInput {
  studentId: string;
  amountCents: number;
  paidAt?: string;
  /** The charges the money may pay; absent means all, oldest first. */
  chargeIds?: string[];
}

/** The payment plus the charge with its recomputed paid/remaining/status. */
export interface PaymentResult {
  payment: Payment;
  /** One per tender (T8); the single payment when it was not split. */
  payments: Payment[];
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
  /** The payment split by method (T8): their sum is `amountCents`. */
  tenders?: Tender[];
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
const PAYMENT_METHODS: readonly PaymentMethod[] = ["cash", "card", "transfer", "credit_note"];
const DEPOSIT_METHODS: readonly DepositMethod[] = [...PAYMENT_METHODS, "mixed"];
const CARD_TYPES: readonly CardType[] = ["debit", "credit"];

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
    ...(input.tenders !== undefined && input.tenders.length > 0 ? { tenders: input.tenders } : {}),
  };
  const res = await sendJSON<{
    payment: Wire<Payment, "method">;
    payments?: Wire<Payment, "method">[];
    charge: Wire<Charge, "status">;
  }>(baseURL, "/payments", "POST", body);
  const payment = toPayment(res.payment);
  return {
    payment,
    // A gateway older than T8 sends only the one payment.
    payments: res.payments !== undefined && res.payments.length > 0 ? res.payments.map(toPayment) : [payment],
    charge: toCharge(res.charge),
  };
}

/** A student's statement: charges, payments and the outstanding balance. */
export async function getAccount(baseURL: string, studentId: string): Promise<AccountStatement> {
  const res = await getJSON<{
    charges?: Wire<Charge, "status">[];
    payments?: Wire<Payment, "method">[];
    totalBalanceCents?: number;
    unallocatedCents?: number;
    dueTodayCents?: number;
    surchargeCents?: number;
    earlyDiscountCents?: number;
    asOf?: string;
  }>(baseURL, `/accounts/${studentId}`);
  return {
    charges: (res.charges ?? []).map(toCharge),
    payments: (res.payments ?? []).map(toPayment),
    totalBalanceCents: res.totalBalanceCents ?? 0,
    unallocatedCents: res.unallocatedCents ?? 0,
    // A gateway written before M18 sends none of these. Zero would read as "the
    // family owes nothing today", so the screen is told the truth — that the day
    // is unknown — through an empty asOf, and decides for itself what to show.
    dueTodayCents: res.dueTodayCents ?? 0,
    surchargeCents: res.surchargeCents ?? 0,
    earlyDiscountCents: res.earlyDiscountCents ?? 0,
    asOf: res.asOf ?? "",
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
    deposit: WireDeposit;
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
    ...(input.chargeIds !== undefined && input.chargeIds.length > 0 ? { chargeIds: input.chargeIds } : {}),
  });
  return {
    allocations: (res.allocations ?? []).map(toPayment),
    surcharges: (res.surcharges ?? []).map(toCharge),
    unallocatedCents: res.unallocatedCents ?? 0,
  };
}

/**
 * What a family must hand over today to be current: the months already due
 * (and the one due later this month, which is how the early-payment reward is
 * earned), what being late has cost, and what paying early saves.
 */
export async function getExpected(baseURL: string, studentId: string, on = ""): Promise<Expected> {
  const query = on === "" ? "" : `&on=${encodeURIComponent(on)}`;
  const res = await getJSON<Partial<Expected>>(baseURL, `/expected?studentId=${encodeURIComponent(studentId)}${query}`);
  return {
    amountCents: res.amountCents ?? 0,
    surchargeCents: res.surchargeCents ?? 0,
    discountCents: res.discountCents ?? 0,
    annualBalanceCents: res.annualBalanceCents ?? 0,
    lines: res.lines ?? [],
  };
}

/** A family's deposits, newest first. */
export async function listDeposits(baseURL: string, studentId: string): Promise<Deposit[]> {
  const rows = await getList<WireDeposit>(baseURL, `/deposits?studentId=${encodeURIComponent(studentId)}`, "deposits");
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
  /** Replaced by a correction (T9). They count for nothing; the correction counts on its own day. */
  superseded: Deposit[];
  /** Taken outside a deposit (a counter sale's). The totals already count them. */
  payments: CashUpPayment[];
  cashCents: number;
  cardCents: number;
  transferCents: number;
  /** Money only: cash + card + transfer. Credit notes are not in it. */
  totalCents: number;
  /** Credit notes taken that day (T8). NOT money: in no other figure here. */
  creditNoteCents: number;
  /** T10 — the day's expenses (cancelled ones listed, not counted). */
  expenses?: Expense[];
  expensesByMethod?: MethodTotals;
  /** T10 — the cash the drawer should hold: cash in − cash out. */
  expectedCashCents?: number;
  /** T10 — the day's live closure; absent while the day is open. */
  closure?: CashCutClosure;
  /** T10 — a movement of the day changed after it was closed. */
  modifiedAfterClose?: boolean;
  /** T10 — earlier closures of the day that were reopened, newest first. */
  reopened?: CashCutClosure[];
}

/** A payment taken outside a deposit, with what it paid for. */
export interface CashUpPayment {
  id: string;
  chargeId: string;
  amountCents: number;
  method: PaymentMethod;
  reference: string;
  paidAt: string;
  description: string;
}

/**
 * What came in on one day. This is what a school closes the day with, instead
 * of adding up a notebook. An empty `day` means today.
 */
export async function getCashUp(baseURL: string, day: string): Promise<CashUp> {
  const res = await getJSON<{
    deposits?: WireDeposit[];
    cancelled?: WireDeposit[];
    superseded?: WireDeposit[];
    payments?: Wire<CashUpPayment, "method">[];
    cashCents?: number;
    cardCents?: number;
    transferCents?: number;
    totalCents?: number;
    creditNoteCents?: number;
    expenses?: WireExpense[];
    expensesByMethod?: Partial<MethodTotals>;
    expectedCashCents?: number;
    closure?: CashCutClosure;
    modifiedAfterClose?: boolean;
    reopened?: CashCutClosure[];
  }>(baseURL, day === "" ? "/cash-up" : `/cash-up?day=${encodeURIComponent(day)}`);
  return {
    deposits: (res.deposits ?? []).map(toDeposit),
    cancelled: (res.cancelled ?? []).map(toDeposit),
    superseded: (res.superseded ?? []).map(toDeposit),
    payments: (res.payments ?? []).map((p) => ({ ...p, method: oneOf(PAYMENT_METHODS, p.method, "cash") })),
    cashCents: res.cashCents ?? 0,
    cardCents: res.cardCents ?? 0,
    transferCents: res.transferCents ?? 0,
    totalCents: res.totalCents ?? 0,
    creditNoteCents: res.creditNoteCents ?? 0,
    expenses: (res.expenses ?? []).map(fromWireExpense),
    expensesByMethod: fromWireTotals(res.expensesByMethod),
    expectedCashCents: res.expectedCashCents ?? res.cashCents ?? 0,
    ...(res.closure !== undefined ? { closure: toClosure(res.closure) } : {}),
    modifiedAfterClose: res.modifiedAfterClose ?? false,
    reopened: (res.reopened ?? []).map(toClosure),
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
  /** Money only: the lines exclude credit notes. */
  totalCents: number;
  /** Credit notes applied in the period (T8), apart from the money. */
  creditNoteCents: number;
}

/**
 * What the school earned between two days, by what it charged for.
 *
 * Counted on the day the money changed hands, not the day the charge was
 * issued: for anyone paying late the two are months apart, and reporting on the
 * charge date credits October with September's arrears.
 */
export async function getIncome(baseURL: string, from: string, to: string): Promise<Income> {
  const res = await getJSON<{ lines?: IncomeLine[]; totalCents?: number; creditNoteCents?: number }>(
    baseURL,
    `/income?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`,
  );
  return { lines: res.lines ?? [], totalCents: res.totalCents ?? 0, creditNoteCents: res.creditNoteCents ?? 0 };
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
    ...(input.grantEarlyPayment === true ? { grantEarlyPayment: true } : {}),
    // Sent only on a card: the server refuses a card type on cash, and rightly.
    ...(input.method === "card" && input.cardType !== undefined ? { cardType: input.cardType } : {}),
    // Sent only when the cashier chose: absent keeps "oldest debt first".
    ...(input.chargeIds !== undefined && input.chargeIds.length > 0 ? { chargeIds: input.chargeIds } : {}),
    ...(input.tenders !== undefined && input.tenders.length > 0 ? { tenders: input.tenders } : {}),
  };
}

type WireTender = Omit<Tender, "method" | "cardType"> & { method: string; cardType?: string };
type WireDeposit = Omit<Wire<Deposit, "method">, "tenders"> & { tenders?: WireTender[] };

const toTender = (t: WireTender): Tender => {
  const method = oneOf(PAYMENT_METHODS, t.method, "cash");
  return {
    method,
    amountCents: t.amountCents,
    ...(t.reference !== undefined && t.reference !== "" ? { reference: t.reference } : {}),
    ...(method === "card" && t.cardType !== undefined && t.cardType !== "" ? { cardType: oneOf(CARD_TYPES, t.cardType, "debit") } : {}),
  };
};

/**
 * The deposit's tenders. A gateway older than T8 sends none; the deposit was
 * then paid with its one method, which is exactly one tender.
 */
function tendersOf(d: WireDeposit): Tender[] {
  if (d.tenders !== undefined && d.tenders.length > 0) return d.tenders.map(toTender);
  return [toTender({ method: d.method, amountCents: d.amountCents, reference: d.reference, cardType: d.cardType })];
}

function toDeposit(d: WireDeposit): Deposit {
  return {
    id: d.id,
    studentId: d.studentId,
    amountCents: d.amountCents,
    method: oneOf(DEPOSIT_METHODS, d.method, "cash"),
    reference: d.reference ?? "",
    paidAt: d.paidAt,
    tenders: tendersOf(d),
    calendarVersion: d.calendarVersion ?? "",
    // The wire sends "" for cash and transfers; the type says "absent".
    ...(d.cardType !== undefined && String(d.cardType) !== "" ? { cardType: d.cardType } : {}),
    ...(d.voidedAt !== undefined ? { voidedAt: d.voidedAt } : {}),
    ...(d.voidReason !== undefined ? { voidReason: d.voidReason } : {}),
    // Who took the money and when it was keyed (task 7): the cash-up's
    // "capturado el …" mark reads these.
    ...(d.receivedByName !== undefined && d.receivedByName !== "" ? { receivedByName: d.receivedByName } : {}),
    ...(d.capturedAt !== undefined && d.capturedAt !== "" ? { capturedAt: d.capturedAt } : {}),
    ...(d.folio !== undefined && d.folio !== "" ? { folio: d.folio } : {}),
    ...present("supersededAt", d.supersededAt),
    ...present("supersededBy", d.supersededBy),
    ...present("supersededByFolio", d.supersededByFolio),
    ...present("correctsDepositId", d.correctsDepositId),
    ...present("correctsFolio", d.correctsFolio),
  };
}

/** `{ key: value }` when the wire sent a value, `{}` when it sent nothing or "". */
function present<K extends string>(key: K, value: string | undefined): Partial<Record<K, string>> {
  return value !== undefined && value !== "" ? ({ [key]: value } as Record<K, string>) : {};
}

// --- corrections (T9) -------------------------------------------------------

/** A payment correction as the office types it. */
export interface CorrectDepositInput {
  amountCents: number;
  /** aaaa-mm-dd; "" keeps the original's day. */
  paidAt: string;
  tenders: Tender[];
  /** The charges the money may pay; empty is oldest debt first. */
  chargeIds: string[];
  reason: string;
  /** One per correction ATTEMPT, reused across its retries. */
  idempotencyKey: string;
}

/** What one side of a correction put on a charge. */
export interface SnapshotLine {
  chargeId: string;
  description?: string;
  amountCents: number;
}

/** A receipt as it stood on one side of a correction. */
export interface DepositSnapshot {
  depositId: string;
  folio: string;
  amountCents: number;
  paidAt?: string;
  tenders: Tender[];
  lines: SnapshotLine[];
}

/** One correction: before, after, who, when and why. */
export interface DepositCorrection {
  id: string;
  originalDepositId: string;
  correctedDepositId: string;
  reason: string;
  before: DepositSnapshot;
  after: DepositSnapshot;
  correctedByName?: string;
  correctedAt: string;
}

/** What a correction did. */
export interface CorrectionResult {
  original: Deposit;
  deposit: Deposit;
  allocations: Payment[];
  unallocatedCents: number;
  replayed: boolean;
  correction: DepositCorrection;
}

type WireSnapshot = Partial<Omit<DepositSnapshot, "tenders">> & { tenders?: WireTender[] };
type WireCorrection = Partial<Omit<DepositCorrection, "before" | "after">> & { before?: WireSnapshot; after?: WireSnapshot };

const toSnapshot = (s: WireSnapshot | undefined): DepositSnapshot => ({
  depositId: s?.depositId ?? "",
  folio: s?.folio ?? "",
  amountCents: s?.amountCents ?? 0,
  ...present("paidAt", s?.paidAt),
  tenders: (s?.tenders ?? []).map(toTender),
  lines: s?.lines ?? [],
});

const toCorrection = (c: WireCorrection): DepositCorrection => ({
  id: c.id ?? "",
  originalDepositId: c.originalDepositId ?? "",
  correctedDepositId: c.correctedDepositId ?? "",
  reason: c.reason ?? "",
  before: toSnapshot(c.before),
  after: toSnapshot(c.after),
  ...present("correctedByName", c.correctedByName),
  correctedAt: c.correctedAt ?? "",
});

// Deposit ids are uuids; anything else never reaches the network (it would be a path).
const DEPOSIT_ID = /^[0-9a-fA-F-]{36}$/;

function depositPath(id: string, suffix: string): string {
  if (!DEPOSIT_ID.test(id)) {
    throw new Error("Ese pago no existe.");
  }
  return `/deposits/${encodeURIComponent(id)}/${suffix}`;
}

/**
 * Correct a registered payment (T9, payments:edit). The original receipt is
 * kept, superseded; the server creates the corrected one with its own folio,
 * re-splits it on the corrected date and tells the family.
 */
export async function correctDeposit(baseURL: string, id: string, input: CorrectDepositInput): Promise<CorrectionResult> {
  const res = await sendJSON<{
    original: WireDeposit;
    deposit: WireDeposit;
    allocations?: Wire<Payment, "method">[];
    unallocatedCents?: number;
    replayed?: boolean;
    correction?: WireCorrection;
  }>(baseURL, depositPath(id, "correct"), "POST", {
    amountCents: input.amountCents,
    ...(input.paidAt !== "" ? { paidAt: input.paidAt } : {}),
    reason: input.reason,
    idempotencyKey: input.idempotencyKey,
    tenders: input.tenders,
    ...(input.chargeIds.length > 0 ? { chargeIds: input.chargeIds } : {}),
  });
  return {
    original: toDeposit(res.original),
    deposit: toDeposit(res.deposit),
    allocations: (res.allocations ?? []).map(toPayment),
    unallocatedCents: res.unallocatedCents ?? 0,
    replayed: res.replayed ?? false,
    correction: toCorrection(res.correction ?? {}),
  };
}

/** A receipt's correction history, oldest first, whichever link is asked about. */
export async function listDepositCorrections(baseURL: string, id: string): Promise<DepositCorrection[]> {
  const rows = await getList<WireCorrection>(baseURL, depositPath(id, "corrections"), "corrections");
  return rows.map(toCorrection);
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
  policy: CollectionPolicy;
  /**
   * An explicit payment plan, replacing the uniform monthly amount. Empty means
   * the year is divided into equal parts, which is what most schools do.
   */
  schedule: Installment[];
  /**
   * A fixed amount off every month of this level — the school's arrangement for
   * maternal, or for any level it prices differently. Set once here rather than
   * typed per pupil, which is the difference between a rule and four hundred
   * chances to mistype it.
   */
  monthlyDiscountCents: number;
  /**
   * The extras the school collects on top of the tuition, each split across the
   * months it chose. Empty is the ordinary state.
   */
  fees: Fee[];
  /**
   * The deadline the school set for the inscription (YYYY-MM-DD). Empty keeps
   * the default rule — the due day in the cycle's first month.
   */
  enrollmentDueOn: string;
  /**
   * The first day the inscription opens to families (YYYY-MM-DD, T5): before it
   * the parents' app does not show it and the counter does not ask for it.
   * Empty keeps it open from the day it is issued. Needs `enrollmentDueOn` and
   * may not come after it.
   */
  enrollmentStartsOn: string;
  /**
   * The lines the inscription is broken into, each with its own price ladder.
   * Empty keeps `enrollmentCents`; the two are mutually exclusive (ADR 0011).
   */
  enrollmentConcepts: EnrollmentConcept[];
}

/**
 * One line of the inscription: a name the family reads and what it costs
 * depending on WHEN they settle it.
 *
 * Deliberately NOT a Fee. A Fee is one amount divided between months; this is
 * one amount that CHANGES with the month — the school's price list doubles the
 * inscription between January and April, and that ladder is the whole incentive
 * to join early.
 */
export interface EnrollmentConcept {
  name: string;
  /** The ladder, ordered by date. It only ever climbs. */
  tiers: PriceTier[];
}

/**
 * What a concept costs between two dates (YYYY-MM-DD).
 *
 * `effectiveTo` may be empty: a price the school has not decided to end. It does
 * NOT bound what is charged — a day no range covers is still priced, at the last
 * amount that took effect — but it is how a price list is written, and two
 * ranges that overlap are a slip worth refusing.
 */
export interface PriceTier {
  effectiveFrom: string;
  effectiveTo: string;
  amountCents: number;
}

/**
 * Money the school collects on top of the monthly tuition, spread across the
 * months it chooses.
 *
 * A school's year and its calendar do not agree: "la cuota de julio" is charged
 * for a month the year does not bill, so it is collected inside months that it
 * does. `amountCents` is the WHOLE fee, not the monthly share — the school says
 * what it collects and the server divides it, so nobody is doing arithmetic in
 * a form field.
 */
export interface Fee {
  /** What the family reads, and the whole explanation of a dearer month. */
  name: string;
  amountCents: number;
  /** The calendar months it is divided between, in the order chosen. */
  months: number[];
}

/**
 * One payment of the year: the month it falls in and how much of the tuition it
 * collects.
 *
 * A school does not always divide the year into equal parts. "Once meses, pero
 * diciembre a la mitad y la otra mitad en marzo" is a real arrangement —
 * families are short in December — and a plan that can only say "N equal
 * payments" cannot express it.
 */
export interface Installment {
  /** 1-12. The year comes from the cycle, so a plan may run past December. */
  month: number;
  amountCents: number;
}

/**
 * What a late payment costs and what an early one saves. It rides with the
 * tuition table because that is where a price lives, and it is per level: a
 * school may treat maternal differently from preparatoria.
 */
export interface CollectionPolicy {
  /**
   * Picks the rule. false keeps the fixed monthly surcharge above; true charges
   * `lateFeeDailyCents` for every day the money is late, and the monthly run
   * stops issuing surcharge charges — the amount changes every night, so it is
   * worked out at the moment of payment instead.
   */
  dailyMode: boolean;
  lateFeeDailyCents: number;
  /** Counts Saturdays, Sundays and holidays: the money is late on a Sunday too. */
  surchargeCountsAllDays: boolean;
  /** A fixed amount off, per month settled, inside the early window. */
  earlyPaymentCents: number;
  /** How many WORKING days that window lasts, from the first of the month. */
  earlyPaymentDays: number;
}

export type TuitionPlanInput = Omit<TuitionPlan, "id">;

/** What a billing run did, as the person who pressed the button reads it. */
export interface BillingRun {
  tuitionCreated: number;
  enrollmentCreated: number;
  lateFeesCreated: number;
  /** Charges issued for the extras collected on top of the tuition. */
  feesCreated: number;
  /**
   * Inscriptions whose deadline the run corrected to match the tuition table.
   * The run never re-issues a charge it has written, so this is how a deadline
   * changed after billing reaches pupils who were already charged.
   */
  enrollmentMoved: number;
  /** Already existed. On a second press this is everything — that IS the proof. */
  skipped: number;
  /** Levels nobody has priced. Reported, never billed at zero. */
  levelsWithoutPlan: string[];
}

/** Sets what one level of one cycle costs, replacing whatever was there. */
export async function upsertTuitionPlan(baseURL: string, input: TuitionPlanInput): Promise<TuitionPlan> {
  return sendJSON<TuitionPlan>(baseURL, "/tuition-plans", "PUT", input);
}

/**
 * The whole tuition table of a cycle.
 *
 * A plan stored before M16 comes back without a policy, so it is filled in with
 * the zero rule — the fixed monthly surcharge it already had. A missing field
 * must not read as "daily surcharge of nothing".
 */
export async function listTuitionPlans(baseURL: string, cycleId: string): Promise<TuitionPlan[]> {
  const plans = await getList<
    Partial<TuitionPlan> &
      Omit<
        TuitionPlan,
        | "policy"
        | "schedule"
        | "monthlyDiscountCents"
        | "fees"
        | "enrollmentDueOn"
        | "enrollmentStartsOn"
        | "enrollmentConcepts"
      >
  >(baseURL, `/tuition-plans?cycleId=${encodeURIComponent(cycleId)}`, "plans");
  return plans.map((p) => ({
    ...p,
    policy: p.policy ?? emptyPolicy(),
    schedule: p.schedule ?? [],
    monthlyDiscountCents: p.monthlyDiscountCents ?? 0,
    fees: p.fees ?? [],
    enrollmentDueOn: p.enrollmentDueOn ?? "",
    // A plan stored before T5 has no start: open from the day it is issued.
    enrollmentStartsOn: p.enrollmentStartsOn ?? "",
    // A plan stored before M18 comes back without them, which is the
    // single-amount inscription it has always had.
    enrollmentConcepts: p.enrollmentConcepts ?? [],
  }));
}

function emptyPolicy(): CollectionPolicy {
  return {
    dailyMode: false,
    lateFeeDailyCents: 0,
    surchargeCountsAllDays: true,
    earlyPaymentCents: 0,
    earlyPaymentDays: 0,
  };
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
