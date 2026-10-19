/**
 * Typed fetch helpers for the gateway's uniform shop (`/api/v1/uniforms/*`),
 * gated behind the `module_uniforms` flag + cookie auth. The session lives ONLY
 * in the HttpOnly cookie the gateway set, so every call uses
 * `credentials:"include"`; nothing secret travels in a body.
 *
 * THE PERMISSIONS ARE SPLIT, and the split shows up here as two groups of calls:
 * the catalogue and the storeroom need `uniforms:stock`, which resolves no
 * student data; the order list needs `uniforms:orders`, which carries every
 * pupil's name and is owner-only. Writes need `uniforms:write`, and the counter
 * sale additionally needs `finance:write`. This client is UX only — the gateway
 * is the security boundary and will refuse regardless of what the UI renders.
 *
 * MONEY IS ALWAYS INTEGER CENTS on the wire, as everywhere else. Pesos exist
 * only in the formatting layer; no float reaches this module.
 *
 * `available` and `belowMinimum` are DERIVED SERVER-SIDE and never computed
 * here. Available is the only number a parent should ever see, and two places
 * deriving it is two chances to disagree with the till.
 */
import { GENERIC_SERVER_FAILURE, spanishServerReason } from "../_lib/serverReason";
import { authFetch } from "../auth/authFetch";
import type { PaymentMethod, Tender } from "../finance/api";

const BASE_PATH = "/api/v1/uniforms";

// --- entity types (match the gateway JSON contract) ------------------------

/**
 * A school level: the id of a level the school defined in Configuración (not a
 * fixed SEP list). Empty on a GARMENT means it applies to every level.
 */
export type Level = string;

/** One garment in one size. */
export interface UniformSize {
  id: string;
  itemId: string;
  sizeLabel: string;
  sizeOrder: number;
  /** Physically in the storeroom. */
  onHand: number;
  /** Committed to a live order line. */
  reserved: number;
  /** onHand − reserved. Server-derived; the only number to show a parent. */
  available: number;
  minStock: number;
  /** Server-derived: measured on what can still be sold, not what is on the shelf. */
  belowMinimum: boolean;
}

/** One garment in the catalogue, with its size run. */
export interface UniformItem {
  id: string;
  name: string;
  description: string;
  /** A config level id; empty means the garment applies to every level. */
  levelId: Level;
  priceCents: number;
  active: boolean;
  sizes: UniformSize[];
  /**
   * What kind of thing this is. The module sells whatever the school sells —
   * the stock rules, the ledger and the counter sale were never about clothing,
   * only the vocabulary was.
   */
  category: ItemCategory;
}

/** uniform | book | supply | other. Everything stored first was a uniform. */
export type ItemCategory = "uniform" | "book" | "supply" | "other";

/**
 * What one garment in one size is doing. `reserved` is the only status holding
 * stock; `released` and `cancelled` are terminal — a line the school reclaimed
 * is not picked up again, the family orders afresh.
 */
export type LineStatus = "awaiting_stock" | "reserved" | "sold" | "released" | "cancelled";

/** One garment in one order. */
export interface UniformOrderLine {
  id: string;
  itemId: string;
  itemName: string;
  sizeLabel: string;
  qty: number;
  unitPriceCents: number;
  status: LineStatus;
  fulfillmentId?: string;
}

/**
 * The state of one delivery. `sold_at_counter` is terminal from birth: the goods
 * were handed over as they were sold, so no sweep can ever return them.
 */
export type FulfillmentStatus =
  | "picking"
  | "ready_for_pickup"
  | "paid"
  | "dispatched"
  | "expired"
  | "cancelled"
  | "sold_at_counter";

/** One delivery of an order. This is where the state machine lives. */
export interface UniformFulfillment {
  id: string;
  seq: number;
  /** The order's folio plus a suffix, e.g. U-2026-000123-A. */
  folio: string;
  status: FulfillmentStatus;
  amountCents: number;
  financeChargeId?: string;
  pickupDeadline?: string;
  /** The school calendar was unreachable; the deadline may still be extended. */
  deadlineEstimated: boolean;
  /** No rule could settle this one — it needs a person. */
  requiresAttention: boolean;
  attentionReason?: string;
}

