/**
 * The attendance SHEET vocabulary shared by the director client
 * (`/api/v1/attendance*`) and the teacher's self-scoped one
 * (`/api/v1/teaching/attendance*`): both return the same sheet and take the same
 * body, so the shape, the body builder and the error parsing live here once.
 * Pure — no fetch, no React.
 */

import type { AttendanceException, AttendanceStatus } from "./api";

/** How a level takes attendance: one list per group·day, or one per group·subject·day. */
export type AttendanceMode = "daily" | "per_class";

/** Who took the list and when; null while it has never been saved. */
export interface AttendanceSession {
  /** ISO timestamp. */
  takenAt: string;
  takenByName: string;
}

export interface AttendanceSheetRow {
  enrollmentId: string;
  studentId: string;
  name: string;
  /** Reads back as "present" when the day has no stored mark for the row. */
  status: AttendanceStatus;
  note: string;
}

/** Why a list is read-only: a future day, or a teacher past the correction window. */
export type ReadOnlyReason = "future" | "window";

export interface AttendanceSheet {
  mode: AttendanceMode;
  date: string;
  /** False when the caller may only look (a teacher past the correction window). */
  editable: boolean;
  /** How many days back a teacher may still correct. */
  windowDays: number;
  session: AttendanceSession | null;
  rows: AttendanceSheetRow[];
  /** Set by newer gateways when `editable` is false. */
  readOnlyReason?: ReadOnlyReason;
  /** The server's school day (`YYYY-MM-DD`, UTC); absent from older gateways. */
  today?: string;
}

export interface AttendanceSheetQuery {
  groupId: string;
  /** `YYYY-MM-DD`. */
  date: string;
  /** Only for a per-class list. */
  subjectId?: string;
}

/** One non-present exception of the list. */
export interface SheetMark {
  enrollmentId: string;
  status: AttendanceException;
  note?: string;
}

/** The COMPLETE exception set of a list: every enrollment left out is stored as present. */
export interface SaveSheetInput extends AttendanceSheetQuery {
  marks: readonly SheetMark[];
}

/**
 * A rejected attendance request. `message` is the server's own text/plain
 * reason ("Solo puedes corregir hasta 7 días atrás"), which is written for the
 * person and is shown as is.
 */
export class AttendanceRequestError extends Error {
  readonly status: number;

  constructor(status: number, detail: string) {
    super(detail !== "" ? detail : `HTTP ${status}`);
    this.name = "AttendanceRequestError";
    this.status = status;
  }
}

/** Reads a failed response's text body into an AttendanceRequestError. */
export async function attendanceFailure(res: Response): Promise<AttendanceRequestError> {
  const body = await res.text().catch(() => "");
  return new AttendanceRequestError(res.status, body.trim());
}

/** `?groupId=…&date=…[&subjectId=…]`. */
export function sheetSearch(q: AttendanceSheetQuery): string {
  const search = new URLSearchParams({ groupId: q.groupId, date: q.date });
  if (q.subjectId !== undefined && q.subjectId !== "") search.set("subjectId", q.subjectId);
  return `?${search.toString()}`;
}

/** The POST body: subjectId only when there is one, a note only when something was typed. */
export function sheetBody(input: SaveSheetInput): Record<string, unknown> {
  const marks = input.marks.map((mark) => {
    const note = mark.note?.trim() ?? "";
    return { enrollmentId: mark.enrollmentId, status: mark.status, ...(note !== "" ? { note } : {}) };
  });
  const subject = input.subjectId !== undefined && input.subjectId !== "" ? { subjectId: input.subjectId } : {};
  return { groupId: input.groupId, date: input.date, ...subject, marks };
}
