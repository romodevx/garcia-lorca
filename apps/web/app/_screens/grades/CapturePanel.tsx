"use client";

import { useEffect, useState, type ReactElement } from "react";
import type { TeachingAssignment, Term } from "../../config/api";
import type { RosterRow } from "../../grades/api";
import { EmptyHint, ErrorAlert, Section, SelectField, type SelectOption } from "../config/parts";
import { GradeRow } from "./GradeRow";

export interface CapturePanelProps {
  assignments: readonly TeachingAssignment[];
  /** Readable "group · subject · teacher" label (ids resolved by the orchestrator). */
  assignmentLabel: (assignment: TeachingAssignment) => string;
  /** Terms of the chosen assignment's cycle. */
  loadTerms: (cycleId: string) => Promise<Term[]>;
  loadRoster: (assignmentId: string, termId: string) => Promise<RosterRow[]>;
  /** Resolve a student's display name by id (from the loaded students list). */
  studentName: (studentId: string) => string;
  onCapture: (assignmentId: string, enrollmentId: string, termId: string, value: number) => Promise<void>;
}

/**
 * Grade-capture surface, fewest-clicks: pick a teaching assignment (group +
 * subject + teacher), pick a term of that assignment's cycle, then edit each
 * enrolled student's 0–10 grade in the roster grid. Cascades mirror the config /
 * students panels; the grid resolves student names by id and reflects a saved
 * value locally. The gateway enforces the same invariants — this panel is UX only.
 */
export function CapturePanel({
  assignments,
  assignmentLabel,
  loadTerms,
  loadRoster,
  studentName,
  onCapture,
}: CapturePanelProps): ReactElement {
  const [assignmentId, setAssignmentId] = useState("");
  const [terms, setTerms] = useState<Term[]>([]);
  const [termId, setTermId] = useState("");
  const [roster, setRoster] = useState<RosterRow[]>([]);
  const [loadingRoster, setLoadingRoster] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  // Assignment → the terms of its cycle; reset the term + roster downstream.
  useEffect(() => {
    if (assignmentId === "") return;
    const assignment = assignments.find((a) => a.id === assignmentId);
    if (assignment === undefined) return;
    let active = true;
    setTermId("");
    setTerms([]);
    setRoster([]);
    setError(undefined);
    loadTerms(assignment.cycleId)
      .then((t) => active && setTerms(t))
      .catch(() => active && setError("No se pudieron cargar los periodos."));
    return () => {
      active = false;
    };
  }, [assignmentId, assignments, loadTerms]);

  // Assignment + term → the roster grid.
  useEffect(() => {
    if (assignmentId === "" || termId === "") return;
    let active = true;
    setLoadingRoster(true);
    setError(undefined);
    loadRoster(assignmentId, termId)
      .then((r) => active && setRoster(r))
      .catch(() => active && setError("No se pudo cargar el roster."))
      .finally(() => active && setLoadingRoster(false));
    return () => {
      active = false;
    };
  }, [assignmentId, termId, loadRoster]);

  const handleSave = async (enrollmentId: string, value: number): Promise<void> => {
    await onCapture(assignmentId, enrollmentId, termId, value);
    // Reflect the saved value locally so the row stays truthful without a refetch.
    setRoster((prev) => prev.map((r) => (r.enrollmentId === enrollmentId ? { ...r, value, hasValue: true } : r)));
  };

  if (assignments.length === 0) {
    return (
      <Section title="Capturar calificaciones" subtitle="Elige una asignación docente para comenzar.">
        <EmptyHint>Crea grupos y asigna profesores en Configuración para poder capturar calificaciones.</EmptyHint>
      </Section>
    );
  }

  const assignmentOptions: SelectOption[] = assignments.map((a) => ({ value: a.id, label: assignmentLabel(a) }));
  const termOptions: SelectOption[] = terms.map((t) => ({ value: t.id, label: t.name }));

  return (
    <Section title="Capturar calificaciones" subtitle="Elige la asignación y el periodo; captura o edita la calificación (0–10) de cada alumno.">
      <form aria-label="Elegir asignación y periodo">
        <div style={{ display: "grid", gridTemplateColumns: "2fr 1fr", gap: 12, marginBottom: 16 }}>
          <SelectField
            label="Asignación"
            value={assignmentId}
            options={assignmentOptions}
            onChange={setAssignmentId}
            placeholder="Selecciona una asignación…"
          />
          <SelectField
            label="Periodo"
            value={termId}
            options={termOptions}
            onChange={setTermId}
            placeholder="Selecciona un periodo…"
            disabled={assignmentId === "" || terms.length === 0}
          />
        </div>
      </form>

      {assignmentId === "" || termId === "" ? (
        <EmptyHint>Elige una asignación y un periodo para ver el roster.</EmptyHint>
      ) : loadingRoster ? (
        <EmptyHint>Cargando roster…</EmptyHint>
      ) : roster.length === 0 ? (
        <EmptyHint>Este grupo aún no tiene alumnos inscritos.</EmptyHint>
      ) : (
        <table aria-label="Roster de calificaciones" style={{ width: "100%", borderCollapse: "collapse" }}>
          <thead>
            <tr>
              <th style={{ textAlign: "left", padding: "0 8px 8px", fontSize: 13, fontWeight: 600, color: "var(--text-muted)" }}>Alumno</th>
              <th style={{ textAlign: "left", padding: "0 8px 8px", fontSize: 13, fontWeight: 600, color: "var(--text-muted)" }}>Calificación</th>
              <th style={{ width: 1 }} />
            </tr>
          </thead>
          <tbody>
            {roster.map((r) => (
              <GradeRow key={r.enrollmentId} row={r} studentName={studentName(r.studentId)} onSave={handleSave} />
            ))}
          </tbody>
        </table>
      )}
      {error !== undefined && <ErrorAlert message={error} />}
    </Section>
  );
}