/**
 * The order's state, DERIVED server-side on every read. `closed_partial` is the
 * honest ending for an order that delivered only some of what was asked for —
 * without it, the module's worst outcome would read exactly like its best.
 */
export type OrderLabel = "awaiting_stock" | "partial" | "in_progress" | "closed" | "closed_partial";

/** What a family asked for, with everything hanging off it. */
export interface UniformOrder {
  id: string;
  folio: string;
  studentId?: string;
  studentName: string;
  /** The pupil's level name at order time ("Preescolar"). */
  studentLevel?: string;
  /** The pupil's config level id at order time. */
  studentLevelId?: Level;
  studentGradeGroup?: string;
  /** family | counter | staff. */
  source: string;
  label: OrderLabel;
  totalCents: number;
  lines: UniformOrderLine[];
  fulfillments: UniformFulfillment[];
  createdAt?: string;
}

/** The school's limits. */
export interface UniformSettings {
  pickupWindowBusinessDays: number;
  autoCancelEnabled: boolean;
  maxExtensions: number;
  maxOpenOrdersPerStudent: number;
  maxLinesPerOrder: number;
  maxUnitsPerLine: number;
  maxLineExpiries: number;
}

/** One page of the panel's order list. */
export interface UniformOrdersPage {
  orders: UniformOrder[];
  nextPageToken?: string;
  totalCount: number;
}

/** One row of a size run being created or updated. */
export interface SizeSpec {
  sizeLabel: string;
  sizeOrder: number;
  minStock: number;
}

/**
 * Created and updated are reported separately so the panel can say "8 sizes
 * created, 2 already existed" instead of upserting in silence.
 */
export interface UpsertSizesResult {
  createdCount: number;
  updatedCount: number;
  sizes: UniformSize[];
}

/** One line of a goods receipt. */
export interface ReceiveLine {
  stockId: string;
  qty: number;
}

/** The shelf after a receipt, plus any waiting lines it satisfied on the way in. */
export interface ReceiveStockResult {
  stock: UniformSize[];
  allocatedLineIds: string[];
}

/**
 * One garment being sold. Note what is NOT here: a price. The amount always
 * comes from the shop's catalogue, and the absence of the field is the
 * enforcement.
 */
export interface SaleLine {
  itemId: string;
  sizeLabel: string;
  qty: number;
}

/** The panel's query. */
export interface OrdersQuery {
  levelId?: Level;
  /** A folio if it looks like one, otherwise a pupil's name. */
  q?: string;
  statuses?: FulfillmentStatus[];
  requiresAttention?: boolean;
  from?: string;
  to?: string;
  sort?: "createdAt" | "deadline" | "status";
  desc?: boolean;
  pageSize?: number;
  pageToken?: string;
}

// --- the error envelope ----------------------------------------------------

/**
 * A failure the panel can branch on.
 *
 * `code` is stable and machine-readable; `message` is Spanish and safe to show.
 * The UI branches on the code and NEVER on the message — `insufficient_stock`
 * and `conflict` are both 409, and only one of them is the cashier's fault.
 */
export class UniformsError extends Error {
  readonly code: string;
  readonly detail?: string;

  constructor(code: string, message: string, detail?: string) {
    super(message);
    this.name = "UniformsError";
    this.code = code;
    this.detail = detail;
  }

  /** True when a size ran out between showing it and taking it. */
  get isOutOfStock(): boolean {
    return this.code === "insufficient_stock";
  }

  /** True when the account lacks a permission this action needs. */
  get isForbidden(): boolean {
    return this.code === "forbidden";
  }
}

function url(baseURL: string, path: string): string {
  return `${baseURL.replace(/\/+$/, "")}${BASE_PATH}${path}`;
}

/**
 * Parses the gateway's error envelope.
 *
 * The gateway's older handlers still answer text/plain, so this reads JSON first
 * and falls back to text. One module moving to the envelope must not fork error
 * handling for the whole edge.
 */
async function failure(res: Response): Promise<UniformsError> {
  const body = await res.text().catch(() => "");
  try {
    const parsed = JSON.parse(body) as { error?: { code?: string; message?: string; details?: Record<string, string> } };
    const err = parsed.error;
    if (err?.code !== undefined && err.message !== undefined) {
      return fromEnvelope(err.code, err.message, spanishServerReason(err.details?.reason));
    }
  } catch {
    // Not the envelope — an older handler, or a proxy that swallowed the body.
  }
  return new UniformsError("internal", spanishServerReason(body) ?? GENERIC_SERVER_FAILURE);
}

