import { useCallback, useEffect, useRef, useState } from "react";

import type { AttendanceStatus } from "../../attendance/api";
import type { CardScanResult, ScannedStudent } from "../../attendance/scan";
import type { AttendanceSheet, AttendanceSheetRow } from "../../attendance/sheet";
import type { ScanFlash, ScanTone } from "./QrScanDialog";
import { clearScanDraft, readScanDraft, scanDraftKey, writeScanDraft, type ScanDraft } from "./scanDraft";
import { createScanFeedback, type ScanFeedback } from "./scanFeedback";
import { draftOf, emptySession, openSession, touched, unregistered, withScan, type ScanSession } from "./scanSession";
import type { SheetTarget } from "./useAttendanceSheet";

export type ResolveCard = (code: string) => Promise<CardScanResult>;

export interface CardScanState {
  open: boolean;
  scanned: ReadonlySet<string>;
  /** The rows shown "Sin registrar" while the camera is open (and turned absent by "Terminar"). */
  unregistered: ReadonlySet<string>;
  flash?: ScanFlash;
  announcement: string;
  /** "Revisa la lista…" / "Recuperamos…": what the sheet says after scanning. */
  banner?: string;
  start: () => void;
  finish: () => void;
  onCode: (code: string) => void;
  /** The teacher changed these rows by hand: the scanner never overwrites them. */
  touch: (enrollmentIds: readonly string[]) => void;
  /** After a successful save of `list` (maybe no longer the one on screen): its scans are on the server now. */
  saved: (list: SheetTarget) => void;
}

interface Args {
  target: SheetTarget;
  sheet: AttendanceSheet | undefined;
  /** The list `sheet` belongs to: the scanner starts a list only once ITS sheet arrived. */
  loadedFor: SheetTarget | undefined;
  rows: readonly AttendanceSheetRow[];
  resolve: ResolveCard | undefined;
  setStatus: (enrollmentId: string, status: AttendanceStatus) => void;
}

/** A code is ignored until it has been out of view this long: a card held in front of the camera is read many times. */
export const SAME_CODE_DEBOUNCE_MS = 2500;
const FLASH_MS: Record<ScanTone, number> = { ok: 1200, duplicate: 1600, error: 2500 };
const FOREIGN = "Este alumno no pertenece a este grupo";
const OFFLINE = "No se pudo leer la credencial. Revisa tu conexión.";
const RESTORED = "Recuperamos tus escaneos sin guardar";
const NONE: ReadonlySet<string> = new Set();

const reviewText = (absent: number): string =>
  absent === 1 ? "Revisa la lista: 1 alumno quedó como ausente." : `Revisa la lista: ${absent} alumnos quedaron como ausentes.`;
const duplicateText = (name: string): string => `Ya registrado: ${name}`;

/**
 * Taking attendance by scanning credentials. A good card makes its row
 * present; "Terminar" turns the rows still "Sin registrar" absent (see
 * scanSession for exactly which) for the teacher to review before the ordinary
 * "Guardar lista". Nothing reaches the server per scan: the session lives in
 * memory, mirrored to sessionStorage so a reload keeps it — and dropped when
 * the list is left or saved.
 */
