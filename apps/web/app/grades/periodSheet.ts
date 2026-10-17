/**
 * The period grade sheet's shared vocabulary (T15): the gateway's JSON shapes
 * and the pure rules both capture surfaces (teacher and director) apply to
 * them — reading what a teacher typed, checking it (whole numbers only, inside
 * the level's range), the sheet's counters and the period chips. No I/O here;
 * the two HTTP clients (`grades/api.ts`, `teaching/api.ts`) import from this file.
 */

import { subjectAverage } from "@garcia-lorca/domain/grade-average";

import { shortDayLabel } from "../_lib/localDate";
import { GradeRequestError } from "./requestError";

// --- gateway shapes -------------------------------------------------------

/** How a level grades: the range and the passing mark (`decimals` no longer applies to capture). */
export interface GradeScale {
  min: number;
  max: number;
  decimals: number;
  passing: number;
}

/** An evaluation period of the group's grade (config, T14). Both dates inclusive, "YYYY-MM-DD". */
export interface EvaluationPeriodRef {
  id: string;
  ordinal: number;
  name: string;
  startsOn: string;
  endsOn: string;
}

export interface PeriodGradeRow {
  enrollmentId: string;
  studentId: string;
  name: string;
  value: number | null;
  updatedAt: string | null;
  /** False for a period that belongs to the pupil's previous group (a transfer). */
  rowEditable: boolean;
  /** Why the row cannot change, in the server's words. */
  rowReadOnlyReason?: string;
}

export interface PeriodSheet {
  assignmentId: string;
  /** The period shown; null when the grade has no periods. */
  periodId: string | null;
  /** The grade's evaluation periods in the assignment's cycle, by ordinal. */
  periods: EvaluationPeriodRef[];
  /** The period containing `today`; null when today falls in none. */
  currentPeriodId: string | null;
  /** The school's current local day, "YYYY-MM-DD": a period starting later is not gradable yet. */
  today: string;
  /** Periods whose level term is closed: read-only for everyone. */
  closedPeriodIds: string[];
  scale: GradeScale;
  /** False for a level without numeric grades (Kínder): its scale is meaningless. */
  gradingEnabled: boolean;
  editable: boolean;
  readOnlyReason?: string;
  rows: PeriodGradeRow[];
}

export interface PeriodSheetQuery {
  assignmentId: string;
  /** Omit to let the server pick the current period. */
  periodId?: string;
}

export interface PeriodGradeKey {
  assignmentId: string;
  periodId: string;
  enrollmentId: string;
}

export interface SavePeriodGradeInput extends PeriodGradeKey {
  value: number;
}

export interface SavedPeriodGrade {
  enrollmentId: string;
  periodId: string;
  value: number;
  updatedAt: string | null;
}

/** What a capture surface needs from its client — teacher or director. */
export interface GradeSheetClient {
  load: (q: PeriodSheetQuery) => Promise<PeriodSheet>;
  save: (input: SavePeriodGradeInput) => Promise<SavedPeriodGrade>;
  remove: (key: PeriodGradeKey) => Promise<void>;
}

// --- refusals -------------------------------------------------------------

/**
 * Where a refused save belongs: on its row (the value is wrong — also any
 * code this page does not know yet, shown in the server's words), a row
 * that can no longer change (the period belongs to the pupil's previous
 * group), or the whole sheet (the level or the period cannot take grades now).
 */
export type RefusalScope = "row" | "rowLocked" | "sheet";

const PERIOD_PREVIOUS_GROUP = "PERIOD_PREVIOUS_GROUP";
/** The period's term is closed (409): only the director reopening it lets it change. */
export const PERIOD_LOCKED = "PERIOD_LOCKED";
const SHEET_CODES: ReadonlySet<string> = new Set(["LEVEL_NOT_GRADED", "LEVEL_SCALE_NOT_CONFIGURED", "PERIOD_NOT_STARTED", PERIOD_LOCKED]);

export function refusalScope(code: string | undefined): RefusalScope {
  if (code === PERIOD_PREVIOUS_GROUP) return "rowLocked";
  return code !== undefined && SHEET_CODES.has(code) ? "sheet" : "row";
}

const BAD_REQUEST = 400;

