"use client";

import { useCallback, useEffect, useMemo, useState, type ReactElement } from "react";
import { Icon } from "../../_design/Icon";
import { Page } from "../../_design/Page";
import type { AttendanceException, AttendanceStatus, Mark } from "../../attendance/api";
import {
  getAttendanceRoster,
  listGroups,
  saveAttendance,
  type TeacherGroup,
  type TeachingAttendanceRosterRow,
} from "../../teaching/api";
import { DateField, SelectField, type SelectOption } from "../../_design/form";
import { EmptyHint, ErrorAlert, Section } from "../config/parts";
import { StatusCounters, StatusPicker, type CounterDef, type StatusOption } from "./parts";

export interface TeacherAttendanceProps {
  onBack: () => void;
  /** Pins the day (tests, deep links); defaults to the local today. */
  today?: string;
}

/** Present sits leftmost — the default the thumb already rests on (fewest clicks). */
const ORDER: readonly AttendanceStatus[] = ["present", "absent", "late", "excused"];
const LABEL: Record<AttendanceStatus, string> = {
  present: "Presente",
  absent: "Ausente",
  late: "Retardo",
  excused: "Justificado",
};

/** Local calendar day as `YYYY-MM-DD` (the picker's and the gateway's format). */
function todayISO(): string {
  const now = new Date();
  const month = `${now.getMonth() + 1}`.padStart(2, "0");
  const day = `${now.getDate()}`.padStart(2, "0");
  return `${now.getFullYear()}-${month}-${day}`;
}

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
 * Teacher-scoped daily pass list (module_attendance, `attendance_own:read/write`).
 * The self-scoped sibling of the director's screen: it talks ONLY to the
 * `/api/v1/teaching/*` client, so a teacher never triggers a students-list,
 * admin `/attendance/*`, or `/config/*` fetch (all of which they lack permission
 * for).
 *
 * Cascade (fewest clicks): pick one of the teacher's OWN groups → pick the day
 * (defaults to today) → the whole roster opens PRESENT and the teacher marks only
 * the exceptions → one "Guardar" sends the COMPLETE exception set (only the
 * non-present rows travel; the server reconciles the rest to present). Student
 * NAMES come from the roster row itself (the teacher cannot list students). The
 * gateway is the security boundary — this is UX only.
 */
export function TeacherAttendance({ onBack, today }: TeacherAttendanceProps): ReactElement {
  const [groups, setGroups] = useState<TeacherGroup[]>([]);
  const [groupId, setGroupId] = useState("");
  const [date, setDate] = useState(today ?? todayISO());
  const [rows, setRows] = useState<TeachingAttendanceRosterRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | undefined>(undefined);
  const [loadingRoster, setLoadingRoster] = useState(false);
  const [rosterError, setRosterError] = useState<string | undefined>(undefined);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | undefined>(undefined);
  const [savedAbsent, setSavedAbsent] = useState<number | undefined>(undefined);

  // Boot: the teacher's own groups.
  useEffect(() => {
    let active = true;
    setLoading(true);
    listGroups()
      .then((gs) => active && setGroups(gs))
      .catch(() => active && setLoadError("No se pudieron cargar tus grupos."))
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, []);

  // Group + day → the day's pass list. The `active` guard drops a stale response
  // so a mid-flight switch can never paint the wrong roster.
  useEffect(() => {
    if (groupId === "" || date === "") {
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
  const groupOptions: SelectOption[] = groups.map((g) => ({ value: g.id, label: g.name }));
  const statusOptions: readonly StatusOption[] = ORDER.map((status) => ({ status, label: LABEL[status] }));
  const counters: readonly CounterDef[] = ORDER.map((status) => ({ status, label: LABEL[status], count: counts[status] }));

  return (
    <Page>
      <button
        onClick={onBack}
        className="fade-up"
        style={{ display: "inline-flex", alignItems: "center", gap: 7, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 14, cursor: "pointer" }}
      >
        <Icon name="back" size={18} stroke={2.4} />
        Volver al inicio
      </button>

      <div className="fade-up" style={{ display: "flex", alignItems: "center", gap: 16, marginBottom: 24 }}>
        <div style={{ width: 56, height: 56, borderRadius: 18, display: "grid", placeItems: "center", background: "var(--lime)", color: "var(--primary-ink)", boxShadow: "0 6px 16px color-mix(in oklab, var(--lime) 40%, transparent)" }}>
          <Icon name="qr" size={30} />
        </div>
        <div>
          <h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>Asistencia</h1>
          <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>Pase de lista diario · solo tus grupos</p>
        </div>
      </div>

      {loading ? (
        <p className="fade-up" style={{ color: "var(--text-muted)", fontSize: 15 }}>Cargando…</p>
      ) : loadError !== undefined ? (
        <div className="fade-up">
          <ErrorAlert message={loadError} />
        </div>
      ) : groups.length === 0 ? (
        <div className="fade-up">
          <Section title="Pase de lista" subtitle="Aún no tienes grupos asignados.">
            <EmptyHint>No tienes grupos asignados. Pide a la dirección que te asigne un grupo.</EmptyHint>
          </Section>
        </div>
      ) : (
        <div className="fade-up">
          <Section title="Pase de lista" subtitle="Elige el grupo y el día; todos empiezan presentes, marca solo las excepciones.">
            <form aria-label="Elegir grupo y fecha">
              <div style={{ display: "grid", gridTemplateColumns: "2fr 1fr", gap: 12, marginBottom: 16 }}>
                <SelectField label="Grupo" value={groupId} options={groupOptions} onChange={setGroupId} placeholder="Selecciona un grupo…" />
                <DateField label="Fecha" value={date} onChange={setDate} />
              </div>
            </form>

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

            {groupId === "" ? (
              <EmptyHint>Selecciona un grupo para pasar lista.</EmptyHint>
            ) : loadingRoster ? (
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
        </div>
      )}
    </Page>
  );
}
