"use client";

import { useEffect, useMemo, useState, type ReactElement } from "react";
import { Icon } from "../../_design/Icon";
import { Page } from "../../_design/Page";
import {
  captureGrade,
  getRoster,
  listAssignments,
  listTerms,
  type TeachingAssignment,
  type TeachingRosterRow,
  type TeachingTerm,
} from "../../teaching/api";
import { SelectField, type SelectOption } from "../../_design/form";
import { EmptyHint, ErrorAlert, Section } from "../config/parts";
import { GradeRow } from "./GradeRow";

export interface TeacherCaptureProps {
  onBack: () => void;
}

/**
 * Teacher-scoped grade-capture surface (module_grades, `teaching:read/write`).
 * The self-scoped sibling of the director's screen: it talks ONLY to the
 * `/api/v1/teaching/*` client, so a teacher never triggers a students-list,
 * `/grades/*`, or `/config/*` fetch (all of which they lack permission for).
 *
 * Cascade (fewest clicks): pick one of the teacher's own assignments
 * (group · subject) → pick a term of that assignment's cycle → edit each enrolled
 * student's 0–10 grade in the roster grid. Student NAMES come from the roster row
 * itself (the teacher cannot resolve them from a students list). The capture call
 * carries only {assignmentId, enrollmentId, termId, value}; subject/teacher are
 * derived server-side. The gateway is the security boundary — this is UX only.
 */
export function TeacherCapture({ onBack }: TeacherCaptureProps): ReactElement {
  const [assignments, setAssignments] = useState<TeachingAssignment[]>([]);
  const [terms, setTerms] = useState<TeachingTerm[]>([]);
  const [assignmentId, setAssignmentId] = useState("");
  const [termId, setTermId] = useState("");
  const [roster, setRoster] = useState<TeachingRosterRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | undefined>(undefined);
  const [loadingRoster, setLoadingRoster] = useState(false);
  const [rosterError, setRosterError] = useState<string | undefined>(undefined);

  // Boot: the teacher's own assignments + the terms of their cycles.
  useEffect(() => {
    let active = true;
    setLoading(true);
    void (async (): Promise<void> => {
      try {
        const [asgs, ts] = await Promise.all([listAssignments(), listTerms()]);
        if (!active) return;
        setAssignments(asgs);
        setTerms(ts);
      } catch {
        if (active) setLoadError("No se pudieron cargar tus asignaciones.");
      } finally {
        if (active) setLoading(false);
      }
    })();
    return () => {
      active = false;
    };
  }, []);

  const selectedAssignment = assignments.find((a) => a.id === assignmentId);

  // Only the terms belonging to the chosen assignment's cycle are offered.
  const cycleTerms = useMemo(
    () => (selectedAssignment === undefined ? [] : terms.filter((t) => t.cycleId === selectedAssignment.cycleId)),
    [terms, selectedAssignment],
  );

  // Assignment + term → the roster grid. The `active` guard drops a stale
  // response so a mid-flight switch can never paint the wrong roster.
  useEffect(() => {
    if (assignmentId === "" || termId === "") {
      setRoster([]);
      return;
    }
    let active = true;
    setLoadingRoster(true);
    setRosterError(undefined);
    getRoster(assignmentId, termId)
      .then((r) => active && setRoster(r))
      .catch(() => active && setRosterError("No se pudo cargar el roster."))
      .finally(() => {
        if (active) setLoadingRoster(false);
      });
    return () => {
      active = false;
    };
  }, [assignmentId, termId]);

  // Switching assignment clears the downstream term so a stale pair can't persist.
  const handleAssignmentChange = (id: string): void => {
    setAssignmentId(id);
    setTermId("");
    setRoster([]);
  };

  const handleSave = async (enrollmentId: string, value: number): Promise<void> => {
    // subject/teacher are DERIVED server-side from the assignment — never sent.
    await captureGrade({ assignmentId, enrollmentId, termId, value });
    // Reflect the saved value locally so the row stays truthful without a refetch.
    setRoster((prev) => prev.map((r) => (r.enrollmentId === enrollmentId ? { ...r, value, hasValue: true } : r)));
  };

  const assignmentOptions: SelectOption[] = assignments.map((a) => ({ value: a.id, label: `${a.groupName} · ${a.subjectName}` }));
  const termOptions: SelectOption[] = cycleTerms.map((t) => ({ value: t.id, label: t.name }));

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
        <div style={{ width: 56, height: 56, borderRadius: 18, display: "grid", placeItems: "center", background: "var(--yellow)", color: "#3A2E00", boxShadow: "0 6px 16px color-mix(in oklab, var(--yellow) 40%, transparent)" }}>
          <Icon name="grades" size={30} />
        </div>
        <div>
          <h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>Calificaciones</h1>
          <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>Captura por asignación y periodo · solo tus grupos</p>
        </div>
      </div>

      {loading ? (
        <p className="fade-up" style={{ color: "var(--text-muted)", fontSize: 15 }}>Cargando…</p>
      ) : loadError !== undefined ? (
        <div className="fade-up">
          <ErrorAlert message={loadError} />
        </div>
      ) : assignments.length === 0 ? (
        <div className="fade-up">
          <Section title="Capturar calificaciones" subtitle="Aún no tienes grupos asignados.">
            <EmptyHint>No tienes asignaciones. Pide a la dirección que te asigne un grupo y materia.</EmptyHint>
          </Section>
        </div>
      ) : (
        <div className="fade-up">
          <Section title="Capturar calificaciones" subtitle="Elige la asignación y el periodo; captura o edita la calificación (0–10) de cada alumno.">
            <form aria-label="Elegir asignación y periodo">
              <div style={{ display: "grid", gridTemplateColumns: "2fr 1fr", gap: 12, marginBottom: 16 }}>
                <SelectField
                  label="Asignación"
                  value={assignmentId}
                  options={assignmentOptions}
                  onChange={handleAssignmentChange}
                  placeholder="Selecciona una asignación…"
                />
                <SelectField
                  label="Periodo"
                  value={termId}
                  options={termOptions}
                  onChange={setTermId}
                  placeholder="Selecciona un periodo…"
                  disabled={assignmentId === "" || termOptions.length === 0}
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
                    <GradeRow key={r.enrollmentId} row={r} studentName={r.studentName} onSave={handleSave} />
                  ))}
                </tbody>
              </table>
            )}
            {rosterError !== undefined && <ErrorAlert message={rosterError} />}
          </Section>
        </div>
      )}
    </Page>
  );
}
