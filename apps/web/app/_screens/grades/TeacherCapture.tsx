"use client";

import { useCallback, useEffect, useRef, useState, type ReactElement } from "react";
import { Icon } from "../../_design/Icon";
import { Page } from "../../_design/Page";
import { listAssignments, teachingGradeSheet, type TeachingAssignment } from "../../teaching/api";
import { ErrorAlert } from "../config/parts";
import { TeachingPicker } from "../teaching/TeachingPicker";
import { PeriodGradeSheet } from "./PeriodGradeSheet";
import { confirmLeavingUnsaved, useGradeSaves } from "./useGradeSaves";
import { ModuleHeaderIcon } from "../../_design/ModuleHeaderIcon";

export interface TeacherCaptureProps {
  onBack: () => void;
}

/**
 * Teacher-scoped grade-capture surface (module_grades, `teaching:read/write`).
 * The self-scoped sibling of the director's screen: it talks ONLY to the
 * `/api/v1/teaching/*` client, so a teacher never triggers a students-list,
 * `/grades/*`, or `/config/*` fetch (all of which they lack permission for).
 *
 * Fewest taps: subject → group → the period's sheet, each step skipped when it
 * has a single choice (TeachingPicker); the evaluation period is a chip inside the sheet.
 * The gateway is the security boundary — this is UX only.
 */
export function TeacherCapture({ onBack }: TeacherCaptureProps): ReactElement {
  const [assignments, setAssignments] = useState<TeachingAssignment[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | undefined>(undefined);
  // Above the keyed sheet: a class switch keeps saves in flight and unsaved values.
  const saves = useGradeSaves(teachingGradeSheet);
  const openAssignment = useRef<string | undefined>(undefined);
  const canLeave = useCallback(() => {
    const open = openAssignment.current;
    return confirmLeavingUnsaved(open === undefined ? 0 : saves.unsavedCount({ assignmentId: open }));
  }, [saves]);

  // Boot: the teacher's own assignments.
  useEffect(() => {
    let active = true;
    setLoading(true);
    void (async (): Promise<void> => {
      try {
        const asgs = await listAssignments();
        if (active) setAssignments(asgs);
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
        <ModuleHeaderIcon icon="grades" color="yellow" />
        <div>
          <h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>Calificaciones</h1>
          <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>Captura por periodo, materia y grupo · solo tus grupos</p>
        </div>
      </div>

      <div className="fade-up">
        {loading ? (
          <p style={{ color: "var(--text-muted)", fontSize: 15 }}>Cargando…</p>
        ) : loadError !== undefined ? (
          <ErrorAlert message={loadError} />
        ) : (
          <TeachingPicker title="Calificaciones" options={assignments} bySubject canLeave={canLeave}>
            {(assignment) => (
              <OpenSheet key={assignment.id} assignmentId={assignment.id} openAssignment={openAssignment}>
                <PeriodGradeSheet assignmentId={assignment.id} client={teachingGradeSheet} saves={saves} />
              </OpenSheet>
            )}
          </TeachingPicker>
        )}
      </div>
    </Page>
  );
}

/** Records which class is open, for the "leave with unsaved grades?" question. */
function OpenSheet({ assignmentId, openAssignment, children }: { assignmentId: string; openAssignment: { current: string | undefined }; children: ReactElement }): ReactElement {
  useEffect(() => {
    openAssignment.current = assignmentId;
    return () => {
      openAssignment.current = undefined;
    };
  }, [assignmentId, openAssignment]);
  return children;
}
