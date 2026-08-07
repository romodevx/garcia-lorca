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
export type Recurrence = "once" | "monthly";

export interface ChargeConcept {
  id: string;
  name: string;
  defaultAmountCents: number;
  recurrence: string;
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
  status: string;
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
  method: string;
  reference: string;
  paidAt: string;
}

/** A student's full statement: what was charged, what was paid, what is left. */
export interface AccountStatement {
  charges: Charge[];
  payments: Payment[];
  totalBalanceCents: number;
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
  recurrence: string;
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
  method: string;
  reference: string;
}

/** Optional list filters; empty values are dropped from the query string. */
export interface ChargeFilters {
  studentId?: string;
  cycleId?: string;
}

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
  const res = await fetch(url(baseURL, `${BASE_PATH}${path}`), {
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
  const res = await fetch(url(baseURL, `${BASE_PATH}${path}`), { method: "POST", credentials: "include" });
  if (!res.ok) {
    throw await failure(res);
  }
  return (await res.json()) as T;
}

/** GET and return the parsed payload. Throws on any non-2xx. */
async function getJSON<T>(baseURL: string, path: string): Promise<T> {
  const res = await fetch(url(baseURL, `${BASE_PATH}${path}`), { credentials: "include" });
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

export function createConcept(baseURL: string, input: CreateConceptInput): Promise<ChargeConcept> {
  const body = {
    name: input.name,
    defaultAmountCents: input.defaultAmountCents,
    recurrence: input.recurrence,
  };
  return sendJSON<ChargeConcept>(baseURL, "/concepts", "POST", body);
}

/** Full replace of a concept's mutable fields, `active` included. */
export function updateConcept(baseURL: string, id: string, input: UpdateConceptInput): Promise<ChargeConcept> {
  const body = {
    name: input.name,
    defaultAmountCents: input.defaultAmountCents,
    recurrence: input.recurrence,
    active: input.active,
  };
  return sendJSON<ChargeConcept>(baseURL, `/concepts/${id}`, "PUT", body);
}

/** Active concepts by default; `includeInactive` adds the archived ones. */
export function listConcepts(baseURL: string, includeInactive = false): Promise<ChargeConcept[]> {
  const qs = query({ includeInactive: includeInactive ? "true" : undefined });
  return getList<ChargeConcept>(baseURL, `/concepts${qs}`, "concepts");
}

// --- charges --------------------------------------------------------------

export function createCharge(baseURL: string, input: CreateChargeInput): Promise<Charge> {
  const body = {
    studentId: input.studentId,
    cycleId: input.cycleId,
    conceptId: input.conceptId,
    amountCentsOverride: input.amountCentsOverride,
    descriptionOverride: input.descriptionOverride,
    dueDate: input.dueDate,
  };
  return sendJSON<Charge>(baseURL, "/charges", "POST", body);
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

export function listCharges(baseURL: string, filters: ChargeFilters = {}): Promise<Charge[]> {
  const qs = query({ studentId: filters.studentId, cycleId: filters.cycleId });
  return getList<Charge>(baseURL, `/charges${qs}`, "charges");
}

export function applyDiscount(baseURL: string, chargeId: string, discountCents: number, reason: string): Promise<Charge> {
  return sendJSON<Charge>(baseURL, `/charges/${chargeId}/discount`, "POST", { discountCents, reason });
}

export function cancelCharge(baseURL: string, chargeId: string): Promise<Charge> {
  return postAction<Charge>(baseURL, `/charges/${chargeId}/cancel`);
}

/** Charges past their due date with a remaining balance. */
export function getOverdue(baseURL: string, cycleId?: string): Promise<Charge[]> {
  return getList<Charge>(baseURL, `/overdue${query({ cycleId })}`, "overdue");
}

// --- payments + account ---------------------------------------------------

/**
 * Register a window payment. The response carries the recomputed charge too, so
 * the caller needs no follow-up read.
 */
export function registerPayment(baseURL: string, input: RegisterPaymentInput): Promise<PaymentResult> {
  const body = {
    chargeId: input.chargeId,
    amountCents: input.amountCents,
    method: input.method,
    reference: input.reference,
  };
  return sendJSON<PaymentResult>(baseURL, "/payments", "POST", body);
}

/** A student's statement: charges, payments and the outstanding balance. */
export async function getAccount(baseURL: string, studentId: string): Promise<AccountStatement> {
  const res = await getJSON<Partial<AccountStatement>>(baseURL, `/accounts/${studentId}`);
  return {
    charges: res.charges ?? [],
    payments: res.payments ?? [],
    totalBalanceCents: res.totalBalanceCents ?? 0,
  };
}
