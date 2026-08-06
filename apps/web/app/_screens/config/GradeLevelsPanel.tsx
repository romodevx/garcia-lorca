"use client";

import { useState, type FormEvent, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import type { CreateGradeLevelInput, GradeLevel, Level } from "../../config/api";
import { EmptyHint, ErrorAlert, NumberField, Section, SelectChip, TextField } from "./parts";

export interface GradeLevelsPanelProps {
  level: Level;
  gradeLevels: readonly GradeLevel[];
  selectedGradeLevelId: string | undefined;
  onSelect: (id: string) => void;
  onCreate: (input: CreateGradeLevelInput) => Promise<void>;
}

/** Suggests the next ordinal so grades are numbered without manual bookkeeping. */
function nextOrdinal(gradeLevels: readonly GradeLevel[]): number {
  return gradeLevels.reduce((max, g) => Math.max(max, g.ordinal), 0) + 1;
}

/** Grades of the selected level, with an auto-incremented ordinal suggestion. */
export function GradeLevelsPanel({
  level,
  gradeLevels,
  selectedGradeLevelId,
  onSelect,
  onCreate,
}: GradeLevelsPanelProps): ReactElement {
  const suggestion = nextOrdinal(gradeLevels);
  const [name, setName] = useState("");
  const [ordinal, setOrdinal] = useState(suggestion);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  const canSubmit = name.trim() !== "" && ordinal >= 1 && !pending;

  const submit = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    setPending(true);
    setError(undefined);
    try {
      await onCreate({ levelId: level.id, name: name.trim(), ordinal });
      setName("");
      setOrdinal(ordinal + 1);
    } catch {
      setError("No se pudo crear el grado. Intenta de nuevo.");
    } finally {
      setPending(false);
    }
  };

  return (
    <Section title={`Grados de ${level.name}`} subtitle="Selecciona un grado para gestionar materias y grupos.">
      {gradeLevels.length > 0 ? (
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap", marginBottom: 18 }}>
          {gradeLevels.map((g) => (
            <SelectChip key={g.id} selected={g.id === selectedGradeLevelId} onClick={() => onSelect(g.id)}>
              {g.ordinal}. {g.name}
            </SelectChip>
          ))}
        </div>
      ) : (
        <EmptyHint>Aún no hay grados en este nivel.</EmptyHint>
      )}

      <form aria-label="Agregar grado" onSubmit={submit}>
        <div style={{ display: "grid", gridTemplateColumns: "1.6fr .8fr auto", gap: 12, alignItems: "end" }}>
          <TextField label="Nombre del grado" value={name} onChange={setName} placeholder="1° primaria" />
          <NumberField label="Orden" value={ordinal} onChange={setOrdinal} min={1} />
          <Btn type="submit" icon="plus" disabled={!canSubmit}>
            Crear grado
          </Btn>
        </div>
      </form>
      {error !== undefined && <ErrorAlert message={error} />}
    </Section>
  );
}
