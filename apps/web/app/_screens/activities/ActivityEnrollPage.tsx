"use client";

import { useEffect, useMemo, useState, type ReactElement } from "react";

import { SelectField } from "../../_design/form";
import { Icon } from "../../_design/Icon";
import { Page } from "../../_design/Page";
import { Toast } from "../../_design/Toast";
import { DualPanelPicker } from "../../_design/students/DualPanelPicker";
import { formatStudentName } from "../../_design/students/studentName";
import { useStudentRoster, type RosterStudent } from "../../_design/students/useStudentRoster";
import { API_URL } from "../../api-url";
import { changeActivityOption, endEnrollments, enrollStudents, listEnrollments, type Activity } from "../../activities/api";
import { optionLabel, PERIODICITY_LABEL, scheduleSummary, sortOptions } from "../../activities/format";
import { ErrorAlert } from "../config/parts";
import { useToast } from "./useToast";

const LABELS = { available: "Alumnos", assigned: "Inscritos", assign: "Inscribir", unassign: "Dar de baja", emptyAssigned: "Nadie inscrito todavía." };

const plural = (n: number, one: string, many: string): string => `${n} ${n === 1 ? one : many}`;

export interface ActivityEnrollPageProps {
  activity: Activity;
  canWrite: boolean;
  onBack: () => void;
}

/**
 * Who takes an activity (T17): the shared DualPanelPicker with the batch's
 * price option, which each pupil moved in starts with, and a per-pupil select
 * on the right to change it. Ending an enrolment never touches issued charges,
 * so it needs no confirmation.
 */
export function ActivityEnrollPage({ activity, canWrite, onBack }: ActivityEnrollPageProps): ReactElement {
  const roster = useStudentRoster();
  const options = useMemo(() => sortOptions(activity.options), [activity.options]);
  const [batch, setBatch] = useState(options[0]?.id ?? "");
  /** studentId → optionId of who is enrolled now. */
  const [enrolled, setEnrolled] = useState<ReadonlyMap<string, string>>(new Map());
  const enrolledIds = useMemo(() => [...enrolled.keys()], [enrolled]);
  const [failure, setFailure] = useState<string | undefined>(undefined);
  const [busy, setBusy] = useState(false);
  const { toast, say } = useToast();

  useEffect(() => {
    let live = true;
    listEnrollments(API_URL, activity.id)
      .then((list) => live && setEnrolled(new Map(list.map((e) => [e.studentId, e.optionId]))))
      .catch((err: unknown) => live && setFailure(err instanceof Error ? err.message : "No se pudo leer quién está inscrito."));
    return () => {
      live = false;
    };
  }, [activity.id]);

  const run = async (work: () => Promise<void>): Promise<void> => {
    if (busy) return;
    setBusy(true);
    setFailure(undefined);
    try {
      await work();
    } catch (err: unknown) {
      setFailure(err instanceof Error ? err.message : "No se pudo guardar.");
    } finally {
      setBusy(false);
    }
  };

  const assign = (ids: string[]): void => void run(async () => {
    const res = await enrollStudents(API_URL, activity.id, ids.map((studentId) => ({ studentId, optionId: batch })));
    setEnrolled((prev) => new Map([...prev, ...res.enrollments.map((e): [string, string] => [e.studentId, e.optionId])]));
    say(`${plural(res.enrollments.length, "alumno inscrito", "alumnos inscritos")} · ${plural(res.chargesCreated, "cargo generado", "cargos generados")}`);
  });

  const unassign = (ids: string[]): void => void run(async () => {
    const ended = await endEnrollments(API_URL, activity.id, ids);
    setEnrolled((prev) => new Map([...prev].filter(([id]) => !ids.includes(id))));
    say(plural(ended, "alumno dado de baja", "alumnos dados de baja"));
  });

  const change = (student: RosterStudent, optionId: string): void => void run(async () => {
    const saved = await changeActivityOption(API_URL, activity.id, student.id, optionId);
    setEnrolled((prev) => new Map(prev).set(student.id, saved.optionId));
    say(`Esquema de ${formatStudentName(student)} actualizado`);
  });

  const optionOf = (student: RosterStudent): string => enrolled.get(student.id) ?? "";
  const optionText = (id: string): string => {
    const o = options.find((x) => x.id === id);
    return o === undefined ? "—" : optionLabel(o);
  };

  return (
    <Page>
      <button onClick={onBack} style={{ display: "inline-flex", alignItems: "center", gap: 7, minHeight: 44, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 8 }}>
        <Icon name="back" size={18} stroke={2.4} />Volver a actividades
      </button>
      <header style={{ marginBottom: 18 }}>
        <h1 style={{ fontSize: 28, fontWeight: 800, color: "var(--text)" }}>{`Inscritos en ${activity.name}`}</h1>
        <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>{`${scheduleSummary(activity.schedule)} · ${PERIODICITY_LABEL[activity.periodicity]}`}</p>
      </header>

      {failure !== undefined && <ErrorAlert message={failure} />}
      {roster.status === "failed" && <ErrorAlert message="No se pudo leer la lista de alumnos." />}
      {roster.status === "loading" && <p style={{ color: "var(--text-muted)" }}>Cargando alumnos…</p>}
      {roster.status === "ok" && (
        <DualPanelPicker
          students={roster.students}
          catalog={roster.catalog}
          assignedIds={enrolledIds}
          onAssign={assign}
          onUnassign={unassign}
          labels={LABELS}
          describe={(s) => s.room}
          readOnly={!canWrite}
          assignedToolbar={canWrite && (
            <SelectField label="Esquema de cobro" value={batch} options={options.map((o) => ({ value: o.id ?? "", label: optionLabel(o) }))}
              onChange={(v) => { if (v !== "") setBatch(v); }}
              help="Con este esquema entran los alumnos que pases a Inscritos. Después puedes cambiarlo por alumno." />
          )}
          renderAssignedExtra={(s) => canWrite ? (
            <select aria-label={`Esquema de ${formatStudentName(s)}`} value={optionOf(s)} onChange={(e) => change(s, e.target.value)}
              style={{ height: 44, maxWidth: 220, padding: "0 10px", borderRadius: "var(--r-md)", border: "1px solid var(--border)", background: "var(--surface)", color: "var(--text)", fontSize: 14 }}>
              {options.map((o) => <option key={o.id} value={o.id}>{optionLabel(o)}</option>)}
            </select>
          ) : (
            <span style={{ fontSize: 13.5, color: "var(--text-muted)" }}>{optionText(optionOf(s))}</span>
          )}
        />
      )}
      {toast !== undefined && <Toast>{toast}</Toast>}
    </Page>
  );
}
