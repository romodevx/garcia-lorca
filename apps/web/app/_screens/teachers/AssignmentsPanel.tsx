"use client";

import { useEffect, useRef, useState, type FormEvent, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import type { Cycle, LabelledGroup, Subject } from "../../config/api";
import type {
  CreateTeachingAssignmentInput,
  Teacher,
  TeachingAssignment,
} from "../../teachers/api";
import { SelectField, useRequired, type SelectOption } from "../../_design/form";
import { EmptyHint, ErrorAlert, Section } from "../config/parts";

/** What an assignment needs, in the order the form shows it (the first missing takes the focus). */
const REQUIRED = ["cycleId", "groupId", "subjectId", "teacherId"] as const;

export interface AssignmentsPanelProps {
  cycles: readonly Cycle[];
  teachers: readonly Teacher[];
  /** Groups of the chosen cycle. */
  loadGroups: (cycleId: string) => Promise<LabelledGroup[]>;
  /** Subjects teachable for a grade level — scopes the subject dropdown. */
  loadSubjects: (gradeLevelId: string) => Promise<Subject[]>;
  /** Current assignments of a group (teacher per subject). */
  loadAssignments: (groupId: string) => Promise<TeachingAssignment[]>;
  onCreate: (input: CreateTeachingAssignmentInput) => Promise<void>;
}

/**
 * Assign a teacher to a group+subject — the knot, rendered so a mismatch is
 * impossible: once a group is chosen the subject dropdown is scoped to THAT
 * group's grade level (only teachable subjects appear), and cycleId is never
 * entered — the server derives it from the group. The gateway enforces the same
 * invariants; this screen is UX only. "Asignar profesor" is never disabled: a
 * submit with something missing marks every missing field and focuses the first.
 */
export function AssignmentsPanel({
  cycles,
  teachers,
  loadGroups,
  loadSubjects,
  loadAssignments,
  onCreate,
}: AssignmentsPanelProps): ReactElement {
  const [cycleId, setCycleId] = useState("");
  const [groups, setGroups] = useState<LabelledGroup[]>([]);
  const [groupId, setGroupId] = useState("");
  const [subjects, setSubjects] = useState<Subject[]>([]);
  const [subjectId, setSubjectId] = useState("");
  const [teacherId, setTeacherId] = useState("");
  const [assignments, setAssignments] = useState<TeachingAssignment[]>([]);
  const pending = useRef(false);
  const [error, setError] = useState<string | undefined>(undefined);
  const required = useRequired(REQUIRED);

  // Auto-select a cycle (active, else first) so the flow starts one click ahead.
  useEffect(() => {
    if (cycleId !== "" || cycles.length === 0) return;
    const preferred = cycles.find((c) => c.status === "active") ?? cycles[0];
    if (preferred !== undefined) setCycleId(preferred.id);
  }, [cycles, cycleId]);

  // Cycle → its groups; reset everything downstream.
  useEffect(() => {
    if (cycleId === "") return;
    let active = true;
    setGroupId("");
    setSubjects([]);
    setSubjectId("");
    setAssignments([]);
    loadGroups(cycleId)
      .then((g) => active && setGroups(g))
      .catch(() => {});
    return () => {
      active = false;
    };
  }, [cycleId, loadGroups]);

  // Group → subjects SCOPED to the group's grade level + the group's assignments.
  useEffect(() => {
    if (groupId === "") return;
    const group = groups.find((g) => g.id === groupId);
    if (group === undefined) return;
    let active = true;
    setSubjectId("");
    loadSubjects(group.gradeLevelId)
      .then((s) => active && setSubjects(s))
      .catch(() => {});
    loadAssignments(groupId)
      .then((a) => active && setAssignments(a))
      .catch(() => {});
    return () => {
      active = false;
    };
  }, [groupId, groups, loadSubjects, loadAssignments]);

  const choose = (key: (typeof REQUIRED)[number], set: (value: string) => void) => (value: string): void => {
    set(value);
    if (value !== "") required.clear(key);
  };

  const submit = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    if (pending.current) return;
    if (!required.check({ cycleId, groupId, subjectId, teacherId })) return;
    pending.current = true;
    setError(undefined);
    try {
      await onCreate({ groupId, subjectId, teacherId });
      setSubjectId("");
      setTeacherId("");
      const refreshed = await loadAssignments(groupId);
      setAssignments(refreshed);
    } catch {
      setError("No se pudo asignar al profesor. Intenta de nuevo.");
    } finally {
      pending.current = false;
    }
  };

  if (cycles.length === 0) {
    return (
      <Section title="Asignaciones docentes" subtitle="Profesor por grupo y materia.">
        <EmptyHint>Crea un ciclo y sus grupos para poder asignar profesores.</EmptyHint>
      </Section>
    );
  }

  const cycleOptions: SelectOption[] = cycles.map((c) => ({ value: c.id, label: c.name }));
  const groupOptions: SelectOption[] = groups.map((g) => ({ value: g.id, label: g.label }));
  const subjectOptions: SelectOption[] = subjects.map((s) => ({ value: s.id, label: s.name }));
  const teacherOptions: SelectOption[] = teachers.map((t) => ({ value: t.id, label: `${t.firstName} ${t.lastName}` }));

  const subjectName = (id: string): string => subjects.find((s) => s.id === id)?.name ?? id;
  const teacherName = (id: string): string => {
    const t = teachers.find((x) => x.id === id);
    return t !== undefined ? `${t.firstName} ${t.lastName}` : id;
  };

  return (
    <Section title="Asignaciones docentes" subtitle="Elige ciclo, grupo, materia y profesor. Las materias se limitan al grado del grupo.">
      {groupId !== "" &&
        (assignments.length > 0 ? (
          <ul style={{ listStyle: "none", padding: 0, margin: "0 0 16px", display: "grid", gap: 6 }}>
            {assignments.map((a) => (
              <li
                key={a.id}
                style={{ padding: "8px 12px", borderRadius: "var(--r-md)", background: "var(--surface-2)", fontSize: 14 }}
              >
                <span style={{ fontWeight: 600 }}>{subjectName(a.subjectId)}</span>{" "}
                <span style={{ color: "var(--text-muted)" }}>· {teacherName(a.teacherId)}</span>
              </li>
            ))}
          </ul>
        ) : (
          <EmptyHint>Este grupo aún no tiene profesores asignados.</EmptyHint>
        ))}

      <form aria-label="Asignar profesor" onSubmit={submit}>
        <div style={{ display: "grid", gap: 12, marginBottom: 12 }}>
          <SelectField
            label="Ciclo"
            value={cycleId}
            options={cycleOptions}
            onChange={choose("cycleId", setCycleId)}
            error={required.errors.cycleId}
            // Sin esta opción, cuando no hay ciclo activo el valor queda en ""
            // y el <select> se pinta en blanco con selectedIndex -1: parece
            // roto. El SelectField viejo siempre traía una opción vacía; el
            // nuevo la trae sólo si se le da un texto.
            help="El ciclo escolar al que pertenece esta asignación."
          />
          <SelectField
            label="Grupo"
            value={groupId}
            options={groupOptions}
            onChange={choose("groupId", setGroupId)}
            error={required.errors.groupId}
            help="El salón que le toca. La lista sale del ciclo elegido."
          />
          <SelectField
            label="Materia"
            value={subjectId}
            options={subjectOptions}
            onChange={choose("subjectId", setSubjectId)}
            error={required.errors.subjectId}
            help="La materia que va a impartir en ese grupo. La lista sale del grado del grupo."
          />
          <SelectField
            label="Profesor"
            value={teacherId}
            options={teacherOptions}
            onChange={choose("teacherId", setTeacherId)}
            error={required.errors.teacherId}
            help="Quién la imparte. Sólo aparecen los docentes dados de alta."
          />
        </div>
        <Btn type="submit" icon="plus">
          Asignar profesor
        </Btn>
      </form>
      {error !== undefined && <ErrorAlert message={error} />}
    </Section>
  );
}
