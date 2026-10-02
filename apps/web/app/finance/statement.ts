import { downloadBlob } from "../_lib/downloadBlob";
import { authFetch } from "../auth/authFetch";

/**
 * A student's account statement (T7): the ledger finance computes ONCE and the
 * office's drawer, the parents' app and the PDF all read. Nothing here derives
 * money — the balance, the running balance and today's figure come from the
 * server as they are.
 */

/** What a line records. scholarship (T18): what a scholarship takes off a charge, a credit. */
export type MovementKind = "charge" | "surcharge" | "discount" | "scholarship" | "payment" | "credit_note";

export interface StatementMovement {
  kind: MovementKind;
  /** YYYY-MM-DD. */
  date: string;
  description: string;
  /** Always positive; `kind` says the direction. */
  amountCents: number;
  /** The running balance after this line. Negative is the family's credit. */
  balanceCents: number;
  chargeId?: string;
  depositId?: string;
  method?: string;
  cardType?: string;
  reference?: string;
  folio?: string;
  /** A surcharge earned up to today and not yet written down. Counted. */
  accrued: boolean;
  /** A charge that has not opened (T5). Shown, not counted. */
  notYetDue: boolean;
  /** A cancelled deposit. Shown struck through, not counted. */
  voided: boolean;
  voidReason?: string;
  /** T9: a receipt replaced by its correction. Shown struck through, not counted. */
  superseded?: boolean;
  supersededByFolio?: string;
  /** T9: on a correction's lines, the receipt it replaces. */
  correctsFolio?: string;
}

export interface StatementTotals {
  chargedCents: number;
  discountCents: number;
  paidCents: number;
  balanceCents: number;
  notYetDueCents: number;
}

export interface Statement {
  movements: StatementMovement[];
  totals: StatementTotals;
  /** What Registrar pago asks for today. */
  dueTodayCents: number;
  surchargeCents: number;
  earlyDiscountCents: number;
  unallocatedCents: number;
  asOf: string;
}

/** A statement the gateway would not give, with its status. */
export class StatementError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "StatementError";
    this.status = status;
  }
}

/** The kinds that raise the balance. */
export function isDebit(kind: MovementKind): boolean {
  return kind === "charge" || kind === "surcharge";
}

// Ids are uuids; anything else never reaches the network (it would be a path).
const ID_SHAPE = /^[0-9a-zA-Z-]{1,64}$/;

function statementURL(baseURL: string, studentId: string, suffix = ""): string {
  if (!ID_SHAPE.test(studentId)) {
    throw new StatementError(400, "Ese alumno no existe.");
  }
  return `${baseURL.replace(/\/+$/, "")}/api/v1/finance/accounts/${encodeURIComponent(studentId)}/statement${suffix}`;
}

const EMPTY_TOTALS: StatementTotals = { chargedCents: 0, discountCents: 0, paidCents: 0, balanceCents: 0, notYetDueCents: 0 };

/** GET the office's view of a student's ledger. */
export async function getStatement(baseURL: string, studentId: string): Promise<Statement> {
  const res = await authFetch(statementURL(baseURL, studentId), { credentials: "include" });
  if (!res.ok) {
    throw new StatementError(res.status, "No se pudo leer el estado de cuenta.");
  }
  const raw = (await res.json()) as Partial<Statement>;
  return {
    movements: raw.movements ?? [],
    totals: { ...EMPTY_TOTALS, ...raw.totals },
    dueTodayCents: raw.dueTodayCents ?? 0,
    surchargeCents: raw.surchargeCents ?? 0,
    earlyDiscountCents: raw.earlyDiscountCents ?? 0,
    unallocatedCents: raw.unallocatedCents ?? 0,
    asOf: raw.asOf ?? "",
  };
}

/** GET the statement's PDF, with the file name the gateway gave it. */
export async function statementPdf(baseURL: string, studentId: string): Promise<{ blob: Blob; filename: string }> {
  const res = await authFetch(statementURL(baseURL, studentId, "/pdf"), { credentials: "include" });
  if (!res.ok) {
    throw new StatementError(res.status, "No se pudo descargar el PDF.");
  }
  const named = /filename="([^"]+)"/.exec(res.headers.get("Content-Disposition") ?? "");
  return { blob: await res.blob(), filename: named?.[1] ?? "estado-de-cuenta.pdf" };
}

/** Saves the statement's PDF through the browser's own download. */
export async function downloadStatementPdf(baseURL: string, studentId: string): Promise<void> {
  const { blob, filename } = await statementPdf(baseURL, studentId);
  downloadBlob(blob, filename);
}
