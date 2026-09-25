"use client";

import { useCallback, useEffect, useState, type ReactElement } from "react";

import { Icon } from "../../_design/Icon";
import { Page } from "../../_design/Page";
import {
  listCycles,
  listLabelledGroups,
  listSubjects,
  type Cycle,
  type LabelledGroup,
  type Subject,
} from "../../config/api";
import {
  createTeacher,
  createTeachingAssignment,
  listTeachers,
  listTeachingAssignments,
  type CreateTeacherInput,
  type CreateTeachingAssignmentInput,
  type Teacher,
  type TeachingAssignment,
} from "../../teachers/api";
import { AssignmentsPanel } from "./AssignmentsPanel";
import { TeachersPanel } from "./TeachersPanel";

import { API_URL } from "../../api-url";


export interface TeachersScreenProps {
  onBack: () => void;
}

/**
 * Docentes: who teaches here, and what each of them teaches.
 *
 * It used to be two panels at the bottom of Configuración académica, below the
 * cycles and grades nobody edits twice a year. Staff is not catalogue — it turns
 * over, it is looked up, it is the thing a director opens on a Monday — so it is
 * a module of its own now, alongside Estudiantes.
 *
 * The two halves have different appetites. The roster stands alone: it needs
 * teachers:read and nothing else. Assigning needs the catalogue as well —
 * an assignment IS (group, subject, teacher) — so it reads cycles, groups and
 * subjects from Configuración, and an account without config:read gets a
 * sentence saying so rather than three empty dropdowns that read like a school
 * with no groups.
 */
export function TeachersScreen({ onBack }: TeachersScreenProps): ReactElement {
  const [teachers, setTeachers] = useState<Teacher[]>([]);
  const [cycles, setCycles] = useState<Cycle[]>([]);
  const [catalogueClosed, setCatalogueClosed] = useState(false);
  const [rosterError, setRosterError] = useState<string | undefined>(undefined);

  useEffect(() => {
    listTeachers(API_URL)
      .then(setTeachers)
      .catch(() => setRosterError("No pudimos cargar la plantilla docente."));
  }, []);

  // Separate from the roster on purpose: losing the catalogue must not take the
  // roster down with it, because they answer to different permissions.
  useEffect(() => {
    listCycles(API_URL)
      .then(setCycles)
      .catch(() => setCatalogueClosed(true));
  }, []);

  const handleCreateTeacher = useCallback(async (input: CreateTeacherInput): Promise<void> => {
    const created = await createTeacher(API_URL, input);
    setTeachers((prev) => [...prev, created]);
  }, []);

  const handleCreateAssignment = useCallback(
    async (input: CreateTeachingAssignmentInput): Promise<void> => {
      await createTeachingAssignment(API_URL, input);
    },
    [],
  );

  const loadGroups = useCallback((cycleId: string): Promise<LabelledGroup[]> => listLabelledGroups(API_URL, cycleId), []);
  const loadSubjects = useCallback(
    (gradeLevelId: string): Promise<Subject[]> => listSubjects(API_URL, gradeLevelId),
    [],
  );
  const loadAssignments = useCallback(
    (groupId: string): Promise<TeachingAssignment[]> => listTeachingAssignments(API_URL, { groupId }),
    [],
  );

  const linked = teachers.filter((t) => t.userId !== "").length;

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
        <div style={{ width: 56, height: 56, borderRadius: 18, display: "grid", placeItems: "center", background: "var(--purple)", color: "#fff", boxShadow: "0 6px 16px color-mix(in oklab, var(--purple) 38%, transparent)" }}>
          <Icon name="user" size={30} />
        </div>
        <div>
          <h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>Docentes</h1>
          <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>
            Plantilla del plantel y quién imparte cada materia
          </p>
        </div>
      </div>

      <div className="fade-up" style={{ display: "flex", flexWrap: "wrap", gap: 16, marginBottom: 24 }}>
        <StatCard icon="user" color="purple" label="Profesores" value={teachers.length} />
        <StatCard icon="shield" color="forest" label="Con acceso al sistema" value={linked} />
      </div>

      {rosterError !== undefined && <Note>{rosterError}</Note>}
      {catalogueClosed && (
        <Note>
          No tienes acceso al catálogo académico, así que no puedes asignar materias desde aquí.
          Pídele a la dirección el permiso de Configuración.
        </Note>
      )}

      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(340px, 1fr))", gap: 18 }}>
        <TeachersPanel teachers={teachers} onCreate={handleCreateTeacher} />
        {!catalogueClosed && (
          <AssignmentsPanel
            cycles={cycles}
            teachers={teachers}
            loadGroups={loadGroups}
            loadSubjects={loadSubjects}
            loadAssignments={loadAssignments}
            onCreate={handleCreateAssignment}
          />
        )}
      </div>
    </Page>
  );
}

function StatCard({
  icon,
  color,
  label,
  value,
}: {
  icon: "user" | "shield";
  color: "purple" | "forest";
  label: string;
  value: number;
}): ReactElement {
  return (
    <div style={{ flex: "1 1 200px", minWidth: 200, display: "flex", alignItems: "center", gap: 14, padding: "16px 18px", borderRadius: "var(--r-lg)", background: "var(--surface)", border: "1px solid var(--border)" }}>
      <div style={{ width: 42, height: 42, borderRadius: 13, display: "grid", placeItems: "center", background: `color-mix(in oklab, var(--${color}) 14%, var(--surface))`, color: `var(--${color})` }}>
        <Icon name={icon} size={21} />
      </div>
      <div>
        <div style={{ fontSize: 24, fontWeight: 800, fontFamily: "var(--font-display)", color: "var(--text)" }}>{value}</div>
        <div style={{ fontSize: 13, color: "var(--text-muted)" }}>{label}</div>
      </div>
    </div>
  );
}

function Note({ children }: { children: React.ReactNode }): ReactElement {
  return (
    <div
      role="status"
      style={{ marginBottom: 18, padding: "14px 16px", borderRadius: "var(--r-md)", background: "var(--warn-bg)", color: "var(--warn-fg)", fontSize: 14.5, lineHeight: 1.45 }}
    >
      {children}
    </div>
  );
}
