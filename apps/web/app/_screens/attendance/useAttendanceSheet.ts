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
  save: () => Promise<void>;
}

/**
 * One list's reads and writes: load the day, edit rows locally, save only the
 * exceptions, then re-read quietly so the "guardada por" line is the server's.
 * A monotonic request id drops stale responses (day or class switched mid-flight).
 */
export function useAttendanceSheet(target: SheetTarget, load: LoadSheet, save: SaveSheet): AttendanceSheetState {
  const { groupId, subjectId, date } = target;
  const [sheet, setSheet] = useState<AttendanceSheet | undefined>(undefined);
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

  const read = useCallback(async (quiet: boolean): Promise<void> => {
    const id = (request.current += 1);
    if (!quiet) {
      setLoading(true);
      setLoadError(undefined);
    }
    try {
      const next = await io.current.load(query);
      if (id !== request.current) return;
      setSheet(next);
      setRows(next.rows);
    } catch (cause) {
      if (id === request.current && !quiet) setLoadError(reasonOf(cause, LOAD_FALLBACK));
    } finally {
      if (id === request.current) setLoading(false);
    }
  }, [query]);

  useEffect(() => {
    if (query.date === "") return;
    setSaveError(undefined);
    void read(false);
  }, [query, read]);

  useEffect(() => () => window.clearTimeout(toastTimer.current), []);

  const setStatus = useCallback((enrollmentId: string, status: AttendanceStatus) => {
    setRows((prev) => prev.map((row) => (row.enrollmentId === enrollmentId ? { ...row, status } : row)));
  }, []);
  const setNote = useCallback((enrollmentId: string, note: string) => {
    setRows((prev) => prev.map((row) => (row.enrollmentId === enrollmentId ? { ...row, note } : row)));
  }, []);
  const allPresent = useCallback(() => setRows((prev) => prev.map((row) => ({ ...row, status: "present" }))), []);

  const saveList = useCallback(async (): Promise<void> => {
    if (saving) return;
    setSaving(true);
    setSaveError(undefined);
    try {
      await io.current.save({ ...query, marks: exceptionsOf(rows) });
      window.clearTimeout(toastTimer.current);
      setToast("Lista guardada");
      toastTimer.current = window.setTimeout(() => setToast(undefined), TOAST_MS);
      await read(true);
    } catch (cause) {
      setSaveError(reasonOf(cause, SAVE_FALLBACK));
    } finally {
      setSaving(false);
    }
  }, [query, read, rows, saving]);

  const counts = useMemo(() => tally(rows), [rows]);
  const dirty = useMemo(() => sheet !== undefined && isDirty(rows, sheet.rows), [rows, sheet]);

  return { sheet, rows, counts, dirty, loading, loadError, saving, saveError, toast, setStatus, setNote, allPresent, save: saveList };
}
