"use client";

import { useState, type FormEvent, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import type { CreateGroupInput, Cycle, GradeLevel, Group } from "../../config/api";
import { EmptyHint, ErrorAlert, NumberField, Section, TextField } from "./parts";

export interface GroupsPanelProps {
  cycle: Cycle;
  gradeLevel: GradeLevel;
  groups: readonly Group[];
  onCreate: (input: CreateGroupInput) => Promise<void>;
}

const DEFAULT_CAPACITY = 30;

/** Suggests the next group letter (A, B, C…) from the count of existing groups. */
function nextLetter(groups: readonly Group[]): string {
  return String.fromCharCode("A".charCodeAt(0) + Math.min(groups.length, 25));
}

/** Groups for a (cycle, grade level), with an A/B/C… name suggestion. */
export function GroupsPanel({ cycle, gradeLevel, groups, onCreate }: GroupsPanelProps): ReactElement {
  const [name, setName] = useState(nextLetter(groups));
  const [capacity, setCapacity] = useState(DEFAULT_CAPACITY);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  const canSubmit = name.trim() !== "" && capacity >= 0 && !pending;

  const submit = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    setPending(true);
    setError(undefined);
    try {
      await onCreate({ cycleId: cycle.id, gradeLevelId: gradeLevel.id, name: name.trim(), capacity });
      setName(nextLetter([...groups, { id: "", cycleId: cycle.id, gradeLevelId: gradeLevel.id, name, capacity }]));
    } catch {
      setError("No se pudo crear el grupo. Intenta de nuevo.");
    } finally {
      setPending(false);
    }
  };

  return (
    <Section title="Grupos" subtitle={`Grupos de ${gradeLevel.name} en ${cycle.name}.`}>
      {groups.length > 0 ? (
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap", marginBottom: 16 }}>
          {groups.map((g) => (
            <span
              key={g.id}
              style={{ padding: "8px 14px", borderRadius: "var(--r-pill)", background: "var(--surface-2)", fontSize: 14, fontWeight: 600 }}
            >
              {g.name} · {g.capacity} lugares
            </span>
          ))}
        </div>
      ) : (
        <EmptyHint>Aún no hay grupos para este grado en el ciclo.</EmptyHint>
      )}

      <form aria-label="Agregar grupo" onSubmit={submit}>
        <div style={{ display: "grid", gridTemplateColumns: ".8fr .8fr auto", gap: 12, alignItems: "end" }}>
          <TextField label="Nombre del grupo" value={name} onChange={setName} placeholder="A" />
          <NumberField label="Cupo" value={capacity} onChange={setCapacity} min={0} />
          <Btn type="submit" icon="plus" disabled={!canSubmit}>
            Crear grupo
          </Btn>
        </div>
      </form>
      {error !== undefined && <ErrorAlert message={error} />}
    </Section>
  );
}
