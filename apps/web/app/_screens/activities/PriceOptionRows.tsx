"use client";

import type { ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { MoneyField, SelectField } from "../../_design/form";
import { FLAT, MAX_OPTIONS, optionKey, type OptionDraft } from "./activityDraft";

const MAX_DAYS = 7;

const KINDS = [
  ...Array.from({ length: MAX_DAYS }, (_, i) => ({ value: String(i + 1), label: `${i + 1} ${i === 0 ? "día" : "días"} por semana` })),
  { value: FLAT, label: "Cuota fija (sin importar los días)" },
];

export interface PriceOptionRowsProps {
  options: readonly OptionDraft[];
  errors: Readonly<Record<string, string>>;
  onChange: (index: number, patch: Partial<OptionDraft>) => void;
  onAdd: () => void;
  onRemove: (index: number) => void;
}

/**
 * What the activity costs per period: by the days a week the pupil comes, or
 * one flat fee whatever the days. Each pupil is enrolled with one of these.
 */
export function PriceOptionRows({ options, errors, onChange, onAdd, onRemove }: PriceOptionRowsProps): ReactElement {
  return (
    <div style={{ display: "grid", gap: 12 }}>
      {options.map((o, i) => (
        <div key={o.key} role="group" aria-label={`Precio ${i + 1}`}
          style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(180px, 1fr))", gap: 10, alignItems: "end" }}>
          <SelectField label="Esquema" required value={o.kind} options={KINDS} error={errors[optionKey(i, "kind")]}
            onChange={(v) => onChange(i, { kind: v as OptionDraft["kind"] })}
            help="Cuántos días a la semana viene el alumno por este precio, o una cuota fija sin importar los días." />
          <MoneyField label="Monto" required value={o.amount} error={errors[optionKey(i, "amount")]}
            onChange={(v) => onChange(i, { amount: v })}
            help="Lo que se cobra por periodo, en pesos." />
          {options.length > 1 && (
            <Btn kind="ghost" icon="trash" onClick={() => onRemove(i)} aria-label={`Quitar precio ${i + 1}`}>Quitar</Btn>
          )}
        </div>
      ))}
      {options.length < MAX_OPTIONS && (
        <div><Btn kind="ghost" icon="plus" onClick={onAdd}>Agregar precio</Btn></div>
      )}
    </div>
  );
}