/** The gateway's sentence for every 409 that is not an empty shelf. */
const CONFLICT_HEADLINE = "Eso ya existe";
const CONFLICT_FALLBACK = "No se pudo hacer el cambio con los datos actuales. Actualiza la página e intenta de nuevo.";

/**
 * ISSUE-CC-113: the gateway titles EVERY `conflict` "Eso ya existe", but a 409
 * is also a correction that would strip a family's hold. The translated reason
 * is the better headline; without one, a conflict says nothing about duplicates.
 */
function fromEnvelope(code: string, message: string, reason: string | undefined): UniformsError {
  if (code !== "conflict" || message !== CONFLICT_HEADLINE) return new UniformsError(code, message, reason);
  return new UniformsError(code, reason ?? CONFLICT_FALLBACK);
}

async function get<T>(baseURL: string, path: string): Promise<T> {
  const res = await authFetch(url(baseURL, path), { credentials: "include" });
  if (!res.ok) {
    throw await failure(res);
  }
  return (await res.json()) as T;
}

async function send<T>(
  baseURL: string,
  path: string,
  method: "POST" | "PUT",
  body: unknown,
  headers: Record<string, string> = {},
): Promise<T> {
  const res = await authFetch(url(baseURL, path), {
    method,
    credentials: "include",
    headers: { "Content-Type": "application/json", ...headers },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw await failure(res);
  }
  return (await res.json()) as T;
}

// --- levels ----------------------------------------------------------------

/** A level as the school defined it in Configuración. */
export interface SchoolLevel {
  id: string;
  name: string;
}

/**
 * The school's levels, for the level chips. Served by the uniforms module
 * itself, so a storeroom role needs no access to Configuración (ADR 0014).
 */
export async function listUniformLevels(baseURL: string): Promise<SchoolLevel[]> {
  const body = await get<{ levels: SchoolLevel[] }>(baseURL, "/levels");
  return body.levels ?? [];
}

// --- catalogue -------------------------------------------------------------

/**
 * Lists the catalogue. `levelId` narrows to what a pupil of that level wears:
 * the garments scoped to it PLUS the ones scoped to none.
 */
export async function listItems(
  baseURL: string,
  opts: { levelId?: Level; includeInactive?: boolean } = {},
): Promise<UniformItem[]> {
  const params = new URLSearchParams();
  if (opts.levelId !== undefined && opts.levelId !== "") params.set("levelId", opts.levelId);
  if (opts.includeInactive === true) params.set("includeInactive", "true");
  const query = params.toString();
  const body = await get<{ items: UniformItem[] }>(baseURL, query === "" ? "/items" : `/items?${query}`);
  return (body.items ?? []).map(normalizeItem);
}

/** No clave (task 3): an article is told apart by its id and found by name. */
export interface NewItem {
  name: string;
  description?: string;
  levelId?: Level;
  priceCents: number;
  /** Omitted means uniform. */
  category?: ItemCategory;
}

export async function createItem(baseURL: string, item: NewItem): Promise<UniformItem> {
  return normalizeItem(await send<UniformItem>(baseURL, "/items", "POST", item));
}

/**
 * Fills the fields the gateway may leave out, in the one place that can be sure.
 *
 * - `sizes`: a freshly created item has no size run yet, and a screen that does
 *   `item.sizes.map(...)` crashes on a missing field rather than showing nothing.
 * - `levelId`: a garment for every level has none, and the gateway omits it
 *   (omitempty). Undefined is not "", so it read as "nivel sin nombre".
 */
function normalizeItem(item: UniformItem): UniformItem {
  return { ...item, sizes: item.sizes ?? [], levelId: item.levelId ?? "" };
}

export interface ItemEdit {
  name: string;
  description?: string;
  levelId?: Level;
  priceCents: number;
  active: boolean;
  category?: ItemCategory;
}

export async function updateItem(baseURL: string, id: string, edit: ItemEdit): Promise<UniformItem> {
  return normalizeItem(await send<UniformItem>(baseURL, `/items/${encodeURIComponent(id)}`, "PUT", edit));
}

/** Writes a garment's whole size run in one call. */
export async function upsertSizes(baseURL: string, itemId: string, sizes: SizeSpec[]): Promise<UpsertSizesResult> {
  return send<UpsertSizesResult>(baseURL, `/items/${encodeURIComponent(itemId)}/sizes`, "POST", { sizes });
}

// --- inventory -------------------------------------------------------------

export async function listStock(
  baseURL: string,
  opts: { itemId?: string; onlyBelowMinimum?: boolean } = {},
): Promise<UniformSize[]> {
  const params = new URLSearchParams();
  if (opts.itemId !== undefined && opts.itemId !== "") params.set("itemId", opts.itemId);
  if (opts.onlyBelowMinimum === true) params.set("onlyBelowMinimum", "true");
  const query = params.toString();
  const body = await get<{ stock: UniformSize[] }>(baseURL, query === "" ? "/stock" : `/stock?${query}`);
  return body.stock ?? [];
}

export async function receiveStock(baseURL: string, lines: ReceiveLine[], note: string): Promise<ReceiveStockResult> {
  return send<ReceiveStockResult>(baseURL, "/stock/receive", "POST", { lines, note });
}

/** Corrects a count. The note is required by the gateway, not merely by the UI. */
export async function adjustStock(
  baseURL: string,
  stockId: string,
  qtyDelta: number,
  note: string,
): Promise<UniformSize> {
  return send<UniformSize>(baseURL, "/stock/adjust", "POST", { stockId, qtyDelta, note });
}

// --- the counter sale ------------------------------------------------------

export interface CounterSale {
  studentId?: string;
  lines: SaleLine[];
  method: PaymentMethod;
  reference?: string;
  /**
   * How it was paid, method by method (T8). The server prices the sale and
   * refuses tenders that do not add up to its own total.
   */
  tenders?: Tender[];
  note?: string;
}

/**
 * Sells and hands over in one act.
 *
 * `idempotencyKey` is REQUIRED and must be minted when the till drawer OPENS,
 * not when the button is pressed: a retry of the same sale carries the same key,
 * while the next parent in the queue carries a new one. This is the only call
 * that moves physical goods and money together, so a double click without it
 * would be a second sale of goods that left once.
 */
export async function counterSale(
  baseURL: string,
  sale: CounterSale,
  idempotencyKey: string,
): Promise<UniformOrder> {
  return send<UniformOrder>(baseURL, "/counter-sale", "POST", sale, { "Idempotency-Key": idempotencyKey });
}

// --- orders ----------------------------------------------------------------

export async function listOrders(baseURL: string, query: OrdersQuery = {}): Promise<UniformOrdersPage> {
  const params = new URLSearchParams();
  if (query.levelId !== undefined && query.levelId !== "") params.set("levelId", query.levelId);
  if (query.q !== undefined && query.q !== "") params.set("q", query.q);
  for (const status of query.statuses ?? []) params.append("status", status);
  if (query.requiresAttention === true) params.set("requiresAttention", "true");
  if (query.from !== undefined && query.from !== "") params.set("from", query.from);
  if (query.to !== undefined && query.to !== "") params.set("to", query.to);
  if (query.sort !== undefined) params.set("sort", query.sort);
  if (query.desc === false) params.set("desc", "false");
  if (query.pageSize !== undefined) params.set("pageSize", String(query.pageSize));
  if (query.pageToken !== undefined && query.pageToken !== "") params.set("pageToken", query.pageToken);
  const q = params.toString();
  return get<UniformOrdersPage>(baseURL, q === "" ? "/orders" : `/orders?${q}`);
}

export async function getOrder(baseURL: string, id: string): Promise<UniformOrder> {
  return get<UniformOrder>(baseURL, `/orders/${encodeURIComponent(id)}`);
}

// --- settings --------------------------------------------------------------

export async function getSettings(baseURL: string): Promise<UniformSettings> {
  return get<UniformSettings>(baseURL, "/settings");
}

export async function updateSettings(baseURL: string, settings: UniformSettings): Promise<UniformSettings> {
  return send<UniformSettings>(baseURL, "/settings", "PUT", settings);
}
