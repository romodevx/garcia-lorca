"use client";

import { useEffect, useRef, type CSSProperties, type ReactElement } from "react";

import { Field, TextField, inputStyle } from "../../../_design/form";
import { Stepper as SharedStepper, type StepDef } from "../../../_design/Stepper";
import type { LevelDeletionPreview } from "../../../config/api";
import { deletedLines, keptLines } from "./copy";
import type { Step } from "./useLevelDeletion";

const STEPS: readonly StepDef<Step>[] = [
  { id: 1, label: "Resumen" },
  { id: 2, label: "Confirmar nombre" },
  { id: 3, label: "Contraseña" },
];

const listStyle: CSSProperties = { margin: "8px 0 0", paddingLeft: 20, display: "grid", gap: 4, fontSize: 15, color: "var(--text)", overflowWrap: "anywhere" };
const listTitle: CSSProperties = { fontSize: 13, fontWeight: 700, textTransform: "uppercase", letterSpacing: ".04em", color: "var(--text-muted)" };

/** "1 Resumen · 2 Confirmar nombre · 3 Contraseña", in red: this flow destroys. */
export function Stepper({ step }: { step: Step }): ReactElement {
  return <SharedStepper steps={STEPS} current={step} tone="red" />;
}

/** Step 1: what goes, what stays, and that it can come back for 7 days. */
export function SummaryStep({ preview }: { preview: LevelDeletionPreview }): ReactElement {
  const goes = deletedLines(preview.counts);
  const stays = keptLines(preview.counts, preview.keeps);
  return (
    <div style={{ display: "grid", gap: 16 }}>
      <p style={{ fontSize: 16, fontWeight: 600, color: "var(--text)", overflowWrap: "anywhere" }}>
        {preview.empty
          ? "Este nivel no tiene grados ni alumnos: se eliminará por completo."
          : `Se eliminará ${preview.levelName} y todo lo académico que depende de él.`}
      </p>
      {goes.length > 0 && (
        <div>
          <p id="deletion-goes" style={listTitle}>Se eliminará</p>
          <ul aria-labelledby="deletion-goes" style={listStyle}>{goes.map((l) => <li key={l}>{l}</li>)}</ul>
        </div>
      )}
      {stays.length > 0 && (
        <div>
          <p id="deletion-stays" style={listTitle}>Se conserva</p>
          <ul aria-labelledby="deletion-stays" style={listStyle}>{stays.map((l) => <li key={l}>{l}</li>)}</ul>
        </div>
      )}
      <p style={{ fontSize: 14, color: "var(--text-muted)" }}>Podrás restaurarlo durante 7 días desde la Papelera.</p>
    </div>
  );
}

export interface NameStepProps {
  levelName: string;
  value: string;
  error: string | undefined;
  onChange: (v: string) => void;
  onBlur: () => void;
}

/** Step 2: typing the name proves the director read WHICH level goes. */
export function NameStep({ levelName, value, error, onChange, onBlur }: NameStepProps): ReactElement {
  return (
    <TextField
      label={`Escribe el nombre del nivel para confirmar: ${levelName}`}
      value={value}
      onChange={onChange}
      onBlur={() => value.trim() !== "" && onBlur()}
      error={error}
      required
    />
  );
}

export interface PasswordStepProps {
  value: string;
  error: string | undefined;
  onChange: (v: string) => void;
}

/** Step 3: the director's own password, again. A wrong one refocuses the emptied field. */
export function PasswordStep({ value, error, onChange }: PasswordStepProps): ReactElement {
  const input = useRef<HTMLInputElement | null>(null);
  useEffect(() => {
    if (error !== undefined) input.current?.focus();
  }, [error]);
  return (
    <Field label="Tu contraseña de director" required error={error}>
      {(id, describedBy) => (
        <input
          ref={input}
          id={id}
          type="password"
          autoComplete="current-password"
          aria-describedby={describedBy}
          aria-invalid={error !== undefined ? true : undefined}
          aria-required
          value={value}
          onChange={(e) => onChange(e.target.value)}
          style={inputStyle(error !== undefined)}
        />
      )}
    </Field>
  );
}
