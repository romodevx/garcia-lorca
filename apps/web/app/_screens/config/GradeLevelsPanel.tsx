"use client";

import { useEffect, useState, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import { SlideOver } from "../../_design/SlideOver";
import type { CreateGradeLevelInput, GradeLevel, Level } from "../../config/api";
import { NumberField, TextField } from "../../_design/form";
import { EmptyHint, Section, SelectChip } from "./parts";

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

/**
 * The grades inside a level — 1° primaria, 2° primaria…
 *
 * Picking a grade and adding one are separate acts now: the panel lists, the
 * drawer creates. The order is suggested from what is already there, so the
 * common case is type a name and save.
 */
export function GradeLevelsPanel({
  level,
  gradeLevels,
  selectedGradeLevelId,
  onSelect,
  onCreate,
}: GradeLevelsPanelProps): ReactElement {
  const suggestion = nextOrdinal(gradeLevels);
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  // Texto, no número: un campo de texto contiene texto, y Number("") es 0 —
  // un orden 0 que nadie escribió.
  const [ordinal, setOrdinal] = useState(String(suggestion));
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  // The suggestion follows the list: adding a grade elsewhere, or switching
  // level, must not leave a stale number sitting in the form.
  useEffect(() => { setOrdinal(String(nextOrdinal(gradeLevels))); }, [gradeLevels]);

  const submit = async (): Promise<void> => {
    if (name.trim() === "") {
      setError("Escribe el nombre del grado.");
      return;
    }
    setPending(true);
    setError(undefined);
    try {
      const ordinalNumero = Number(ordinal);
      if (ordinal.trim() === "" || !Number.isFinite(ordinalNumero)) {
        // Number("") es 0, y un 0 que nadie escribió es un dato falso: un grupo
        // con cupo 0 no acepta alumnos y la asignación automática lo salta.
        setError("El orden del grado es obligatorio.");
        return;
      }
      await onCreate({ levelId: level.id, name: name.trim(), ordinal: ordinalNumero });
      setName("");
      setCreating(false);
    } catch {
      setError("No se pudo crear el grado. Revisa que no exista ya con ese orden.");
    } finally {
      setPending(false);
    }
  };

  return (
    <Section title={`Grados de ${level.name}`} subtitle="Elige uno para sus materias y grupos, o agrega otro.">
      {gradeLevels.length > 0 ? (
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap", marginBottom: 16 }}>
          {gradeLevels.map((g) => (
            <SelectChip key={g.id} selected={g.id === selectedGradeLevelId} onClick={() => onSelect(g.id)}>
              {g.ordinal}. {g.name}
            </SelectChip>
          ))}
        </div>
      ) : (
        <EmptyHint>Aún no hay grados en este nivel.</EmptyHint>
      )}

      <Btn icon="plus" onClick={() => { setError(undefined); setCreating(true); }}>Nuevo grado</Btn>

      {creating && (
        <SlideOver
          title={`Nuevo grado en ${level.name}`}
          subtitle="1° primaria, 2° primaria…"
          icon="grades"
          submitLabel="Crear grado"
          pending={pending}
          error={error}
          onClose={() => { setCreating(false); setError(undefined); }}
          onSubmit={() => void submit()}
        >
          <TextField label="Nombre del grado" value={name} onChange={setName} uppercase required placeholder="1° PRIMARIA" />
          <NumberField label="Orden" value={ordinal} onChange={setOrdinal} required min={1} />
          <div style={{ display: "flex", gap: 11, padding: "14px 16px", borderRadius: "var(--r-md)", background: "var(--surface-3)", border: "1px solid var(--border)" }}>
            <p style={{ fontSize: 13.5, color: "var(--text-muted)", lineHeight: 1.5, margin: 0 }}>
              El <strong>orden</strong> es el que verán las listas y las boletas. Ya viene sugerido
              el siguiente; cámbialo sólo si estás intercalando un grado.
            </p>
          </div>
        </SlideOver>
      )}
    </Section>
  );
}
