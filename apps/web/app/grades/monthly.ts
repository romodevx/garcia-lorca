/**
 * The monthly grade sheet's shared vocabulary: the gateway's JSON shapes and
 * the pure rules both capture surfaces (teacher and director) apply to them —
 * reading what a teacher typed, checking it against the level's scale, the
 * sheet's counters and the month chips. No I/O here; the two HTTP clients
 * (`grades/api.ts`, `teaching/api.ts`) import from this file.
 */

import { averageHalfUp } from "./average";

// --- gateway shapes -------------------------------------------------------

/** How a level grades: the range, the decimals allowed and the passing mark. */
export interface GradeScale {
  min: number;
  max: number;
  decimals: number;
  passing: number;
}

export interface MonthlyGradeRow {
  enrollmentId: string;
  studentId: string;
  name: string;
  value: number | null;
  updatedAt: string | null;
  /** False for a month that belongs to the pupil's previous group (a transfer). */
  rowEditable: boolean;
  /** Why the row cannot change, in the server's words. */
  rowReadOnlyReason?: string;
}

export interface GradeSheet {
  assignmentId: string;
  /** "YYYY-MM". */
  month: string;
  /** Every month of the cycle the sheet can show, oldest first. */
  months: string[];
  scale: GradeScale;
  /** False for a level without numeric grades (Kínder): its scale is meaningless. */
  gradingEnabled?: boolean;
  editable: boolean;
  readOnlyReason?: string;
  rows: MonthlyGradeRow[];
}

export interface GradeSheetQuery {
  assignmentId: string;
  /** Omit to let the server pick the current month. */
  month?: string;
}

export interface MonthlyGradeKey {
  assignmentId: string;
  month: string;
  enrollmentId: string;
}

export interface SaveMonthlyGradeInput extends MonthlyGradeKey {
  value: number;
}

export interface SavedMonthlyGrade {
  enrollmentId: string;
  month: string;
  value: number;
  updatedAt: string;
}

/** What a capture surface needs from its client — teacher or director. */
export interface GradeSheetClient {
  load: (q: GradeSheetQuery) => Promise<GradeSheet>;
  save: (input: SaveMonthlyGradeInput) => Promise<SavedMonthlyGrade>;
  remove: (key: MonthlyGradeKey) => Promise<void>;
}

// --- errors ---------------------------------------------------------------

/** A rejected grade request, carrying the server's own (text/plain) reason. */
export class GradeRequestError extends Error {
  readonly status: number;
  readonly detail: string;
  /** The refusal's machine code (`X-Error-Code`), when the server sent one. */
  readonly code: string | undefined;

  constructor(status: number, detail: string, code?: string) {
    super(detail !== "" ? detail : `HTTP ${status}`);
    this.name = "GradeRequestError";
    this.status = status;
    this.detail = detail;
    this.code = code;
  }
}

export const ERROR_CODE_HEADER = "X-Error-Code";

export async function gradeFailure(res: Response): Promise<GradeRequestError> {
  const body = await res.text().catch(() => "");
  return new GradeRequestError(res.status, body.trim(), res.headers.get(ERROR_CODE_HEADER) ?? undefined);
}

/**
 * Where a refused save belongs: on its row (the value is wrong — also any
 * code this page does not know yet, shown in the server's words), a row
 * that can no longer change (the month belongs to the pupil's previous
 * group), or the whole sheet (the level cannot take grades right now).
 */
export type RefusalScope = "row" | "rowLocked" | "sheet";

const MONTH_PREVIOUS_GROUP = "MONTH_PREVIOUS_GROUP";
const LEVEL_CODES: ReadonlySet<string> = new Set(["LEVEL_NOT_GRADED", "LEVEL_SCALE_NOT_CONFIGURED"]);

export function refusalScope(code: string | undefined): RefusalScope {
  if (code === MONTH_PREVIOUS_GROUP) return "rowLocked";
  return code !== undefined && LEVEL_CODES.has(code) ? "sheet" : "row";
}

const BAD_REQUEST = 400;

