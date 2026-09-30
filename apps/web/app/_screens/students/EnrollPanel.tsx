"use client";

import { useEffect, useState, type FormEvent, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import type { Cycle, Group } from "../../config/api";
import type { EnrollInput, Student } from "../../students/api";
import { SelectField, type SelectOption } from "../../_design/form";
import { EmptyHint, ErrorAlert, Section } from "../config/parts";
import { formatStudentName, sortStudents } from "../../_design/students/studentName";

export interface EnrollPanelProps {
  students: readonly Student[];
  cycles: readonly Cycle[];
  /** Groups of the chosen cycle. */
  loadGroups: (cycleId: string) => Promise<Group[]>;
  onEnroll: (input: EnrollInput) => Promise<void>;
}

/**
 * Place a student into a group — the enroll knot, rendered so drift is
 * impossible: cycleId is NEVER entered (the server derives it from the group),
 * and the group dropdown is scoped to the chosen cycle. The active cycle is
 * auto-selected so the flow starts one click ahead. The gateway enforces the same
 * invariants; this panel is UX only.
 */
export function EnrollPanel({ students, cycles, loadGroups, onEnroll }: EnrollPanelProps): ReactElement {
  const [studentId, setStudentId] = useState("");
  const [cycleId, setCycleId] = useState("");
  const [groups, setGroups] = useState<Group[]>([]);
  const [groupId, setGroupId] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  // Auto-select a cycle (active, else first) so the flow starts one click ahead.
  useEffect(() => {
    if (cycleId !== "" || cycles.length === 0) return;
    const preferred = cycles.find((c) => c.status === "active") ?? cycles[0];
    if (preferred !== undefined) setCycleId(preferred.id);
  }, [cycles, cycleId]);

  // Cycle → its groups; reset the group selection downstream.
  useEffect(() => {
    if (cycleId === "") return;
    let active = true;
    setGroupId("");
    loadGroups(cycleId)
      .then((g) => active && setGroups(g))
      .catch(() => {});
    return () => {
      active = false;
    };
  }, [cycleId, loadGroups]);

  const canSubmit = studentId !== "" && groupId !== "" && !pending;

  const submit = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    setPending(true);
    setError(undefined);
    try {
      await onEnroll({ studentId, groupId });
      setStudentId("");
      setGroupId("");
    } catch {
      setError("No se pudo inscribir al alumno. Intenta de nuevo.");
    } finally {
      setPending(false);
    }
  };

  if (students.length === 0) {
    return (
      <Section title="Inscribir alumno" subtitle="Coloca a un alumno en un grupo del ciclo.">
        <EmptyHint>Registra alumnos en Admisión para poder inscribirlos.</EmptyHint>
      </Section>
    );
  }

  const studentOptions: SelectOption[] = sortStudents(students).map((s) => ({ value: s.id, label: formatStudentName(s) }));
  const cycleOptions: SelectOption[] = cycles.map((c) => ({ value: c.id, label: c.name }));
  const groupOptions: SelectOption[] = groups.map((g) => ({ value: g.id, label: g.name }));

  return (
    <Section title="Inscribir alumno" subtitle="Elige alumno, ciclo y grupo. El ciclo se deriva del grupo en el servidor.">
      <form aria-label="Inscribir alumno" onSubmit={submit}>
        <div style={{ display: "grid", gap: 12, marginBottom: 12 }}>
          <SelectField label="Alumno" value={studentId} options={studentOptions} onChange={setStudentId} help="El alumno que vas a inscribir." />
          <SelectField label="Ciclo" value={cycleId} options={cycleOptions} onChange={setCycleId} help="El ciclo escolar sobre el que trabajas." disabled={cycles.length === 0} />
          <SelectField
            label="Grupo"
            value={groupId}
            options={groupOptions}
            onChange={setGroupId}
            help="El salón. Sólo muestra los grupos del ciclo elegido."
            disabled={cycleId === "" || groups.length === 0}
          />
        </div>
        <Btn type="submit" icon="plus" disabled={!canSubmit}>
          Inscribir alumno
        </Btn>
      </form>
      {error !== undefined && <ErrorAlert message={error} />}
    </Section>
  );
}
