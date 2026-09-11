"use client";

import { useCallback, useEffect, useState, type ReactElement } from "react";
import { Icon } from "../../_design/Icon";
import { Page } from "../../_design/Page";
import { listGroups, listSubjects, listTerms, type Group, type Subject, type Term } from "../../config/api";
import {
  listTeachers,
  listTeachingAssignments,
  type Teacher,
  type TeachingAssignment,
} from "../../teachers/api";
import { listStudents, type Student } from "../../students/api";
import { captureGrade, getRoster, type RosterRow } from "../../grades/api";
import { CapturePanel } from "./CapturePanel";

import { API_URL } from "../../api-url";

// Gateway base URL for the browser (public env; matches AppShell/compose).

export interface GradeCaptureProps {
  onBack: () => void;
}

/** Distinct non-empty ids, preserving first-seen order. */
function distinct(ids: readonly string[]): string[] {
  return [...new Set(ids.filter((id) => id !== ""))];
}

/**
 * Grade-capture surface (module_grades). The single fetch owner: on boot it loads
 * the teaching assignments (config), the students (students service, for name
 * resolution), and the groups/subjects/teachers needed to render readable
 * assignment labels — resolving group/subject/teacher names by id and falling back
 * to the id when a name is missing. It hands plain data + async callbacks
 * (loadTerms, loadRoster, onCapture) down to the dumb CapturePanel. The capture
 * call carries NO subject/teacher — the server derives them from the assignment.
 * The gateway (behind the flag + grades:read/write) is the security boundary.
 */
export function GradeCapture({ onBack }: GradeCaptureProps): ReactElement {
  const [assignments, setAssignments] = useState<TeachingAssignment[]>([]);
  const [students, setStudents] = useState<Student[]>([]);
  const [groups, setGroups] = useState<Group[]>([]);
  const [subjects, setSubjects] = useState<Subject[]>([]);
  const [teachers, setTeachers] = useState<Teacher[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | undefined>(undefined);

  // Boot: assignments + students + teachers, then the groups/subjects that back
  // the human-readable assignment labels (groups per referenced cycle, subjects
  // per referenced grade level).
  useEffect(() => {
    let active = true;
    setLoading(true);
    void (async (): Promise<void> => {
      try {
        const [asgs, ss, ts] = await Promise.all([
          listTeachingAssignments(API_URL),
          listStudents(API_URL),
          listTeachers(API_URL),
        ]);
        const groupLists = await Promise.all(distinct(asgs.map((a) => a.cycleId)).map((id) => listGroups(API_URL, id)));
        const gs = groupLists.flat();
        const subjectLists = await Promise.all(distinct(gs.map((g) => g.gradeLevelId)).map((id) => listSubjects(API_URL, id)));
        if (!active) return;
        setAssignments(asgs);
        setStudents(ss);
        setTeachers(ts);
        setGroups(gs);
        setSubjects(subjectLists.flat());
      } catch {
        if (active) setLoadError("No se pudieron cargar los datos de calificaciones.");
      } finally {
        if (active) setLoading(false);
      }
    })();
    return () => {
      active = false;
    };
  }, []);

  const groupName = useCallback((id: string): string => groups.find((g) => g.id === id)?.name ?? id, [groups]);
  const subjectName = useCallback((id: string): string => subjects.find((s) => s.id === id)?.name ?? id, [subjects]);
  const teacherName = useCallback(
    (id: string): string => {
      const t = teachers.find((x) => x.id === id);
      return t !== undefined ? `${t.firstName} ${t.lastName}` : id;
    },
    [teachers],
  );
  const studentName = useCallback(
    (id: string): string => {
      const s = students.find((x) => x.id === id);
      return s !== undefined ? `${s.firstName} ${s.lastName}` : id;
    },
    [students],
  );

  const assignmentLabel = useCallback(
    (a: TeachingAssignment): string => `${groupName(a.groupId)} · ${subjectName(a.subjectId)} · ${teacherName(a.teacherId)}`,
    [groupName, subjectName, teacherName],
  );

  const loadTerms = useCallback((cycleId: string): Promise<Term[]> => listTerms(API_URL, cycleId), []);
  const loadRoster = useCallback(
    (assignmentId: string, termId: string): Promise<RosterRow[]> => getRoster(API_URL, { assignmentId, termId }),
    [],
  );
  const onCapture = useCallback(
    async (assignmentId: string, enrollmentId: string, termId: string, value: number): Promise<void> => {
      // subject/teacher are DERIVED server-side from the assignment — never sent.
      await captureGrade(API_URL, { assignmentId, enrollmentId, termId, value });
    },
    [],
  );

  return (
    <Page>
      <button
        onClick={onBack}
        className="fade-up"
        style={{ display: "inline-flex", alignItems: "center", gap: 7, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 14 }}
      >
        <Icon name="back" size={18} stroke={2.4} />
        Volver al inicio
      </button>

      <div className="fade-up" style={{ display: "flex", alignItems: "center", gap: 16, marginBottom: 24 }}>
        <div style={{ width: 56, height: 56, borderRadius: 18, display: "grid", placeItems: "center", background: "var(--surface-3)", color: "var(--text-muted)" }}>
          <Icon name="grades" size={30} />
        </div>
        <div>
          <h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>Calificaciones</h1>
          <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>Captura y edita la calificación de cada alumno por asignación y periodo</p>
        </div>
      </div>

      {loading ? (
        <p className="fade-up" style={{ color: "var(--text-muted)", fontSize: 15 }}>Cargando…</p>
      ) : loadError !== undefined ? (
        <p role="alert" className="fade-up" style={{ color: "var(--bad-fg)", fontSize: 15 }}>{loadError}</p>
      ) : (
        <div className="fade-up">
          <CapturePanel
            assignments={assignments}
            assignmentLabel={assignmentLabel}
            loadTerms={loadTerms}
            loadRoster={loadRoster}
            studentName={studentName}
            onCapture={onCapture}
          />
        </div>
      )}
    </Page>
  );
}
