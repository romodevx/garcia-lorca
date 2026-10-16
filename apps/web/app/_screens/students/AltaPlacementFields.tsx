"use client";

import type { ChangeEvent, ReactElement } from "react";

import { DateField, Field, inputStyle, SelectField } from "../../_design/form";
import { Icon } from "../../_design/Icon";
import type { GradeLevel, LabelledGroup, Level } from "../../config/api";
import { groupOptionLabel } from "./altaModel";
import type { AltaForm } from "./useAltaForm";

/**
 * Where and since when a new pupil enters (T9 / ISSUE-009). Both have a default
 * that fits the everyday alta — the first group with a free seat, and today —
 * and on a page they are shown outright, already set to those defaults.
 */

export const AUTOMATIC_GROUP_LABEL = "Automático (el primero con lugar)";
const PAST_ENTRY_HINT = "Se cobrarán las colegiaturas desde ese mes.";
export const FUTURE_ENTRY_ERROR = "La fecha de ingreso no puede ser posterior a hoy.";

/**
 * The entry day the request carries: "" for today (or a cleared field), so a
 * user without finance:write is never refused for a day they did not choose.
 */
export function entryDayToSend(entryDay: string, today: string): string {
  return entryDay === today ? "" : entryDay;
}

/** The chosen grade's groups, by name: the order the school reads them in. */
export function groupsOfGrade(groups: readonly LabelledGroup[], gradeLevelId: string): LabelledGroup[] {
  return groups
    .filter((g) => g.gradeLevelId === gradeLevelId)
    .sort((a, b) => a.name.localeCompare(b.name, "es", { numeric: true }));
}

interface Props {
  form: AltaForm;
  levels: readonly Level[];
  grades: readonly GradeLevel[];
  /** Only the chosen grade's groups (see groupsOfGrade). */
  groups: readonly LabelledGroup[];
  cycleStartsOn: string;
}

export function AltaPlacementFields({ form, levels, grades, groups, cycleStartsOn }: Props): ReactElement {
  const { draft, errors, clearError, today } = form;
  const isPast = draft.entryDay !== "" && draft.entryDay < today;
  return (
    <>
      {/* Level and grade are one logical choice, so they may sit side by side. */}
      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(240px, 1fr))", gap: 14 }}>
        <SelectField
          label="Nivel"
          value={draft.levelId}
          onChange={form.chooseLevel}
          required
          // The pupil's file holds the level: once created it is not changed here.
          disabled={form.created}
          help="El nivel educativo donde va a cursar: preescolar, primaria o secundaria."
          options={levels.map((l) => ({ value: l.id, label: l.name }))}
          error={errors.levelId}
          onClearError={() => clearError("levelId")}
        />
        <SelectField
          label="Grado"
          value={draft.gradeLevelId}
          onChange={form.chooseGrade}
          required
          disabled={grades.length === 0}
          help="El grado dentro del nivel. Se enciende en cuanto eliges el nivel."
          options={grades.map((g) => ({ value: g.id, label: g.name }))}
          error={errors.gradeLevelId}
          onClearError={() => clearError("gradeLevelId")}
        />
      </div>
      <Field label="Grupo" help="Déjalo en automático y el alumno entra al primer grupo de su grado con lugar. Elige uno si ya sabes a qué salón va.">
        {(id, describedBy) => (
          <select
            id={id}
            aria-describedby={describedBy}
            value={draft.groupId}
            onChange={(e: ChangeEvent<HTMLSelectElement>) => form.set("groupId", e.target.value)}
            style={inputStyle(false)}
          >
            <option value="">{AUTOMATIC_GROUP_LABEL}</option>
            {groups.map((g) => (
              <option key={g.id} value={g.id}>{groupOptionLabel(g)}</option>
            ))}
          </select>
        )}
      </Field>
      <div>
        <DateField
          label="Fecha de ingreso"
          value={draft.entryDay}
          onChange={(v) => form.set("entryDay", v)}
          min={cycleStartsOn !== "" ? cycleStartsOn : undefined}
          max={today}
          error={errors.enrolledOn}
          onClearError={() => clearError("enrolledOn")}
          help="El día en que el alumno empezó a venir. Va entre el inicio del ciclo y hoy; una fecha anterior a hoy necesita permiso de finanzas."
        />
        {isPast && <p style={{ fontSize: 13.5, color: "var(--text-muted)", margin: "6px 0 0" }}>{PAST_ENTRY_HINT}</p>}
      </div>
      <div style={{ display: "flex", gap: 11, padding: "14px 16px", borderRadius: "var(--r-md)", background: "var(--surface-3)", border: "1px solid var(--border)" }}>
        <span style={{ color: "var(--forest)", flexShrink: 0, marginTop: 1 }}><Icon name="check" size={19} stroke={2.6} /></span>
        <p style={{ fontSize: 13.5, color: "var(--text-muted)", lineHeight: 1.5, margin: 0 }}>Si no eliges grupo, <strong>se asigna solo</strong> según el cupo de cada salón: se llena A y el siguiente alumno pasa a B. Los documentos se cargan después.</p>
      </div>
    </>
  );
}
