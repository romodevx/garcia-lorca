/**
 * The unsaved scan session of one list, kept in sessionStorage so a reload in
 * the middle of a queue does not lose it. Nothing reaches the server until
 * "Guardar lista"; this is only the tab's memory.
 *
 * The draft is the whole list as the teacher left it — every row's status,
 * which rows were scanned and which were changed by hand — so a reload after
 * "Terminar" and a few corrections comes back exactly as it was.
 *
 * Storage can throw (private mode, blocked site data) or come back empty: every
 * access is wrapped and the list works without it.
 */

import type { AttendanceStatus } from "../../attendance/api";
import type { SheetTarget } from "./useAttendanceSheet";

const PREFIX = "attendance-scan";
const STATUSES: readonly AttendanceStatus[] = ["present", "absent", "late", "excused"];

export interface ScanDraft {
  /** enrollmentId → the status on screen. */
  statuses: Record<string, AttendanceStatus>;
  /** The enrollments a card was read for. */
  scanned: string[];
  /** The enrollments the teacher changed by hand: never "Sin registrar" again. */
  touched: string[];
}

export function scanDraftKey({ groupId, date, subjectId }: SheetTarget): string {
  return `${PREFIX}:${groupId}:${date}:${subjectId ?? ""}`;
}

const isStatus = (value: unknown): value is AttendanceStatus => STATUSES.includes(value as AttendanceStatus);
const idsOf = (value: unknown): string[] => (Array.isArray(value) ? value.filter((id): id is string => typeof id === "string") : []);

function statusesOf(value: unknown): Record<string, AttendanceStatus> {
  if (typeof value !== "object" || value === null) return {};
  return Object.fromEntries(Object.entries(value).filter((entry): entry is [string, AttendanceStatus] => isStatus(entry[1])));
}

/** The stored session, or undefined when there is none or it cannot be read. */
export function readScanDraft(target: SheetTarget): ScanDraft | undefined {
  try {
    const raw = window.sessionStorage.getItem(scanDraftKey(target));
    if (raw === null) return undefined;
    const parsed = JSON.parse(raw) as Record<string, unknown> | null;
    if (typeof parsed?.statuses !== "object" || parsed.statuses === null) return undefined;
    return { statuses: statusesOf(parsed.statuses), scanned: idsOf(parsed.scanned), touched: idsOf(parsed.touched) };
  } catch {
    return undefined;
  }
}

export function clearScanDraft(target: SheetTarget): void {
  try {
    window.sessionStorage.removeItem(scanDraftKey(target));
  } catch {
    // Storage blocked: there is nothing to clear.
  }
}

/** Stores the session; without one (or with nothing scanned) the draft is removed. */
export function writeScanDraft(target: SheetTarget, draft: ScanDraft | undefined): void {
  if (draft === undefined || draft.scanned.length === 0) {
    clearScanDraft(target);
    return;
  }
  try {
    window.sessionStorage.setItem(scanDraftKey(target), JSON.stringify(draft));
  } catch {
    // Storage blocked or full: the session still lives in memory until saved.
  }
}
