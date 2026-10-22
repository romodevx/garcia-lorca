"use client";

import { useEffect, useRef, useState, type FormEvent, type KeyboardEvent, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { DateField, SelectField, TextField } from "../../_design/form";
import { MEXICAN_STATES } from "../../students/profileRules";
import type { ImportRow, OverrideName, RowDecision, RowFieldError, RowOverrides } from "../../studentImport/api";
import { rowErrorText } from "../../studentImport/copy";
import { changedOverrides, correctionError, editableFields, type EditableField } from "../../studentImport/review";

/** A pupil cannot lack these: clearing one is not a correction. */
const NEVER_EMPTY: readonly OverrideName[] = ["curp", "fullName"];
const MISSING = "Falta este dato.";
const STATE_OPTIONS = MEXICAN_STATES.map((s) => ({ value: s, label: s }));

export interface RowCorrectionProps {
  row: ImportRow;
  decision: RowDecision | undefined;
  /** The server's refusals for this row: shown on their field. */
  errors: readonly RowFieldError[];
  today: string;
  /** Only the values that changed; `{}` when nothing did. */
  onSave: (overrides: RowOverrides) => void;
  onCancel: () => void;
}

/**
 * The row's flagged values, edited where the row is (≤ 4 fields: the inline
 * container of the forms rule). Format is checked on blur once a value is
 * there, then on every keystroke while an error shows. Enter saves, Esc cancels.
 */
export function RowCorrection({ row, decision, errors, today, onSave, onCancel }: RowCorrectionProps): ReactElement {
  const fields = editableFields(row, decision);
  const [values, setValues] = useState<Partial<Record<OverrideName, string>>>(() => Object.fromEntries(fields.map((f) => [f.name, f.initial])));
  const [fieldErrors, setFieldErrors] = useState<Partial<Record<OverrideName, string>>>(() => serverErrors(errors));
  const form = useRef<HTMLFormElement>(null);

  useEffect(() => {
    form.current?.querySelector<HTMLElement>("input, select")?.focus();
  }, []);

  const check = (name: OverrideName, value: string): string | undefined => correctionError(name, value, today);
  const change = (name: OverrideName, value: string): void => {
    setValues((v) => ({ ...v, [name]: value }));
    if (fieldErrors[name] !== undefined) setFieldErrors((e) => ({ ...e, [name]: check(name, value) }));
  };
  const blur = (name: OverrideName): void => setFieldErrors((e) => ({ ...e, [name]: check(name, values[name] ?? "") }));

  const save = (e: FormEvent): void => {
    e.preventDefault();
    const overrides = changedOverrides(fields, values);
    const found: Partial<Record<OverrideName, string>> = {};
    for (const f of fields) {
      const value = (values[f.name] ?? "").trim();
      const cleared = value === "" && NEVER_EMPTY.includes(f.name) && f.name in overrides;
      const wrong = cleared ? MISSING : check(f.name, value);
      if (wrong !== undefined) found[f.name] = wrong;
    }
    setFieldErrors(found);
    if (Object.keys(found).length > 0) {
      form.current?.querySelector<HTMLElement>("[aria-invalid=true]")?.focus();
      return;
    }
    onSave(overrides);
  };

  const escape = (e: KeyboardEvent): void => {
    if (e.key !== "Escape") return;
    e.preventDefault();
    onCancel();
  };

  return (
    <form ref={form} aria-label={`Corregir fila ${row.row}`} onSubmit={save} onKeyDown={escape} noValidate style={{ display: "grid", gap: 12, maxWidth: 520 }}>
      {fields.map((f) => (
        <CorrectionField key={f.name} field={f} value={values[f.name] ?? ""} error={fieldErrors[f.name]} today={today} onChange={(v) => change(f.name, v)} onBlur={() => blur(f.name)} />
      ))}
      <div style={{ display: "flex", flexWrap: "wrap", gap: 10, alignItems: "center" }}>
        <Btn type="submit" icon="check" style={{ height: 44, fontSize: 14.5 }}>Guardar corrección</Btn>
        <button type="button" onClick={onCancel} style={{ minHeight: 44, padding: "0 12px", color: "var(--text-muted)", fontWeight: 600 }}>Cancelar</button>
      </div>
    </form>
  );
}

function serverErrors(errors: readonly RowFieldError[]): Partial<Record<OverrideName, string>> {
  return Object.fromEntries(errors.map((e) => [e.field, rowErrorText(e)]));
}

function CorrectionField({ field, value, error, today, onChange, onBlur }: {
  field: EditableField;
  value: string;
  error: string | undefined;
  today: string;
  onChange: (value: string) => void;
  onBlur: () => void;
}): ReactElement {
  if (field.kind === "date") return <DateField label={field.label} value={value} max={today} onChange={onChange} onBlur={onBlur} error={error} />;
  if (field.kind === "state") return <SelectField label={field.label} value={value} onChange={onChange} options={STATE_OPTIONS} error={error} />;
  const uppercase = field.name === "curp" || field.name === "fullName";
  const help = field.name === "fullName" ? "Como lo escribe el sistema anterior: APELLIDOS, NOMBRES." : undefined;
  return <TextField label={field.label} value={value} onChange={onChange} onBlur={onBlur} uppercase={uppercase} error={error} help={help} />;
}
