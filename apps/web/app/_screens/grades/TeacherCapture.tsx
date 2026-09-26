"use client";

import { useEffect, useState, type ReactElement } from "react";
import { Icon } from "../../_design/Icon";
import { Page } from "../../_design/Page";
import { listAssignments, listTerms, type TeachingAssignment, type TeachingTerm } from "../../teaching/api";
import { ErrorAlert } from "../config/parts";
import { TeachingPicker } from "../teaching/TeachingPicker";
import { TeacherGradeList } from "./TeacherGradeList";

export interface TeacherCaptureProps {
  onBack: () => void;
}

/**
 * Teacher-scoped grade-capture surface (module_grades, `teaching:read/write`).
 * The self-scoped sibling of the director's screen: it talks ONLY to the
 * `/api/v1/teaching/*` client, so a teacher never triggers a students-list,
 * `/grades/*`, or `/config/*` fetch (all of which they lack permission for).
 *
 * Fewest taps: subject → group → the capture list, each step skipped when it
 * has a single choice (TeachingPicker), then the term inside the list. The
 * gateway is the security boundary — this is UX only.
 */
export function TeacherCapture({ onBack }: TeacherCaptureProps): ReactElement {
  const [assignments, setAssignments] = useState<TeachingAssignment[]>([]);
  const [terms, setTerms] = useState<TeachingTerm[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | undefined>(undefined);

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

  return (
    <Page>
      <button
        onClick={onBack}
        className="fade-up"
        style={{ display: "inline-flex", alignItems: "center", gap: 7, minHeight: 44, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 6, cursor: "pointer" }}
      >
        <Icon name="back" size={18} stroke={2.4} />
        Volver al inicio
      </button>

      <div className="fade-up" style={{ display: "flex", alignItems: "center", gap: 16, marginBottom: 20 }}>
        <div style={{ width: 56, height: 56, flexShrink: 0, borderRadius: 18, display: "grid", placeItems: "center", background: "var(--yellow)", color: "#3A2E00", boxShadow: "0 6px 16px color-mix(in oklab, var(--yellow) 40%, transparent)" }}>
          <Icon name="grades" size={30} />
        </div>
        <div>
          <h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>Calificaciones</h1>
          <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>Captura por materia y grupo · solo tus grupos</p>
        </div>
      </div>

      <div className="fade-up">
        {loading ? (
          <p style={{ color: "var(--text-muted)", fontSize: 15 }}>Cargando…</p>
        ) : loadError !== undefined ? (
          <ErrorAlert message={loadError} />
        ) : (
          <TeachingPicker title="Calificaciones" options={assignments} bySubject>
            {(assignment) => <TeacherGradeList key={assignment.id} assignment={assignment} terms={terms} />}
          </TeachingPicker>
        )}
      </div>
    </Page>
  );
}