/**
 * The server refused for a reason resending cannot fix: the value itself is
 * wrong (400), or the period is closed (PERIOD_LOCKED, whatever status).
 */
export function isRejectedValue(err: unknown): err is GradeRequestError {
  if (!(err instanceof GradeRequestError) || err.detail === "") return false;
  return err.status === BAD_REQUEST || err.code === PERIOD_LOCKED;
}

/** `?assignmentId=…[&periodId=…]`. */
export function sheetSearch(q: PeriodSheetQuery): string {
  const search = new URLSearchParams({ assignmentId: q.assignmentId });
  if (q.periodId !== undefined && q.periodId !== "") search.set("periodId", q.periodId);
  return `?${search.toString()}`;
}

/** `?assignmentId=…&periodId=…&enrollmentId=…`. */
export function gradeKeySearch(k: PeriodGradeKey): string {
  return `?${new URLSearchParams({ assignmentId: k.assignmentId, periodId: k.periodId, enrollmentId: k.enrollmentId }).toString()}`;
}

// --- reading and checking what was typed ----------------------------------

export type ParsedGrade =
  | { kind: "empty" }
  | { kind: "invalid" }
  | { kind: "value"; value: number; decimals: number };

/** An optional minus, digits, optionally a point and more digits — no plus, exponents, hex or inner spaces. A minus is read so the range check names the scale (CC-307). */
const NUMBER_SHAPE = /^-?(\d+)(?:\.(\d+))?$/;

/**
 * "9,5" and "9.5" read alike (so the decimals can be refused, never rounded);
 * an empty field clears the grade; "9.0" has no decimals.
 */
export function parseGradeInput(raw: string): ParsedGrade {
  const text = raw.trim().replace(",", ".");
  if (text === "") return { kind: "empty" };
  const match = NUMBER_SHAPE.exec(text);
  if (match === null) return { kind: "invalid" };
  const significant = (match[2] ?? "").replace(/0+$/, "");
  return { kind: "value", value: Number(text), decimals: significant.length };
}

export const NOT_INTEGER_MESSAGE = "Solo se permiten calificaciones enteras";

/** The same words the server uses, so a teacher reads one message either way. */
export function validateGrade(raw: string, scale: GradeScale): string | undefined {
  const parsed = parseGradeInput(raw);
  if (parsed.kind === "empty") return undefined;
  if (parsed.kind === "invalid") return "La calificación debe ser un número";
  if (parsed.decimals > 0) return NOT_INTEGER_MESSAGE;
  if (parsed.value < scale.min || parsed.value > scale.max) {
    return `La calificación debe estar entre ${scale.min} y ${scale.max}`;
  }
  return undefined;
}

// --- counters -------------------------------------------------------------

export interface SheetStats {
  captured: number;
  total: number;
  /** The exact mean truncated to 2 decimals (the shared rule); undefined when nothing is captured. */
  average: number | undefined;
  failing: number;
}

export function sheetStats(values: readonly (number | null)[], passing: number): SheetStats {
  const captured = values.filter((v): v is number => v !== null);
  return {
    captured: captured.length,
    total: values.length,
    average: subjectAverage(captured),
    failing: captured.filter((v) => v < passing).length,
  };
}

// --- period chips ---------------------------------------------------------

export interface PeriodChip {
  id: string;
  label: string;
  /** "24 ago – 30 nov". */
  range: string;
  /** Starts after the school's today: not gradable yet. */
  notStarted: boolean;
  /** Its level term is closed: read-only until the director reopens it. */
  closed: boolean;
  /** Contains today. */
  current: boolean;
}

export function periodChips(sheet: Pick<PeriodSheet, "periods" | "today" | "closedPeriodIds" | "currentPeriodId">): PeriodChip[] {
  const closed = new Set(sheet.closedPeriodIds);
  return sheet.periods.map((p) => ({
    id: p.id,
    label: p.name,
    range: `${shortDayLabel(p.startsOn)} – ${shortDayLabel(p.endsOn)}`,
    // "YYYY-MM-DD" strings order like the days they name.
    notStarted: p.startsOn > sheet.today,
    closed: closed.has(p.id),
    current: p.id === sheet.currentPeriodId,
  }));
}
