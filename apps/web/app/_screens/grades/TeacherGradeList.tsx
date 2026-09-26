"use client";

import { useEffect, useMemo, useState, type ReactElement } from "react";

import { SelectField, type SelectOption } from "../../_design/form";
import { captureGrade, getRoster, type TeachingAssignment, type TeachingRosterRow, type TeachingTerm } from "../../teaching/api";
import { EmptyHint, ErrorAlert, Section } from "../config/parts";
import { groupLabel } from "../teaching/selection";
import { GradeRow } from "./GradeRow";

export interface TeacherGradeListProps {
  assignment: TeachingAssignment;
  /** Every term of the teacher's cycles; only this assignment's cycle is offered. */
  terms: readonly TeachingTerm[];
}

const TH = { textAlign: "left", padding: "0 8px 8px", fontSize: 13, fontWeight: 600, color: "var(--text-muted)" } as const;

/**
 * One class's capture list: pick the term, then edit each enrolled student's
 * 0–10 grade. Mounted per assignment (keyed by the picker's choice), so a
 * switch of class starts clean. Names come from the roster row itself; the
 * capture call carries only {assignmentId, enrollmentId, termId, value}.
 */
export function TeacherGradeList({ assignment, terms }: TeacherGradeListProps): ReactElement {
  const [termId, setTermId] = useState("");
  const [roster, setRoster] = useState<TeachingRosterRow[]>([]);
  const [loadingRoster, setLoadingRoster] = useState(false);
  const [rosterError, setRosterError] = useState<string | undefined>(undefined);

  const cycleTerms = useMemo(() => terms.filter((t) => t.cycleId === assignment.cycleId), [terms, assignment.cycleId]);

  // The `active` guard drops a stale response so a mid-flight switch can never
  // paint the wrong roster.
  useEffect(() => {
    if (termId === "") {
      setRoster([]);
      return;
    }
    let active = true;
    setLoadingRoster(true);
    setRosterError(undefined);
    getRoster(assignment.id, termId)
      .then((r) => active && setRoster(r))
      .catch(() => active && setRosterError("No se pudo cargar el roster."))
      .finally(() => {
        if (active) setLoadingRoster(false);
      });
    return () => {
      active = false;
    };
  }, [assignment.id, termId]);

  const handleSave = async (enrollmentId: string, value: number): Promise<void> => {
    // subject/teacher are DERIVED server-side from the assignment — never sent.
    await captureGrade({ assignmentId: assignment.id, enrollmentId, termId, value });
    setRoster((prev) => prev.map((r) => (r.enrollmentId === enrollmentId ? { ...r, value, hasValue: true } : r)));
  };

  const termOptions: SelectOption[] = cycleTerms.map((t) => ({ value: t.id, label: t.name }));

  return (
    <Section title={`${assignment.subjectName} · ${groupLabel(assignment)}`} subtitle="Elige el periodo; captura o edita la calificación (0–10) de cada alumno.">
      <form aria-label="Elegir periodo" style={{ maxWidth: 320, marginBottom: 16 }}>
        <SelectField
          label="Periodo"
          value={termId}
          options={termOptions}
          onChange={setTermId}
          help="El periodo de evaluación que estás capturando."
          disabled={termOptions.length === 0}
        />
      </form>

      {termId === "" ? (
        <EmptyHint>Elige un periodo para ver el roster.</EmptyHint>
      ) : loadingRoster ? (
        <EmptyHint>Cargando roster…</EmptyHint>
      ) : roster.length === 0 ? (
        <EmptyHint>Este grupo aún no tiene alumnos inscritos.</EmptyHint>
      ) : (
        <table aria-label="Roster de calificaciones" style={{ width: "100%", borderCollapse: "collapse" }}>
          <thead>
            <tr>
              <th style={TH}>Alumno</th>
              <th style={TH}>Calificación</th>
              <th style={{ width: 1 }} />
            </tr>
          </thead>
          <tbody>
            {roster.map((r) => (
              <GradeRow key={r.enrollmentId} row={r} studentName={r.studentName} onSave={handleSave} />
            ))}
          </tbody>
        </table>
      )}
      {rosterError !== undefined && <ErrorAlert message={rosterError} />}
    </Section>
  );
}
