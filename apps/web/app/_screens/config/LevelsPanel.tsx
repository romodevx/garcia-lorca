"use client";

import { useState, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import { SlideOver } from "../../_design/SlideOver";
import type { CreateLevelInput, EnrollmentGranularity, Level } from "../../config/api";
import { TextField } from "../../_design/form";
import { EmptyHint, RadioGroup, Section, SelectChip } from "./parts";

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
 * The school's levels: maternal, kínder, primaria…
 *
 * Choosing one and creating one are different acts, and the screen used to do
 * both at once — a list of chips with a form permanently open underneath it.
 * Now the panel is the list, and creating opens the same drawer a new pupil
 * does.
 */
export function LevelsPanel({ levels, selectedLevelId, onSelect, onCreate }: LevelsPanelProps): ReactElement {
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  const [sepCode, setSepCode] = useState("");
  const [granularity, setGranularity] = useState("cycle");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  const submit = async (): Promise<void> => {
    if (name.trim() === "" || sepCode.trim() === "") {
      setError("Un nivel necesita nombre y clave SEP.");
      return;
    }
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
      setGranularity("cycle");
      setCreating(false);
    } catch {
      setError("No se pudo crear el nivel. Revisa que no exista ya con ese nombre.");
    } finally {
      setPending(false);
    }
  };

  return (
    <Section title="Niveles educativos" subtitle="Elige uno para ver sus grados, o agrega otro.">
      {levels.length > 0 ? (
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap", marginBottom: 16 }}>
          {levels.map((l) => (
            <SelectChip key={l.id} selected={l.id === selectedLevelId} onClick={() => onSelect(l.id)}>
              {l.name} · {GRANULARITY_LABEL[l.enrollmentGranularity] ?? l.enrollmentGranularity}
            </SelectChip>
          ))}
        </div>
      ) : (
        <EmptyHint>Aún no hay niveles. Agrega el primero.</EmptyHint>
      )}

      <Btn icon="plus" onClick={() => { setError(undefined); setCreating(true); }}>Nuevo nivel</Btn>

      {creating && (
        <SlideOver
          title="Nuevo nivel educativo"
          subtitle="Maternal, kínder, primaria…"
          icon="students"
          submitLabel="Crear nivel"
          pending={pending}
          error={error}
          onClose={() => { setCreating(false); setError(undefined); }}
          onSubmit={() => void submit()}
        >
          <TextField label="Nombre" value={name} onChange={setName} uppercase required placeholder="PRIMARIA" />
          <TextField label="Clave SEP" value={sepCode} onChange={setSepCode} uppercase placeholder="PRI" />
          <RadioGroup legend="Inscripción" options={GRANULARITY_OPTIONS} value={granularity} onChange={setGranularity} />
          <div style={{ display: "flex", gap: 11, padding: "14px 16px", borderRadius: "var(--r-md)", background: "var(--surface-3)", border: "1px solid var(--border)" }}>
            <p style={{ fontSize: 13.5, color: "var(--text-muted)", lineHeight: 1.5, margin: 0 }}>
              <strong>Por ciclo</strong> es lo normal: el alumno se inscribe una vez al año.
              <strong> Por periodo</strong> es para niveles que se cursan por cuatrimestre o semestre.
            </p>
          </div>
        </SlideOver>
      )}
    </Section>
  );
}
