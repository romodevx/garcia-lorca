"use client";

import { useState, type ReactElement } from "react";

import { SelectField, TextField, useRequired, type RequiredState } from "../../_design/form";
import { API_URL } from "../../api-url";
import { createBlock, updateBlock, type ScheduleBlock, type ScheduleBoard } from "../../schedule/api";
import { assignmentsOfGroup, WEEKDAYS, type ScheduleIndex } from "../../schedule/catalog";
import { defaultEnd, SLOT_MINUTES, timeOptions, toHHMM, toMinutes } from "../../schedule/time";

/** What the drawer was opened on: an empty slot, or a class. */
export type DrawerTarget =
  | { kind: "new"; weekday: number; start: number; groupId: string; teacherId: string }
  | { kind: "edit"; block: ScheduleBlock };

interface Values {
  weekday: string;
  startsAt: string;
  endsAt: string;
  groupId: string;
  assignmentId: string;
  reason: string;
}

export interface BlockForm {
  values: Values;
  set: (patch: Partial<Values>) => void;
  required: RequiredState;
  timeError: string | undefined;
  /** Valid: the values the gateway takes. Invalid: marks the fields and gives null. */
  valid: () => Values | null;
}

const END_BEFORE_START = "La clase debe terminar después de empezar.";

function initial(target: DrawerTarget, board: ScheduleBoard, gridEnd: number): Values {
  if (target.kind === "edit") {
    const b = target.block;
    return { weekday: String(b.weekday), startsAt: b.startsAt, endsAt: b.endsAt, groupId: b.groupId, assignmentId: b.assignmentId, reason: "" };
  }
  // A teacher filter on: their assignment in this group is the likely class.
  const mine = board.assignments.find((a) => a.groupId === target.groupId && a.teacherId === target.teacherId);
  return {
    weekday: String(target.weekday),
    startsAt: toHHMM(target.start),
    endsAt: toHHMM(defaultEnd(target.start, gridEnd)),
    groupId: target.groupId,
    assignmentId: target.teacherId !== "" ? (mine?.id ?? "") : "",
    reason: "",
  };
}

export function useBlockForm(target: DrawerTarget, board: ScheduleBoard, gridEnd: number): BlockForm {
  const [values, setValues] = useState<Values>(() => initial(target, board, gridEnd));
  const [timeError, setTimeError] = useState<string | undefined>(undefined);
  const required = useRequired(board.started ? ["weekday", "startsAt", "endsAt", "groupId", "assignmentId", "reason"] : ["weekday", "startsAt", "endsAt", "groupId", "assignmentId"]);
  const order = (v: Values): string | undefined => (toMinutes(v.endsAt) <= toMinutes(v.startsAt) ? END_BEFORE_START : undefined);

  const set = (patch: Partial<Values>): void => {
    const next = { ...values, ...patch };
    setValues(next);
    // Times are selects: a choice is final, so it is judged at once (and forgiven at once).
    if (patch.startsAt !== undefined || patch.endsAt !== undefined) setTimeError(order(next));
  };

  const valid = (): Values | null => {
    const ok = required.check({ ...values });
    const problem = order(values);
    setTimeError(problem);
    return ok && problem === undefined ? values : null;
  };

  return { values, set, required, timeError, valid };
}

/** Saves the form; null when it did not validate (nothing was sent). */
export async function saveBlock(form: BlockForm, target: DrawerTarget, started: boolean): Promise<string | null> {
  const v = form.valid();
  if (v === null) return null;
  const input = { assignmentId: v.assignmentId, weekday: Number(v.weekday), startsAt: v.startsAt, endsAt: v.endsAt, ...(started ? { reason: v.reason.trim() } : {}) };
  if (target.kind === "edit") {
    await updateBlock(API_URL, target.block.id, input);
    return "Clase actualizada";
  }
  await createBlock(API_URL, input);
  return "Clase agregada";
}

/** Day, start, end, group, "Asignatura — Docente" and, once the cycle started, the reason. */
export function BlockFields({ form, board, index, grid }: { form: BlockForm; board: ScheduleBoard; index: ScheduleIndex; grid: { start: number; end: number } }): ReactElement {
  const { values, set, required, timeError } = form;
  const starts = timeOptions(grid.start, grid.end - SLOT_MINUTES).map((t) => ({ value: t, label: t }));
  const ends = timeOptions(grid.start + SLOT_MINUTES, grid.end).map((t) => ({ value: t, label: t }));
  const groups = board.groups
    .map((g) => ({ value: g.id, label: index.groupLabel(g.id) }))
    .sort((a, b) => a.label.localeCompare(b.label, "es-MX", { numeric: true }));
  const err = (key: keyof Values): string | undefined => required.errors[key];
  return (
    <div style={{ display: "grid", gap: 16, gridTemplateColumns: "minmax(0, 1fr)" }}>
      <SelectField label="Día" required value={values.weekday} error={err("weekday")} onClearError={() => required.clear("weekday")}
        onChange={(weekday) => set({ weekday })} options={WEEKDAYS.map((d) => ({ value: String(d.weekday), label: d.label }))} />
      {/* Start and end are one logical entity: a pair, side by side. */}
      <div style={{ display: "grid", gap: 12, gridTemplateColumns: "repeat(2, minmax(0, 1fr))" }}>
        <SelectField label="Inicio" required value={values.startsAt} error={err("startsAt")} onClearError={() => required.clear("startsAt")}
          onChange={(startsAt) => set({ startsAt })} options={starts} />
        <SelectField label="Fin" required value={values.endsAt} error={err("endsAt") ?? timeError} onClearError={() => required.clear("endsAt")}
          onChange={(endsAt) => set({ endsAt })} options={ends} />
      </div>
      <SelectField label="Grupo" required value={values.groupId} error={err("groupId")} onClearError={() => required.clear("groupId")}
        onChange={(groupId) => set({ groupId, assignmentId: "" })} options={groups} />
      <SelectField label="Asignatura — Docente" required value={values.assignmentId} error={err("assignmentId")} onClearError={() => required.clear("assignmentId")}
        help="Sólo las materias que tiene asignadas este grupo en Docentes (carga académica)."
        onChange={(assignmentId) => set({ assignmentId })} options={assignmentsOfGroup(board, index, values.groupId)} />
      {board.started && (
        <TextField label="Motivo del cambio" required value={values.reason} error={err("reason")} onClearError={() => required.clear("reason")}
          help="El ciclo ya empezó: cada cambio al horario queda en el historial con su motivo."
          onChange={(reason) => set({ reason })} />
      )}
    </div>
  );
}
