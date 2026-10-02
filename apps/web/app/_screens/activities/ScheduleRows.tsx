"use client";

import type { ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { SelectField } from "../../_design/form";
import { WEEKDAY_NAMES } from "../../activities/format";
import { timeOptions, toMinutes } from "../../schedule/time";
import { MAX_SLOTS, slotKey, type SlotDraft } from "./activityDraft";

/** Activities run outside class hours too: early morning to evening. */
const FIRST_TIME = "06:00";
const LAST_TIME = "22:00";

const TIMES = timeOptions(toMinutes(FIRST_TIME), toMinutes(LAST_TIME)).map((t) => ({ value: t, label: t }));
const DAYS = WEEKDAY_NAMES.slice(1).map((name, i) => ({ value: String(i + 1), label: name }));

export interface ScheduleRowsProps {
  slots: readonly SlotDraft[];
  errors: Readonly<Record<string, string>>;
  onChange: (index: number, patch: Partial<SlotDraft>) => void;
  onAdd: () => void;
  onRemove: (index: number) => void;
}

/**
 * The activity's week: one row per day, each with its own hours ("varios días
 * con horarios distintos"). Día, inicio y fin side by side: they are one slot.
 */
export function ScheduleRows({ slots, errors, onChange, onAdd, onRemove }: ScheduleRowsProps): ReactElement {
  return (
    <div style={{ display: "grid", gap: 12 }}>
      {slots.map((s, i) => (
        <div key={s.key} role="group" aria-label={`Horario ${i + 1}`}
          style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(130px, 1fr))", gap: 10, alignItems: "end" }}>
          <SelectField label="Día" required value={s.weekday} options={DAYS} error={errors[slotKey(i, "weekday")]}
            onChange={(v) => onChange(i, { weekday: v })} />
          <SelectField label="Inicio" required value={s.start} options={TIMES} error={errors[slotKey(i, "start")]}
            onChange={(v) => onChange(i, { start: v })} />
          <SelectField label="Fin" required value={s.end} options={TIMES} error={errors[slotKey(i, "end")]}
            onChange={(v) => onChange(i, { end: v })} />
          {slots.length > 1 && (
            <Btn kind="ghost" icon="trash" onClick={() => onRemove(i)} aria-label={`Quitar horario ${i + 1}`}>Quitar</Btn>
          )}
        </div>
      ))}
      {slots.length < MAX_SLOTS && (
        <div><Btn kind="ghost" icon="plus" onClick={onAdd}>Agregar día</Btn></div>
      )}
    </div>
  );
}
