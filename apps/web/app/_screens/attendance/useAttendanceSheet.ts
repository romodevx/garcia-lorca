import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import type { AttendanceStatus } from "../../attendance/api";
import type { AttendanceSheet, AttendanceSheetQuery, AttendanceSheetRow, SaveSheetInput } from "../../attendance/sheet";
import { exceptionsOf, isDirty, tally, type StatusCounts } from "./sheetModel";

const TOAST_MS = 2600;
const LOAD_FALLBACK = "No se pudo cargar la lista. Reintenta.";
const SAVE_FALLBACK = "No se pudo guardar la lista. Reintenta.";

/** The server writes its reasons for people (text/plain); show them, else a plain fallback. */
const reasonOf = (cause: unknown, fallback: string): string =>
  cause instanceof Error && cause.message !== "" && !cause.message.startsWith("HTTP ") ? cause.message : fallback;

export type LoadSheet = (q: AttendanceSheetQuery) => Promise<AttendanceSheet>;
export type SaveSheet = (input: SaveSheetInput) => Promise<void>;

export interface SheetTarget {
  groupId: string;
  subjectId?: string;
  date: string;
}

export interface AttendanceSheetState {
  sheet: AttendanceSheet | undefined;
  /** The list `sheet` was loaded for: until the new list arrives, the old one stays on screen. */
  loadedFor: AttendanceSheetQuery | undefined;
  rows: readonly AttendanceSheetRow[];
  counts: StatusCounts;
  dirty: boolean;
  loading: boolean;
  loadError?: string;
  saving: boolean;
  saveError?: string;
  toast?: string;
  setStatus: (enrollmentId: string, status: AttendanceStatus) => void;
  setNote: (enrollmentId: string, note: string) => void;
  allPresent: () => void;
  /**
   * Resolves the list the server now has — the one on screen when "Guardar"
   * was pressed, even if the day changed meanwhile — or undefined when the
   * save failed or was refused.
   */
  save: () => Promise<AttendanceSheetQuery | undefined>;
}

/**
 * One list's reads and writes: load the day, edit rows locally, save only the
 * exceptions (what was saved becomes the clean baseline at once), then re-read
 * quietly so the "guardada por" line is the server's —
 * only if that list is still the one on screen. Rows are locked while saving.
 * A monotonic request id drops stale responses (day or class switched mid-flight).
 */
export function useAttendanceSheet(target: SheetTarget, load: LoadSheet, save: SaveSheet): AttendanceSheetState {
  const { groupId, subjectId, date } = target;
  const [sheet, setSheet] = useState<AttendanceSheet | undefined>(undefined);
  const [loadedFor, setLoadedFor] = useState<AttendanceSheetQuery | undefined>(undefined);
  const [rows, setRows] = useState<AttendanceSheetRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [loadError, setLoadError] = useState<string | undefined>(undefined);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | undefined>(undefined);
  const [toast, setToast] = useState<string | undefined>(undefined);
  const request = useRef(0);
  const toastTimer = useRef<number | undefined>(undefined);
  const io = useRef({ load, save });
  io.current = { load, save };

  const query = useMemo<AttendanceSheetQuery>(
    () => ({ groupId, date, ...(subjectId !== undefined ? { subjectId } : {}) }),
    [groupId, subjectId, date],
  );
  /** The list on screen now — a save compares against it before re-reading. */
  const currentQuery = useRef(query);
  currentQuery.current = query;
  /** Mirrors `saving` synchronously so edits are refused while a save is in flight. */
  const savingNow = useRef(false);
  /** The list being saved: only ITS rows are locked; another day's list is free to edit. */
  const savingFor = useRef<AttendanceSheetQuery | undefined>(undefined);

  const read = useCallback(async (quiet: boolean, q: AttendanceSheetQuery): Promise<void> => {
    const id = (request.current += 1);
    if (!quiet) {
      setLoading(true);
      setLoadError(undefined);
    }
    try {
      const next = await io.current.load(q);
      if (id !== request.current) return;
      setSheet(next);
      setLoadedFor(q);
      setRows(next.rows);
    } catch (cause) {
      if (id === request.current && !quiet) setLoadError(reasonOf(cause, LOAD_FALLBACK));
    } finally {
      if (id === request.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (query.date === "") return;
    setSaveError(undefined);
    void read(false, query);
  }, [query, read]);

  useEffect(() => () => window.clearTimeout(toastTimer.current), []);

  /** The list being saved is locked: the post-save reread would otherwise drop the edit. */
  const edit = useCallback((change: (prev: AttendanceSheetRow[]) => AttendanceSheetRow[]) => {
    if (!savingNow.current || savingFor.current !== currentQuery.current) setRows(change);
  }, []);
  const setStatus = useCallback((enrollmentId: string, status: AttendanceStatus) => {
    edit((prev) => prev.map((row) => (row.enrollmentId === enrollmentId ? { ...row, status } : row)));
  }, [edit]);
  const setNote = useCallback((enrollmentId: string, note: string) => {
    edit((prev) => prev.map((row) => (row.enrollmentId === enrollmentId ? { ...row, note } : row)));
  }, [edit]);
  const allPresent = useCallback(() => edit((prev) => prev.map((row) => ({ ...row, status: "present" }))), [edit]);

  const saveList = useCallback(async (): Promise<AttendanceSheetQuery | undefined> => {
    if (savingNow.current) return undefined;
    const saved = query;
    savingNow.current = true;
    savingFor.current = saved;
    setSaving(true);
    setSaveError(undefined);
    try {
      await io.current.save({ ...saved, marks: exceptionsOf(rows) });
      window.clearTimeout(toastTimer.current);
      setToast("Lista guardada");
      toastTimer.current = window.setTimeout(() => setToast(undefined), TOAST_MS);
      // The day or class changed mid-save: that list's own load already owns the screen.
      if (currentQuery.current !== saved) return saved;
      // What was just saved IS the server's list now: it becomes the baseline at
      // once, so the screen is clean even if the quiet re-read below fails.
      setSheet((prev) => (prev === undefined ? prev : { ...prev, rows: [...rows] }));
      await read(true, saved);
      return saved;
    } catch (cause) {
      setSaveError(reasonOf(cause, SAVE_FALLBACK));
      return undefined;
    } finally {
      savingNow.current = false;
      setSaving(false);
    }
  }, [query, read, rows]);

  const counts = useMemo(() => tally(rows), [rows]);
  const dirty = useMemo(() => sheet !== undefined && isDirty(rows, sheet.rows), [rows, sheet]);

  return { sheet, loadedFor, rows, counts, dirty, loading, loadError, saving, saveError, toast, setStatus, setNote, allPresent, save: saveList };
}
