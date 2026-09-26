/**
 * The pure rules of a pass list — what a tap does, what is counted, what is
 * sent, and which days may be opened. No React, no fetch, no clock (the caller
 * passes "today"), so every rule is tested in isolation.
 */

import type { AttendanceException, AttendanceStatus } from "../../attendance/api";
import type { AttendanceSession, AttendanceSheetRow, SheetMark } from "../../attendance/sheet";

const MS_PER_DAY = 86_400_000;

/** The big tap: present ↔ absent. A late or excused student goes back to present. */
export function toggled(status: AttendanceStatus): AttendanceStatus {
  return status === "present" ? "absent" : "present";
}

export type StatusCounts = Record<AttendanceStatus, number>;

export function tally(rows: readonly AttendanceSheetRow[]): StatusCounts {
  const counts: StatusCounts = { present: 0, absent: 0, late: 0, excused: 0 };
  for (const row of rows) counts[row.status] += 1;
  return counts;
}

const isException = (status: AttendanceStatus): status is AttendanceException => status !== "present";

/** The non-present rows, in list order, each with its note — exactly what the save carries. */
export function exceptionsOf(rows: readonly AttendanceSheetRow[]): SheetMark[] {
  return rows.flatMap((row) => {
    if (!isException(row.status)) return [];
    const note = row.note.trim();
    return [{ enrollmentId: row.enrollmentId, status: row.status, ...(note !== "" ? { note } : {}) }];
  });
}

/** True when any row's status or note differs from what was loaded. */
export function isDirty(rows: readonly AttendanceSheetRow[], baseline: readonly AttendanceSheetRow[]): boolean {
  const loaded = new Map(baseline.map((row) => [row.enrollmentId, row]));
  return rows.some((row) => {
    const before = loaded.get(row.enrollmentId);
    return before === undefined || before.status !== row.status || before.note !== row.note;
  });
}

/** `YYYY-MM-DD` plus `days` (negative goes back). Calendar arithmetic in UTC: no DST drift. */
export function addDays(isoDate: string, days: number): string {
  const [year, month, day] = isoDate.split("-").map(Number) as [number, number, number];
  return new Date(Date.UTC(year, month - 1, day) + days * MS_PER_DAY).toISOString().slice(0, 10);
}

/** The local calendar day as `YYYY-MM-DD`. */
export function todayISO(now: Date = new Date()): string {
  const month = `${now.getMonth() + 1}`.padStart(2, "0");
  const day = `${now.getDate()}`.padStart(2, "0");
  return `${now.getFullYear()}-${month}-${day}`;
}

/** The days a list may be opened on: a teacher today…today−window, the director any. */
export function dateWindow(today: string, windowDays: number, restricted: boolean): { min?: string; max?: string } {
  return restricted ? { min: addDays(today, -windowDays), max: today } : {};
}

/** "Lista guardada a las 8:05 por Mtra. X", or that it has not been taken. */
export function sessionLine(session: AttendanceSession | null): string {
  if (session === null) return "Lista aún no tomada";
  const at = new Date(session.takenAt);
  const time = `${at.getHours()}:${`${at.getMinutes()}`.padStart(2, "0")}`;
  return `Lista guardada a las ${time} por ${session.takenByName}`;
}
