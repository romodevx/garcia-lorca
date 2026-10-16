"use client";

import { useState, type ChangeEvent, type ReactElement } from "react";
import { DateField, Field, inputStyle } from "../../_design/form";
import { Icon } from "../../_design/Icon";
import type { LabelledGroup } from "../../config/api";
import { dayWithYearLabel } from "../../_lib/localDate";
import { schoolISODate } from "../../_lib/schoolDay";

/**
 * Where and since when a new pupil enters (T9 / ISSUE-009).
 *
 * Both have a default that fits the everyday alta — the first group with a free
 * seat, and today — so they wait behind one disclosure that SAYS those defaults.
 * Visible, the drawer would pass eight fields, the most a drawer takes.
 */

export const AUTOMATIC_GROUP_LABEL = "Automático (el primero con lugar)";
const PAST_ENTRY_HINT = "Se cobrarán las colegiaturas desde ese mes.";
export const FUTURE_ENTRY_ERROR = "La fecha de ingreso no puede ser posterior a hoy.";

export interface PlacementDraft {
  /** "" lets capacity choose the group (POST /enrollments/by-grade). */
  groupId: string;
  setGroupId: (id: string) => void;
  entryDay: string;
  setEntryDay: (day: string) => void;
  /** The school's today, fixed when the drawer opened. */
  today: string;
  open: boolean;
  setOpen: (open: boolean) => void;
}

export function usePlacementDraft(): PlacementDraft {
  const [today] = useState(() => schoolISODate());
  const [groupId, setGroupId] = useState("");
  const [entryDay, setEntryDay] = useState(today);
  const [open, setOpen] = useState(false);
  return { groupId, setGroupId, entryDay, setEntryDay, today, open, setOpen };
}

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

function summary(draft: PlacementDraft, groups: readonly LabelledGroup[]): string {
  const chosen = groups.find((g) => g.id === draft.groupId);
  const where = chosen === undefined ? "Grupo automático (el primero con lugar)" : `Grupo ${chosen.name}`;
  const day = entryDayToSend(draft.entryDay, draft.today);
  return `${where} · ${day === "" ? "ingresa hoy" : `ingresa el ${dayWithYearLabel(day)}`}`;
}

interface Props {
  draft: PlacementDraft;
  /** Only the chosen grade's groups (see groupsOfGrade). */
  groups: readonly LabelledGroup[];
  cycleStartsOn: string;
  entryDayError?: string;
  onClearEntryDayError: () => void;
}

export function QuickAddPlacement({ draft, groups, cycleStartsOn, entryDayError, onClearEntryDayError }: Props): ReactElement {
  const isPast = draft.entryDay !== "" && draft.entryDay < draft.today;
  return (
    <div>
      <p style={{ fontSize: 14, color: "var(--text-muted)", margin: "0 0 2px" }}>{summary(draft, groups)}</p>
      <button
        type="button"
        aria-expanded={draft.open}
        aria-controls="alta-grupo-fecha"
        onClick={() => draft.setOpen(!draft.open)}
        style={{ display: "inline-flex", alignItems: "center", gap: 6, minHeight: 44, padding: "0 4px", background: "none", border: "none", color: "var(--forest)", fontSize: 14.5, fontWeight: 700, cursor: "pointer" }}
      >
        <span style={{ display: "inline-flex", transform: draft.open ? "rotate(90deg)" : "none", transition: "transform .15s" }}>
          <Icon name="chevron" size={16} stroke={2.4} />
        </span>
        Elegir grupo o fecha de ingreso
      </button>
      {draft.open && (
        <div id="alta-grupo-fecha" style={{ display: "grid", gap: 14, marginTop: 8 }}>
          <Field label="Grupo" help="Déjalo en automático y el alumno entra al primer grupo de su grado con lugar. Elige uno si ya sabes a qué salón va.">
            {(id, describedBy) => (
              <select
                id={id}
                aria-describedby={describedBy}
                value={draft.groupId}
                onChange={(e: ChangeEvent<HTMLSelectElement>) => draft.setGroupId(e.target.value)}
                style={inputStyle(false)}
              >
                <option value="">{AUTOMATIC_GROUP_LABEL}</option>
                {groups.map((g) => (
                  <option key={g.id} value={g.id}>{`${g.name} · ${g.capacity} lugares`}</option>
                ))}
              </select>
            )}
          </Field>
          <div>
            <DateField
              label="Fecha de ingreso"
              value={draft.entryDay}
              onChange={draft.setEntryDay}
              min={cycleStartsOn !== "" ? cycleStartsOn : undefined}
              max={draft.today}
              error={entryDayError}
              onClearError={onClearEntryDayError}
              help="El día en que el alumno empezó a venir. Va entre el inicio del ciclo y hoy; una fecha anterior a hoy necesita permiso de finanzas."
            />
            {isPast && <p style={{ fontSize: 13.5, color: "var(--text-muted)", margin: "6px 0 0" }}>{PAST_ENTRY_HINT}</p>}
          </div>
        </div>
      )}
    </div>
  );
}
