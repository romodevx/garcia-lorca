"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import {
  getDailyRoster,
  saveDailyAttendance,
  type AttendanceException,
  type AttendanceStatus,
  type Mark,
  type RosterRow,
} from "../../attendance/api";
import { listCycles, listLabelledGroups, type Cycle, type LabelledGroup } from "../../config/api";
import { listStudents, type Student } from "../../students/api";

import { API_URL } from "../../api-url";

// Gateway base URL for the browser (public env; matches AppShell/compose).
const TOAST_MS = 2600;

const detailOf = (cause: unknown): string => (cause instanceof Error ? cause.message : String(cause));

/** Local calendar day as `YYYY-MM-DD` (the picker's and the gateway's format). */
function todayISO(): string {
  const now = new Date();
  const month = `${now.getMonth() + 1}`.padStart(2, "0");
  const day = `${now.getDate()}`.padStart(2, "0");
  return `${now.getFullYear()}-${month}-${day}`;
}

const isException = (status: AttendanceStatus): status is AttendanceException => status !== "present";

/** The non-present rows, in list order — exactly what the save payload carries. */
function exceptionsOf(rows: readonly RosterRow[]): Mark[] {
  return rows.flatMap((row) =>
    isException(row.status) ? [{ enrollmentId: row.enrollmentId, status: row.status }] : [],
  );
}

function tally(rows: readonly RosterRow[]): Record<AttendanceStatus, number> {
  const counts: Record<AttendanceStatus, number> = { present: 0, absent: 0, late: 0, excused: 0 };
  for (const row of rows) {
    counts[row.status] += 1;
  }
  return counts;
}

/** Everything the pass-list view needs: pickers, the day's rows, and the save. */
export interface PassListState {
  cycles: readonly Cycle[];
  groups: readonly LabelledGroup[];
  cycleId: string;
  setCycleId: (cycleId: string) => void;
  groupId: string;
  setGroupId: (groupId: string) => void;
  date: string;
  setDate: (date: string) => void;
  rows: readonly RosterRow[];
  counts: Record<AttendanceStatus, number>;
  loadingRoster: boolean;
  saving: boolean;
  /** Set ONLY when the shared boot pickers fail: the surface is not usable. */
  loadError?: string;
  /** Set when the day's list fails to load; the pickers still work. */
  rosterError?: string;
  /** Set when the save is rejected, so the day is never reported as stored. */
  saveError?: string;
  toast?: string;
  studentName: (studentId: string) => string;
  setStatus: (enrollmentId: string, status: AttendanceStatus) => void;
  save: () => Promise<void>;
}

/**
 * All state, reads and writes of the daily pass list — the single fetch owner,
 * kept out of the view so the panels stay presentational.
 *
 *   boot ──> cycles + students ──> cycleId ──> groups
 *   groupId + date ──> roster (every row PRESENT unless the day was already taken)
 *   Guardar ──> only the non-present rows travel; the server reconciles the day.
 *
 * `initialDate` exists so callers (and tests) can pin the day; it defaults to the
 * local today, which is what a teacher wants on open.
 */