/** The server said the value itself is wrong: retrying the same one won't help. */
export function isRejectedValue(err: unknown): err is GradeRequestError {
  return err instanceof GradeRequestError && err.status === BAD_REQUEST && err.detail !== "";
}

/** `?assignmentId=…[&month=…]`. */
export function sheetSearch(q: GradeSheetQuery): string {
  const search = new URLSearchParams({ assignmentId: q.assignmentId });
  if (q.month !== undefined && q.month !== "") search.set("month", q.month);
  return `?${search.toString()}`;
}

/** `?assignmentId=…&month=…&enrollmentId=…`. */
export function gradeKeySearch(k: MonthlyGradeKey): string {
  return `?${new URLSearchParams({ assignmentId: k.assignmentId, month: k.month, enrollmentId: k.enrollmentId }).toString()}`;
}

// --- reading and checking what was typed ----------------------------------

export type ParsedGrade =
  | { kind: "empty" }
  | { kind: "invalid" }
  | { kind: "value"; value: number; decimals: number };

/** Digits, optionally a point and more digits — no signs, exponents, hex or inner spaces. */
const NUMBER_SHAPE = /^(\d+)(?:\.(\d+))?$/;

/** "9,5" and "9.5" are the same grade; an empty field clears it; "9.50" has one decimal. */
export function parseGradeInput(raw: string): ParsedGrade {
  const text = raw.trim().replace(",", ".");
  if (text === "") return { kind: "empty" };
  const match = NUMBER_SHAPE.exec(text);
  if (match === null) return { kind: "invalid" };
  const significant = (match[2] ?? "").replace(/0+$/, "");
  return { kind: "value", value: Number(text), decimals: significant.length };
}

function decimalsMessage(decimals: number): string {
  if (decimals === 0) return "Solo se permiten calificaciones enteras";
  return decimals === 1 ? "Solo se permite 1 decimal" : `Solo se permiten ${decimals} decimales`;
}

/** The same words the server uses, so a teacher reads one message either way. */
export function validateGrade(raw: string, scale: GradeScale): string | undefined {
  const parsed = parseGradeInput(raw);
  if (parsed.kind === "empty") return undefined;
  if (parsed.kind === "invalid") return "La calificación debe ser un número";
  if (parsed.value < scale.min || parsed.value > scale.max) {
    return `La calificación debe estar entre ${scale.min} y ${scale.max}`;
  }
  return parsed.decimals > scale.decimals ? decimalsMessage(scale.decimals) : undefined;
}

// --- counters -------------------------------------------------------------

export interface SheetStats {
  captured: number;
  total: number;
  average: number | undefined;
  failing: number;
}

export function sheetStats(values: readonly (number | null)[], passing: number): SheetStats {
  const captured = values.filter((v): v is number => v !== null);
  return {
    captured: captured.length,
    total: values.length,
    average: averageHalfUp(captured),
    failing: captured.filter((v) => v < passing).length,
  };
}

// --- month chips ----------------------------------------------------------

const SHORT = ["Ene", "Feb", "Mar", "Abr", "May", "Jun", "Jul", "Ago", "Sep", "Oct", "Nov", "Dic"] as const;
const FULL = ["enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"] as const;

export interface MonthChip {
  month: string;
  label: string;
  fullName: string;
}

function splitMonth(month: string): { year: string; index: number } {
  const [year = "", mm = "1"] = month.split("-");
  return { year, index: Number(mm) - 1 };
}

/** "Sep", "Oct"… — with the year only when a short name would repeat. */
export function monthChips(months: readonly string[]): MonthChip[] {
  const shortOf = (m: string): string => SHORT[splitMonth(m).index] ?? m;
  const seen = new Map<string, number>();
  for (const m of months) seen.set(shortOf(m), (seen.get(shortOf(m)) ?? 0) + 1);
  return months.map((month) => {
    const { year, index } = splitMonth(month);
    const short = shortOf(month);
    return {
      month,
      label: (seen.get(short) ?? 0) > 1 ? `${short} ${year}` : short,
      fullName: `${FULL[index] ?? month} de ${year}`,
    };
  });
}
