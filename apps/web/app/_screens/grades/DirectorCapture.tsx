"use client";

import { useMemo, useState, type ReactElement } from "react";

import { Icon } from "../../_design/Icon";
import { Page } from "../../_design/Page";
import { SelectField } from "../../_design/form";
import { API_URL } from "../../api-url";
import { gradeSheetClient } from "../../grades/api";
import { EmptyHint, ErrorAlert } from "../config/parts";
import { MonthlyGradeSheet } from "./MonthlyGradeSheet";
import { useDirectorAssignments } from "./useDirectorAssignments";

export interface DirectorCaptureProps {
  onBack: () => void;
}

/**
 * Director-scoped grade capture (module_grades, `grades:read/write`): any
 * group's assignment, school-wide, then the same monthly sheet the teacher
 * uses. Names come from the sheet itself, so no students list is fetched. The
 * gateway is the security boundary; this is UX only.
 */
export function DirectorCapture({ onBack }: DirectorCaptureProps): ReactElement {
  const { choices, loading, failed } = useDirectorAssignments();
  const [assignmentId, setAssignmentId] = useState("");
  const client = useMemo(() => gradeSheetClient(API_URL), []);

  return (
    <Page>
      <button onClick={onBack} className="fade-up" style={{ display: "inline-flex", alignItems: "center", gap: 7, minHeight: 44, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 6, cursor: "pointer" }}>
        <Icon name="back" size={18} stroke={2.4} />
        Volver al inicio
      </button>

      <div className="fade-up" style={{ display: "flex", alignItems: "center", gap: 16, marginBottom: 20 }}>
        <div style={{ width: 56, height: 56, flexShrink: 0, borderRadius: 18, display: "grid", placeItems: "center", background: "var(--yellow)", color: "#3A2E00", boxShadow: "0 6px 16px color-mix(in oklab, var(--yellow) 40%, transparent)" }}>
          <Icon name="grades" size={30} />
        </div>
        <div>
          <h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>Calificaciones</h1>
          <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>Captura mensual por asignación · se guarda sola</p>
        </div>
      </div>

      <div className="fade-up">
        {failed ? (
          <ErrorAlert message="No se pudieron cargar las asignaciones." />
        ) : (
          <div style={{ maxWidth: 520, marginBottom: 16 }}>
            <SelectField
              label="Asignación"
              value={assignmentId}
              options={choices.map((c) => ({ value: c.id, label: c.label }))}
              onChange={setAssignmentId}
              disabled={loading}
              help="El grupo, la materia y quien la imparte. Ordenadas por nivel y grado."
            />
          </div>
        )}
        {!failed && assignmentId === "" && <EmptyHint>Elige una asignación para capturar sus calificaciones del mes.</EmptyHint>}
        {assignmentId !== "" && <MonthlyGradeSheet key={assignmentId} assignmentId={assignmentId} client={client} />}
      </div>
    </Page>
  );
}
