"use client";

import { useState, type FormEvent, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import type { CreateLevelInput, EnrollmentGranularity, Level } from "../../config/api";
import { EmptyHint, ErrorAlert, RadioGroup, Section, SelectChip, TextField } from "./parts";

export interface LevelsPanelProps {
  levels: readonly Level[];
  selectedLevelId: string | undefined;
  onSelect: (id: string) => void;
  onCreate: (input: CreateLevelInput) => Promise<void>;
}

const GRANULARITY_OPTIONS = [
  { value: "cycle", label: "Por ciclo" },
  { value: "term", label: "Por periodo" },
] as const;

const GRANULARITY_LABEL: Record<string, string> = { cycle: "por ciclo", term: "por periodo" };

/**
 * Levels catalog (cycle-independent). Granularity is a required, defaultless
 * choice, so the submit stays disabled until one is picked.
 */
export function LevelsPanel({ levels, selectedLevelId, onSelect, onCreate }: LevelsPanelProps): ReactElement {
  const [name, setName] = useState("");
  const [sepCode, setSepCode] = useState("");
  const [granularity, setGranularity] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  const canSubmit = name.trim() !== "" && sepCode.trim() !== "" && granularity !== "" && !pending;

  const submit = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    setPending(true);
    setError(undefined);
    try {
      await onCreate({
        name: name.trim(),
        sepCode: sepCode.trim(),
        enrollmentGranularity: granularity as EnrollmentGranularity,
      });
      setName("");
      setSepCode("");
      setGranularity("");
    } catch {
      setError("No se pudo crear el nivel. Intenta de nuevo.");
    } finally {
      setPending(false);
    }
  };

  return (
    <Section title="Niveles educativos" subtitle="Kínder, primaria, secundaria… con su clave SEP.">
      {levels.length > 0 ? (
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap", marginBottom: 18 }}>
          {levels.map((l) => (
            <SelectChip key={l.id} selected={l.id === selectedLevelId} onClick={() => onSelect(l.id)}>
              {l.name} · {GRANULARITY_LABEL[l.enrollmentGranularity] ?? l.enrollmentGranularity}
            </SelectChip>
          ))}
        </div>
      ) : (
        <EmptyHint>Aún no hay niveles. Agrega el primero.</EmptyHint>
      )}

      <form aria-label="Agregar nivel" onSubmit={submit} style={{ display: "grid", gap: 12 }}>
        <div style={{ display: "grid", gridTemplateColumns: "1.4fr 1fr", gap: 12 }}>
          <TextField label="Nombre del nivel" value={name} onChange={setName} placeholder="Primaria" />
          <TextField label="Clave SEP" value={sepCode} onChange={setSepCode} placeholder="PRI" />
        </div>
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "end", gap: 12, flexWrap: "wrap" }}>
          <RadioGroup legend="Inscripción" options={GRANULARITY_OPTIONS} value={granularity} onChange={setGranularity} />
          <Btn type="submit" icon="plus" disabled={!canSubmit}>
            Crear nivel
          </Btn>
        </div>
      </form>
      {error !== undefined && <ErrorAlert message={error} />}
    </Section>
  );
}