export function useCardScan({ target, sheet, loadedFor, rows, resolve, setStatus }: Args): CardScanState {
  const draftKey = scanDraftKey(target);
  const [open, setOpen] = useState(false);
  const [session, setSession] = useState<ScanSession>(() => emptySession(draftKey));
  const [flash, setFlash] = useState<ScanFlash | undefined>(undefined);
  const [announcement, setAnnouncement] = useState("");
  const [banner, setBanner] = useState<string | undefined>(undefined);
  /** The stored draft of the list on screen, read before anything can clear it. */
  const [stash, setStash] = useState<{ key: string; draft: ScanDraft | undefined }>(() => ({ key: draftKey, draft: readScanDraft(target) }));
  if (stash.key !== draftKey) setStash({ key: draftKey, draft: readScanDraft(target) });

  const sessionRef = useRef(session);
  const lastSeen = useRef(new Map<string, number>());
  /** Codes resolved in this session: a known card of a scanned student needs no server call. */
  const codes = useRef(new Map<string, ScannedStudent>());
  /** Bumped whenever a session closes: a lookup answering later belongs to nobody. */
  const ticket = useRef(0);
  const feedback = useRef<ScanFeedback | undefined>(undefined);
  const flashTimer = useRef<number | undefined>(undefined);
  const flashId = useRef(0);
  /** The list whose session was already opened: the quiet re-read after a save must not reopen it. */
  const appliedFor = useRef<string | undefined>(undefined);
  const latest = useRef({ target, draftKey, rows, resolve, setStatus });
  latest.current = { target, draftKey, rows, resolve, setStatus };

  const commit = useCallback((next: ScanSession): void => {
    sessionRef.current = next;
    setSession(next);
  }, []);

  // A freshly loaded list: open its session, picking up what a reload interrupted.
  useEffect(() => {
    if (sheet === undefined || loadedFor === undefined || scanDraftKey(loadedFor) !== draftKey || appliedFor.current === draftKey) return;
    appliedFor.current = draftKey;
    codes.current.clear();
    const { session: opened, restore } = openSession(draftKey, sheet, stash.key === draftKey ? stash.draft : undefined);
    commit(opened);
    setBanner(undefined);
    if (restore === undefined) return;
    for (const [id, status] of Object.entries(restore)) latest.current.setStatus(id, status);
    setBanner(RESTORED);
  }, [sheet, loadedFor, draftKey, stash, commit]);

  // sessionStorage mirrors the session of the list on screen (its stored draft until the list arrives).
  const current = session.key === draftKey;
  useEffect(() => {
    writeScanDraft(latest.current.target, current ? draftOf(session, rows) : stash.draft);
  }, [current, session, rows, stash]);

  // Leaving a list (another day, class, or screen) discards its unsaved session. A reload never runs this.
  const { groupId, subjectId, date } = target;
  useEffect(() => {
    const list: SheetTarget = { groupId, subjectId, date };
    return () => {
      clearScanDraft(list);
      appliedFor.current = undefined;
      ticket.current += 1;
    };
  }, [groupId, subjectId, date]);

  useEffect(() => () => {
    window.clearTimeout(flashTimer.current);
    feedback.current?.dispose();
  }, []);

  const show = useCallback((tone: ScanTone, text: string, spoken: string): void => {
    flashId.current += 1;
    setFlash({ id: flashId.current, tone, text });
    setAnnouncement(spoken);
    feedback.current?.[tone === "duplicate" ? "duplicate" : tone === "ok" ? "ok" : "error"]();
    window.clearTimeout(flashTimer.current);
    flashTimer.current = window.setTimeout(() => setFlash(undefined), FLASH_MS[tone]);
  }, []);

  const accept = useCallback((code: string, result: CardScanResult): void => {
    if (result.kind === "rejected") {
      show("error", result.message, result.message);
      return;
    }
    const { enrollmentId, name } = result.student;
    if (!latest.current.rows.some((row) => row.enrollmentId === enrollmentId)) {
      show("error", FOREIGN, FOREIGN);
      return;
    }
    codes.current.set(code, result.student);
    if (sessionRef.current.scanned.has(enrollmentId)) {
      show("duplicate", duplicateText(name), duplicateText(name));
      return;
    }
    latest.current.setStatus(enrollmentId, "present");
    commit(withScan(sessionRef.current, enrollmentId));
    show("ok", name, `Presente: ${name}`);
  }, [commit, show]);

  const onCode = useCallback((code: string): void => {
    const resolveCard = latest.current.resolve;
    const now = Date.now();
    const seen = lastSeen.current.get(code);
    // Every sighting slides the window: a card still in view is the same read.
    lastSeen.current.set(code, now);
    if (resolveCard === undefined || (seen !== undefined && now - seen < SAME_CODE_DEBOUNCE_MS)) return;
    const known = codes.current.get(code);
    if (known !== undefined && sessionRef.current.scanned.has(known.enrollmentId)) {
      show("duplicate", duplicateText(known.name), duplicateText(known.name));
      return;
    }
    const mine = ticket.current;
    resolveCard(code).then(
      (result) => mine === ticket.current && accept(code, result),
      () => mine === ticket.current && show("error", OFFLINE, OFFLINE),
    );
  }, [accept, show]);

  /** Must run inside the click: iOS only unlocks audio within a user gesture. */
  const start = useCallback((): void => {
    feedback.current?.dispose();
    feedback.current = createScanFeedback();
    lastSeen.current.clear();
    ticket.current += 1;
    setBanner(undefined);
    setAnnouncement("");
    setOpen(true);
  }, []);

  /**
   * With nothing scanned (a camera that never opened, a change of mind) the
   * list is left as it was: turning the whole class absent would be one
   * "Guardar lista" away from a notice to every family.
   */
  const finish = useCallback((): void => {
    setOpen(false);
    setFlash(undefined);
    feedback.current?.dispose();
    feedback.current = undefined;
    ticket.current += 1;
    if (sessionRef.current.scanned.size === 0) return;
    const rest = [...unregistered(sessionRef.current)];
    for (const id of rest) latest.current.setStatus(id, "absent");
    setBanner(rest.length === 0 ? undefined : reviewText(rest.length));
  }, []);

  const touch = useCallback((enrollmentIds: readonly string[]): void => {
    if (sessionRef.current.key === latest.current.draftKey) commit(touched(sessionRef.current, enrollmentIds));
  }, [commit]);

  const saved = useCallback((list: SheetTarget): void => {
    clearScanDraft(list);
    if (scanDraftKey(list) !== sessionRef.current.key) return;
    ticket.current += 1;
    codes.current.clear();
    commit(emptySession(sessionRef.current.key));
    setBanner(undefined);
  }, [commit]);

  const pending = current ? unregistered(session) : NONE;
  return { open, scanned: current ? session.scanned : NONE, unregistered: pending, flash, announcement, banner, start, finish, onCode, touch, saved };
}
