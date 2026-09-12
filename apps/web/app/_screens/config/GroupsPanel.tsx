"use client";

import { useEffect, useState, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import { SlideOver } from "../../_design/SlideOver";
import type { CreateGroupInput, Cycle, GradeLevel, Group } from "../../config/api";
import { NumberField, TextField } from "../../_design/form";
import { EmptyHint, Section } from "./parts";

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

/**
 * The classrooms of a grade in one cycle: A, B, C…
 *
 * The next letter and the usual capacity come filled in, because opening a
 * second group is one of those things a school does twenty times in an
 * afternoon and then not again until next August.
 */
export function GroupsPanel({ cycle, gradeLevel, groups, onCreate }: GroupsPanelProps): ReactElement {
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState(nextLetter(groups));
  const [capacity, setCapacity] = useState(String(DEFAULT_CAPACITY));
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  // The suggested letter follows the list, so it is never stale after a save or
  // after switching grade.
  useEffect(() => { setName(nextLetter(groups)); }, [groups]);

  const submit = async (): Promise<void> => {
    if (name.trim() === "") {
      setError("Escribe el nombre del grupo.");
      return;
    }
    setPending(true);
    setError(undefined);
    try {
      const capacityNumero = Number(capacity);
      if (capacity.trim() === "" || !Number.isFinite(capacityNumero)) {
        // Number("") es 0, y un 0 que nadie escribió es un dato falso: un grupo
        // con cupo 0 no acepta alumnos y la asignación automática lo salta.
        setError("El cupo del grupo es obligatorio.");
        return;
      }
      await onCreate({ cycleId: cycle.id, gradeLevelId: gradeLevel.id, name: name.trim(), capacity: capacityNumero });
      setCreating(false);
    } catch {
      setError("No se pudo crear el grupo. Revisa que no exista ya con ese nombre.");
    } finally {
      setPending(false);
    }
  };

  return (
    <Section title="Grupos" subtitle={`De ${gradeLevel.name} en ${cycle.name}.`}>
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

      <Btn icon="plus" onClick={() => { setError(undefined); setCreating(true); }}>Nuevo grupo</Btn>

      {creating && (
        <SlideOver
          title={`Nuevo grupo de ${gradeLevel.name}`}
          subtitle={`Ciclo ${cycle.name}`}
          icon="students"
          submitLabel="Crear grupo"
          pending={pending}
          error={error}
          onClose={() => { setCreating(false); setError(undefined); }}
          onSubmit={() => void submit()}
        >
          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 14 }}>
            <TextField label="Nombre del grupo" value={name} onChange={setName} uppercase required placeholder="A" />
            <NumberField label="Cupo" value={capacity} onChange={setCapacity} required min={0} />
          </div>
          <div style={{ display: "flex", gap: 11, padding: "14px 16px", borderRadius: "var(--r-md)", background: "var(--surface-3)", border: "1px solid var(--border)" }}>
            <p style={{ fontSize: 13.5, color: "var(--text-muted)", lineHeight: 1.5, margin: 0 }}>
              El <strong>cupo</strong> es lo que decide a qué grupo entra un alumno nuevo: se llena
              A y el siguiente pasa a B, sin que nadie lo asigne a mano.
            </p>
          </div>
        </SlideOver>
      )}
    </Section>
  );
}
