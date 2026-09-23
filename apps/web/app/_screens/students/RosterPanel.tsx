"use client";

import { useEffect, useState, type ReactElement } from "react";
import type { Cycle, Group } from "../../config/api";
import type { Enrollment, Student, TransferInput } from "../../students/api";
import { SelectField, type SelectOption } from "../../_design/form";
import { EmptyHint, ErrorAlert, Section } from "../config/parts";
import { EnrollmentRow } from "./EnrollmentRow";

export interface RosterPanelProps {
  students: readonly Student[];
  cycles: readonly Cycle[];
  loadGroups: (cycleId: string) => Promise<Group[]>;
  loadEnrollments: (groupId: string) => Promise<Enrollment[]>;
  onTransfer: (input: TransferInput) => Promise<void>;
  onWithdraw: (enrollmentId: string) => Promise<void>;
}

/**
 * A group's roster: pick a cycle (active auto-selected) and one of its groups to
 * see who is enrolled, resolving each student's name by id from the loaded list.
 * Each active row carries a transfer (same-cycle group + reason) and a withdraw
 * action; both refresh the roster so the view stays truthful. The gateway is the
 * security boundary — this panel is UX only.
 */
export function RosterPanel({ students, cycles, loadGroups, loadEnrollments, onTransfer, onWithdraw }: RosterPanelProps): ReactElement {
  const [cycleId, setCycleId] = useState("");
  const [groups, setGroups] = useState<Group[]>([]);
  const [groupId, setGroupId] = useState("");
  const [enrollments, setEnrollments] = useState<Enrollment[]>([]);
  const [error, setError] = useState<string | undefined>(undefined);

  // Auto-select a cycle (active, else first).
  useEffect(() => {
    if (cycleId !== "" || cycles.length === 0) return;
    const preferred = cycles.find((c) => c.status === "active") ?? cycles[0];
    if (preferred !== undefined) setCycleId(preferred.id);
  }, [cycles, cycleId]);

  // Cycle → its groups; reset the group + roster downstream.
  useEffect(() => {
    if (cycleId === "") return;
    let active = true;
    setGroupId("");
    setEnrollments([]);
    loadGroups(cycleId)
      .then((g) => active && setGroups(g))
      .catch(() => {});
    return () => {
      active = false;
    };
  }, [cycleId, loadGroups]);

  // Group → its roster.
  useEffect(() => {
    if (groupId === "") return;
    let active = true;
    loadEnrollments(groupId)
      .then((e) => active && setEnrollments(e))
      .catch(() => active && setError("No se pudo cargar el roster."));
    return () => {
      active = false;
    };
  }, [groupId, loadEnrollments]);

  const refresh = async (): Promise<void> => {
    if (groupId === "") return;
    setEnrollments(await loadEnrollments(groupId));
  };

  const handleTransfer = async (input: TransferInput): Promise<void> => {
    setError(undefined);
    try {
      await onTransfer(input);
      await refresh();
    } catch {
      setError("No se pudo trasladar al alumno. Intenta de nuevo.");
    }
  };

  const handleWithdraw = async (enrollmentId: string): Promise<void> => {
    setError(undefined);
    try {
      await onWithdraw(enrollmentId);
      await refresh();
    } catch {
      setError("No se pudo dar de baja al alumno. Intenta de nuevo.");
    }
  };

  const studentName = (id: string): string => {
    const s = students.find((x) => x.id === id);
    return s !== undefined ? `${s.firstName} ${s.lastName}` : id;
  };

  const cycleOptions: SelectOption[] = cycles.map((c) => ({ value: c.id, label: c.name }));
  const groupOptions: SelectOption[] = groups.map((g) => ({ value: g.id, label: g.name }));

  return (
    <Section title="Roster del grupo" subtitle="Consulta a los alumnos inscritos, trasládalos o da de baja.">
      <form aria-label="Ver roster del grupo">
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12, marginBottom: 16 }}>
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
      </form>

      {groupId === "" ? (
        <EmptyHint>Elige un grupo para ver a sus alumnos inscritos.</EmptyHint>
      ) : enrollments.length === 0 ? (
        <EmptyHint>Este grupo aún no tiene alumnos inscritos.</EmptyHint>
      ) : (
        <ul aria-label="Alumnos inscritos" style={{ listStyle: "none", padding: 0, margin: 0, display: "grid", gap: 10 }}>
          {enrollments.map((e) => (
            <EnrollmentRow
              key={e.id}
              enrollment={e}
              studentName={studentName(e.studentId)}
              groups={groups}
              onTransfer={handleTransfer}
              onWithdraw={handleWithdraw}
            />
          ))}
        </ul>
      )}
      {error !== undefined && <ErrorAlert message={error} />}
    </Section>
  );
}
