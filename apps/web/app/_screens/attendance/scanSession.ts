/**
 * The pure rules of a scan session on one list. No React, no storage.
 *
 * "Sin registrar" (and what "Terminar" turns absent) applies ONLY to a row
 * that is present by default on a list nobody has saved yet, that no card was
 * read for and that the teacher has not changed by hand. On a list that
 * already exists a scan only turns rows present — a latecomer saved as absent —
 * and "Terminar" marks nobody absent. A late, excused or absent row, loaded or
 * set by hand, is never overwritten.
 */

import type { AttendanceStatus } from "../../attendance/api";
import type { AttendanceSheet, AttendanceSheetRow } from "../../attendance/sheet";
import type { ScanDraft } from "./scanDraft";

export interface ScanSession {
  /** The list (scanDraftKey) this session belongs to. */
  key: string;
  /** Present-by-default rows of a never-saved list: the only ones "Sin registrar" applies to. */
  candidates: ReadonlySet<string>;
  scanned: ReadonlySet<string>;
  touched: ReadonlySet<string>;
}

const NONE: ReadonlySet<string> = new Set();

export function emptySession(key: string): ScanSession {
  return { key, candidates: NONE, scanned: NONE, touched: NONE };
}

export interface OpenedSession {
  session: ScanSession;
  /** The statuses to put back, verbatim, when a reload interrupted a session. */
  restore?: Record<string, AttendanceStatus>;
}

/** A freshly loaded list: its candidates, and the interrupted session when there is one to pick up. */
export function openSession(key: string, sheet: AttendanceSheet, draft: ScanDraft | undefined): OpenedSession {
  const candidates = sheet.session === null ? new Set(sheet.rows.filter((row) => row.status === "present").map((row) => row.enrollmentId)) : NONE;
  const fresh: ScanSession = { ...emptySession(key), candidates };
  const onList = new Set(sheet.rows.map((row) => row.enrollmentId));
  const scanned = (draft?.scanned ?? []).filter((id) => onList.has(id));
  if (draft === undefined || scanned.length === 0 || !sheet.editable) return { session: fresh };
  const restore = Object.fromEntries(Object.entries(draft.statuses).filter(([id]) => onList.has(id)));
  const touchedIds = draft.touched.filter((id) => onList.has(id));
  return { session: { ...fresh, scanned: new Set(scanned), touched: new Set(touchedIds) }, restore };
}

/** The rows still "Sin registrar": candidates nobody scanned or changed by hand. */
export function unregistered(session: ScanSession): Set<string> {
  return new Set([...session.candidates].filter((id) => !session.scanned.has(id) && !session.touched.has(id)));
}

export function withScan(session: ScanSession, enrollmentId: string): ScanSession {
  return { ...session, scanned: new Set(session.scanned).add(enrollmentId) };
}

/** The teacher changed these rows by hand: the scanner leaves them alone from now on. */
export function touched(session: ScanSession, enrollmentIds: readonly string[]): ScanSession {
  return { ...session, touched: new Set([...session.touched, ...enrollmentIds]) };
}

/** What sessionStorage keeps: the whole list as it is on screen; nothing until a card was read. */
export function draftOf(session: ScanSession, rows: readonly AttendanceSheetRow[]): ScanDraft | undefined {
  if (session.scanned.size === 0) return undefined;
  return {
    statuses: Object.fromEntries(rows.map((row) => [row.enrollmentId, row.status])),
    scanned: [...session.scanned],
    touched: [...session.touched],
  };
}
