"use client";

import { useCallback, useEffect, useMemo, useState, type ReactElement } from "react";
import type { AttendanceException, AttendanceStatus, Mark } from "../../attendance/api";
import { DateField } from "../../_design/form";
import { getAttendanceRoster, saveAttendance, type TeacherGroup, type TeachingAttendanceRosterRow } from "../../teaching/api";
import { EmptyHint, ErrorAlert, Section } from "../config/parts";
import { groupLabel } from "../teaching/selection";
import { StatusCounters, StatusPicker, type CounterDef, type StatusOption } from "./parts";

export interface TeacherPassListProps {
  group: TeacherGroup;
  /** The day the list opens on (local today unless pinned). */
  today: string;
}

/** Present sits leftmost — the default the thumb already rests on (fewest clicks). */
const ORDER: readonly AttendanceStatus[] = ["present", "absent", "late", "excused"];
const LABEL: Record<AttendanceStatus, string> = {
  present: "Presente",
  absent: "Ausente",
  late: "Retardo",
  excused: "Justificado",
};

const isException = (status: AttendanceStatus): status is AttendanceException => status !== "present";

/** The non-present rows, in list order — exactly what the save payload carries. */
function exceptionsOf(rows: readonly TeachingAttendanceRosterRow[]): Mark[] {
  return rows.flatMap((row) => (isException(row.status) ? [{ enrollmentId: row.enrollmentId, status: row.status }] : []));
}

function tally(rows: readonly TeachingAttendanceRosterRow[]): Record<AttendanceStatus, number> {
  const counts: Record<AttendanceStatus, number> = { present: 0, absent: 0, late: 0, excused: 0 };
  for (const row of rows) {
    counts[row.status] += 1;
  }
  return counts;
}

/**
 * One group's pass list for a day: the whole roster opens PRESENT, the teacher
 * marks only the exceptions, and one "Guardar lista" sends the COMPLETE
 * exception set (the server reconciles the rest to present). Mounted per group
 * (keyed by the picker's choice), so a switch starts clean.
 */
export function TeacherPassList({ group, today }: TeacherPassListProps): ReactElement {
  const groupId = group.id;
  const [date, setDate] = useState(today);
  const [rows, setRows] = useState<TeachingAttendanceRosterRow[]>([]);
  const [loadingRoster, setLoadingRoster] = useState(false);
  const [rosterError, setRosterError] = useState<string | undefined>(undefined);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | undefined>(undefined);
  const [savedAbsent, setSavedAbsent] = useState<number | undefined>(undefined);

  // Group + day → the day's pass list. The `active` guard drops a stale response
  // so a mid-flight switch can never paint the wrong roster.
  useEffect(() => {
    if (date === "") {
      setRows([]);
      return;
    }
    let active = true;
    setLoadingRoster(true);
    setRosterError(undefined);
    setSaveError(undefined);
    setSavedAbsent(undefined);
    getAttendanceRoster(groupId, date)
      .then((r) => active && setRows(r))
      .catch(() => active && setRosterError("No se pudo cargar el pase de lista. Reintenta."))
      .finally(() => {
        if (active) setLoadingRoster(false);
      });
    return () => {
      active = false;
    };
  }, [groupId, date]);

  const setStatus = useCallback((enrollmentId: string, status: AttendanceStatus): void => {
    setSavedAbsent(undefined);
    setRows((prev) => prev.map((row) => (row.enrollmentId === enrollmentId ? { ...row, status } : row)));
  }, []);

  const handleSave = async (): Promise<void> => {
    setSaving(true);
    setSaveError(undefined);
    try {
      // DEFAULT-PRESENT: only the exceptions travel; the server resets the rest.
      const { absentCount } = await saveAttendance({ groupId, date, marks: exceptionsOf(rows) });
      setSavedAbsent(absentCount);
    } catch {
      setSaveError("No se pudo guardar la lista. Reintenta.");
    } finally {
      setSaving(false);
    }
  };

  const counts = useMemo(() => tally(rows), [rows]);
  const statusOptions: readonly StatusOption[] = ORDER.map((status) => ({ status, label: LABEL[status] }));
  const counters: readonly CounterDef[] = ORDER.map((status) => ({ status, label: LABEL[status], count: counts[status] }));

  return (
    <Section title={`Pase de lista · ${groupLabel({ groupName: group.name, gradeLevelName: group.gradeLevelName })}`} subtitle="Todos empiezan presentes; marca solo las excepciones.">
      <div style={{ maxWidth: 240, marginBottom: 16 }}>
        <DateField label="Fecha" value={date} onChange={setDate} />
      </div>

      {rows.length > 0 && (
        <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", justifyContent: "space-between", gap: 12, marginBottom: 14 }}>
          <StatusCounters counters={counters} />
          <button
            onClick={() => void handleSave()}
            disabled={saving}
            style={{ height: 44, padding: "0 22px", borderRadius: "var(--r-pill)", background: "var(--primary)", color: "var(--primary-ink)", fontWeight: 700, fontSize: 15, opacity: saving ? 0.6 : 1 }}
          >
            {saving ? "Guardando…" : "Guardar lista"}
          </button>
        </div>
      )}

      {loadingRoster ? (
        <EmptyHint>Cargando pase de lista…</EmptyHint>
      ) : rosterError !== undefined ? (
        <ErrorAlert message={rosterError} />
      ) : rows.length === 0 ? (
        <EmptyHint>Este grupo no tiene alumnos inscritos.</EmptyHint>
      ) : (
        <table aria-label="Pase de lista" style={{ width: "100%", borderCollapse: "collapse" }}>
          <thead>
            <tr>
              <th style={{ textAlign: "left", padding: "0 8px 8px", fontSize: 13, fontWeight: 600, color: "var(--text-muted)" }}>Alumno</th>
              <th style={{ width: 1 }} />
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.enrollmentId} style={{ borderTop: "1px solid var(--border)" }}>
                <td style={{ padding: "10px 8px", fontSize: 15, color: "var(--text)", fontWeight: 600 }}>{row.studentName}</td>
                <td style={{ padding: "10px 8px", textAlign: "right" }}>
                  <StatusPicker
                    label={`Asistencia de ${row.studentName}`}
                    options={statusOptions}
                    value={row.status}
                    onChange={(status) => setStatus(row.enrollmentId, status)}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {saveError !== undefined && <ErrorAlert message={saveError} />}
      {savedAbsent !== undefined && (
        <p
          role="status"
          style={{ margin: "12px 0 0", padding: "10px 14px", borderRadius: "var(--r-md)", background: "color-mix(in oklab, var(--forest) 12%, var(--surface))", color: "var(--forest)", fontSize: 13.5, fontWeight: 600 }}
        >
          Lista guardada · {savedAbsent} {savedAbsent === 1 ? "ausencia" : "ausencias"}.
        </p>
      )}
    </Section>
  );
}