export function usePassList(initialDate?: string): PassListState {
  const t = useTranslations("attendance");
  const [cycles, setCycles] = useState<Cycle[]>([]);
  const [groups, setGroups] = useState<LabelledGroup[]>([]);
  const [students, setStudents] = useState<Student[]>([]);
  const [cycleId, setCycleId] = useState("");
  const [groupId, setGroupId] = useState("");
  const [date, setDate] = useState(initialDate ?? todayISO());
  const [rows, setRows] = useState<RosterRow[]>([]);
  const [loadingRoster, setLoadingRoster] = useState(false);
  const [saving, setSaving] = useState(false);
  const [loadError, setLoadError] = useState<string | undefined>(undefined);
  const [rosterError, setRosterError] = useState<string | undefined>(undefined);
  const [saveError, setSaveError] = useState<string | undefined>(undefined);
  const [toast, setToast] = useState<string | undefined>(undefined);
  // Monotonic id of the newest roster read; an older response is dropped so a
  // slow request can never repaint another group's or day's list.
  const rosterRequest = useRef(0);
  // Handle of the pending toast-dismiss timer, so a rapid second toast clears
  // the first's timer instead of dismissing early, and unmount cancels it.
  const toastTimer = useRef<number | undefined>(undefined);

  const announce = useCallback((message: string): void => {
    if (toastTimer.current !== undefined) {
      window.clearTimeout(toastTimer.current);
    }
    setToast(message);
    toastTimer.current = window.setTimeout(() => setToast(undefined), TOAST_MS);
  }, []);

  // Cancel a pending toast timer on unmount (no state update after teardown).
  useEffect(
    () => () => {
      if (toastTimer.current !== undefined) {
        window.clearTimeout(toastTimer.current);
      }
    },
    []
  );

  // Boot: the cycle picker + the students that resolve the row names. Attendance
  // itself carries NO student PII — the roster is ids only.
  useEffect(() => {
    let active = true;
    Promise.all([listCycles(API_URL), listStudents(API_URL)])
      .then(([cs, ss]) => {
        if (!active) return;
        setCycles(cs);
        setStudents(ss);
        setCycleId((cs.find((cycle) => cycle.status === "active") ?? cs[0])?.id ?? "");
      })
      .catch(() => active && setLoadError(t("error.load")));
    return () => {
      active = false;
    };
  }, [t]);

  // Groups of the selected cycle; changing cycle drops the chosen group.
  useEffect(() => {
    if (cycleId === "") return;
    let active = true;
    setGroupId("");
    listLabelledGroups(API_URL, cycleId)
      .then((gs) => active && setGroups(gs))
      .catch(() => active && setGroups([]));
    return () => {
      active = false;
    };
  }, [cycleId]);

  // The day's pass list. Every read starts a new request id; only the newest may
  // write state (group and date can both change while a read is in flight).
  useEffect(() => {
    if (groupId === "" || date === "") {
      setRows([]);
      return;
    }
    const request = (rosterRequest.current += 1);
    const isCurrent = (): boolean => request === rosterRequest.current;
    setLoadingRoster(true);
    setRosterError(undefined);
    setSaveError(undefined);
    getDailyRoster(API_URL, { groupId, date })
      .then((roster) => {
        if (isCurrent()) setRows(roster);
      })
      .catch((cause: unknown) => {
        if (!isCurrent()) return;
        setRows([]);
        setRosterError(t("error.roster", { detail: detailOf(cause) }));
      })
      .finally(() => {
        if (isCurrent()) setLoadingRoster(false);
      });
  }, [groupId, date, t]);

  const studentName = useCallback(
    (id: string): string => {
      const student = students.find((candidate) => candidate.id === id);
      return student !== undefined ? `${student.firstName} ${student.lastName}` : id;
    },
    [students],
  );

  const setStatus = useCallback((enrollmentId: string, status: AttendanceStatus): void => {
    setRows((prev) => prev.map((row) => (row.enrollmentId === enrollmentId ? { ...row, status } : row)));
  }, []);

  const save = useCallback(async (): Promise<void> => {
    setSaving(true);
    setSaveError(undefined);
    try {
      // DEFAULT-PRESENT: only the exceptions travel; the server resets the rest.
      const { absentCount } = await saveDailyAttendance(API_URL, { groupId, date, marks: exceptionsOf(rows) });
      announce(t("toast.saved", { absent: absentCount }));
    } catch (cause) {
      setSaveError(t("error.save", { detail: detailOf(cause) }));
    } finally {
      setSaving(false);
    }
  }, [announce, date, groupId, rows, t]);

  const counts = useMemo(() => tally(rows), [rows]);

  return {
    cycles,
    groups,
    cycleId,
    setCycleId,
    groupId,
    setGroupId,
    date,
    setDate,
    rows,
    counts,
    loadingRoster,
    saving,
    loadError,
    rosterError,
    saveError,
    toast,
    studentName,
    setStatus,
    save,
  };
}
