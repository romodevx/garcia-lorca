import { useCallback, useEffect, useRef, useState } from "react";

import type { AttendanceStatus } from "../../attendance/api";
import type { CardScanResult } from "../../attendance/scan";
import type { AttendanceSheet, AttendanceSheetRow } from "../../attendance/sheet";
import type { ScanFlash, ScanTone } from "./QrScanDialog";
import { clearScanDraft, readScanDraft, scanDraftKey, writeScanDraft } from "./scanDraft";
import { createScanFeedback, type ScanFeedback } from "./scanFeedback";
import type { SheetTarget } from "./useAttendanceSheet";

export type ResolveCard = (code: string) => Promise<CardScanResult>;

export interface CardScanState {
  open: boolean;
  scanned: ReadonlySet<string>;
  flash?: ScanFlash;
  announcement: string;
  /** "Revisa la lista…" / "Recuperamos…": what the sheet says after scanning. */
  banner?: string;
  start: () => void;
  finish: () => void;
  onCode: (code: string) => void;
  /** After a successful save: the scans are on the server now. */
  saved: () => void;
}

interface Args {
  target: SheetTarget;
  sheet: AttendanceSheet | undefined;
  rows: readonly AttendanceSheetRow[];
  resolve: ResolveCard | undefined;
  setStatus: (enrollmentId: string, status: AttendanceStatus) => void;
}

/** The same code is ignored for this long: a card held in front of the camera is read many times. */
export const SAME_CODE_DEBOUNCE_MS = 2500;
const FLASH_MS: Record<ScanTone, number> = { ok: 1200, duplicate: 1600, error: 2500 };
const FOREIGN = "Este alumno no pertenece a este grupo";
const OFFLINE = "No se pudo leer la credencial. Revisa tu conexión.";
const RESTORED = "Recuperamos tus escaneos sin guardar";

const reviewText = (absent: number): string =>
  absent === 1 ? "Revisa la lista: 1 alumno quedó como ausente." : `Revisa la lista: ${absent} alumnos quedaron como ausentes.`;

/**
 * Taking attendance by scanning credentials. While the camera is open every
 * student not yet scanned is "Sin registrar"; a good card makes that row
 * present. "Terminar" turns the rest absent for the teacher to review before
 * the ordinary "Guardar lista". Nothing reaches the server per scan: the scans
 * live in memory, mirrored to sessionStorage so a reload keeps them.
 */
export function useCardScan({ target, sheet, rows, resolve, setStatus }: Args): CardScanState {
  const [open, setOpen] = useState(false);
  const [scanned, setScanned] = useState<ReadonlySet<string>>(new Set());
  const [flash, setFlash] = useState<ScanFlash | undefined>(undefined);
  const [announcement, setAnnouncement] = useState("");
  const [banner, setBanner] = useState<string | undefined>(undefined);
  const scannedRef = useRef<Set<string>>(new Set());
  const lastSeen = useRef(new Map<string, number>());
  const feedback = useRef<ScanFeedback | undefined>(undefined);
  const flashTimer = useRef<number | undefined>(undefined);
  const flashId = useRef(0);
  const latest = useRef({ target, rows, resolve, setStatus });
  latest.current = { target, rows, resolve, setStatus };

  const commit = useCallback((next: Set<string>): void => {
    scannedRef.current = next;
    setScanned(new Set(next));
    writeScanDraft(latest.current.target, [...next]);
  }, []);

  /** Every row not scanned becomes absent; returns how many. */
  const markRestAbsent = useCallback((ids: ReadonlySet<string>): number => {
    const rest = latest.current.rows.filter((row) => !ids.has(row.enrollmentId));
    for (const row of rest) latest.current.setStatus(row.enrollmentId, "absent");
    return rest.length;
  }, []);

  const draftKey = scanDraftKey(target);
  /** The list whose draft was already picked up: the quiet re-read after a save must not re-apply it. */
  const appliedFor = useRef<string | undefined>(undefined);

  // A freshly loaded list: pick up the scans a reload interrupted (or start clean).
  useEffect(() => {
    if (sheet === undefined || sheet.date !== latest.current.target.date || appliedFor.current === draftKey) return;
    appliedFor.current = draftKey;
    const onList = new Set(sheet.rows.map((row) => row.enrollmentId));
    const restored = new Set(readScanDraft(latest.current.target).filter((id) => onList.has(id)));
    scannedRef.current = restored;
    setScanned(new Set(restored));
    setBanner(undefined);
    if (restored.size === 0 || !sheet.editable) return;
    for (const id of restored) latest.current.setStatus(id, "present");
    markRestAbsent(restored);
    setBanner(RESTORED);
  }, [sheet, draftKey, markRestAbsent]);

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

  const accept = useCallback((result: CardScanResult): void => {
    if (result.kind === "rejected") {
      show("error", result.message, result.message);
      return;
    }
    const { enrollmentId, name } = result.student;
    if (!latest.current.rows.some((row) => row.enrollmentId === enrollmentId)) {
      show("error", FOREIGN, FOREIGN);
      return;
    }
    if (scannedRef.current.has(enrollmentId)) {
      show("duplicate", `Ya registrado: ${name}`, `Ya registrado: ${name}`);
      return;
    }
    latest.current.setStatus(enrollmentId, "present");
    commit(new Set(scannedRef.current).add(enrollmentId));
    show("ok", name, `Presente: ${name}`);
  }, [commit, show]);

  const onCode = useCallback((code: string): void => {
    const resolveCard = latest.current.resolve;
    const now = Date.now();
    const seen = lastSeen.current.get(code);
    if (resolveCard === undefined || (seen !== undefined && now - seen < SAME_CODE_DEBOUNCE_MS)) return;
    lastSeen.current.set(code, now);
    resolveCard(code).then(accept, () => show("error", OFFLINE, OFFLINE));
  }, [accept, show]);

  /** Must run inside the click: iOS only unlocks audio within a user gesture. */
  const start = useCallback((): void => {
    feedback.current?.dispose();
    feedback.current = createScanFeedback();
    lastSeen.current.clear();
    setBanner(undefined);
    setAnnouncement("");
    setOpen(true);
  }, []);

  const finish = useCallback((): void => {
    setOpen(false);
    setFlash(undefined);
    feedback.current?.dispose();
    feedback.current = undefined;
    setBanner(reviewText(markRestAbsent(scannedRef.current)));
  }, [markRestAbsent]);

  const saved = useCallback((): void => {
    clearScanDraft(latest.current.target);
    scannedRef.current = new Set();
    setScanned(new Set());
    setBanner(undefined);
  }, []);

  return { open, scanned, flash, announcement, banner, start, finish, onCode, saved };
}
