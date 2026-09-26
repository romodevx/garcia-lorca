/**
 * The unsaved scans of one list, kept in sessionStorage so a reload in the
 * middle of a queue does not lose them. Scans are NOT persisted to the server
 * until "Guardar lista"; this is only the tab's memory.
 *
 * Storage can throw (private mode, blocked site data) or come back empty: every
 * access is wrapped and the list works without it.
 */

import type { SheetTarget } from "./useAttendanceSheet";

const PREFIX = "attendance-scan";

interface StoredDraft {
  scanned: unknown;
}

export function scanDraftKey({ groupId, date, subjectId }: SheetTarget): string {
  return `${PREFIX}:${groupId}:${date}:${subjectId ?? ""}`;
}

/** The enrollments scanned and not yet saved; [] when there is none or it cannot be read. */
export function readScanDraft(target: SheetTarget): string[] {
  try {
    const raw = window.sessionStorage.getItem(scanDraftKey(target));
    if (raw === null) return [];
    const parsed = JSON.parse(raw) as StoredDraft | null;
    const scanned = parsed?.scanned;
    return Array.isArray(scanned) ? scanned.filter((id): id is string => typeof id === "string") : [];
  } catch {
    return [];
  }
}

export function clearScanDraft(target: SheetTarget): void {
  try {
    window.sessionStorage.removeItem(scanDraftKey(target));
  } catch {
    // Storage blocked: there is nothing to clear.
  }
}

/** Stores the scanned enrollments; an empty set removes the draft. */
export function writeScanDraft(target: SheetTarget, scanned: readonly string[]): void {
  if (scanned.length === 0) {
    clearScanDraft(target);
    return;
  }
  try {
    window.sessionStorage.setItem(scanDraftKey(target), JSON.stringify({ scanned }));
  } catch {
    // Storage blocked or full: the scans still live in memory until saved.
  }
}
