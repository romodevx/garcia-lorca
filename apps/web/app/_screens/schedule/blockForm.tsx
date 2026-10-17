"use client";

import { useState, type ReactElement } from "react";

import { SelectField, TextField, TimeField, useRequired, type RequiredState } from "../../_design/form";
import { API_URL } from "../../api-url";
import { createBlock, updateBlock, type ScheduleBlock, type ScheduleBoard } from "../../schedule/api";
import { assignmentsOfGroup, WEEKDAYS, type ScheduleIndex } from "../../schedule/catalog";
import { defaultEnd, SLOT_MINUTES, timeError, toHHMM, toMinutes } from "../../schedule/time";

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
  /** What is wrong with the typed start or end (range, step, order), judged on blur. */
  timeErrors: TimeErrors;
  /** Judges both times: the fields' blur. */
  judgeTimes: () => void;
  /** Valid: the values the gateway takes. Invalid: marks the fields and gives null. */
  valid: () => Values | null;
}

const END_BEFORE_START = "La clase debe terminar después de empezar.";

export interface TimeErrors {
  startsAt?: string;
  endsAt?: string;
}

/** The minutes the level's day spans; a class starts and ends inside it. */
export interface GridSpan {
  start: number;
  end: number;
}

/** The bounds of each time: a start leaves room for one slot, an end follows one. */
function timeBounds(grid: GridSpan): { startsAt: { min: number; max: number }; endsAt: { min: number; max: number } } {
  return {
    startsAt: { min: grid.start, max: grid.end - SLOT_MINUTES },
    endsAt: { min: grid.start + SLOT_MINUTES, max: grid.end },
  };
}

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

export function useBlockForm(target: DrawerTarget, board: ScheduleBoard, grid: GridSpan): BlockForm {
  const [saved] = useState<Values>(() => initial(target, board, grid.end));
  const [values, setValues] = useState<Values>(saved);
  const [timeErrors, setTimeErrors] = useState<TimeErrors>({});
  const required = useRequired(board.started ? ["weekday", "startsAt", "endsAt", "groupId", "assignmentId", "reason"] : ["weekday", "startsAt", "endsAt", "groupId", "assignmentId"]);

  // A time already saved is kept as it is, even where the picker would not reach (ISSUE-028).
  const judge = (v: Values): TimeErrors => {
    const bounds = timeBounds(grid);
    const startsAt = timeError(v.startsAt, { ...bounds.startsAt, kept: saved.startsAt });
    const endsAt = timeError(v.endsAt, { ...bounds.endsAt, kept: saved.endsAt });
    const order = v.startsAt !== "" && v.endsAt !== "" && toMinutes(v.endsAt) <= toMinutes(v.startsAt) ? END_BEFORE_START : undefined;
    return { startsAt, endsAt: endsAt ?? (startsAt === undefined ? order : undefined) };
  };
  const showing = timeErrors.startsAt !== undefined || timeErrors.endsAt !== undefined;

  const set = (patch: Partial<Values>): void => {
    const next = { ...values, ...patch };
    setValues(next);
    // A typed time is judged on blur; once an error shows, every change re-checks it.
    if (showing && (patch.startsAt !== undefined || patch.endsAt !== undefined)) setTimeErrors(judge(next));
  };

  const valid = (): Values | null => {
    const ok = required.check({ ...values });
    const problems = judge(values);
    setTimeErrors(problems);
    return ok && problems.startsAt === undefined && problems.endsAt === undefined ? values : null;
  };

  return { values, set, required, timeErrors, judgeTimes: () => setTimeErrors(judge(values)), valid };
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
export function BlockFields({ form, board, index, grid }: { form: BlockForm; board: ScheduleBoard; index: ScheduleIndex; grid: GridSpan }): ReactElement {
  const { values, set, required, timeErrors, judgeTimes } = form;
  const bounds = timeBounds(grid);
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
        <TimeField label="Inicio" required value={values.startsAt} error={err("startsAt") ?? timeErrors.startsAt} onClearError={() => required.clear("startsAt")}
          onChange={(startsAt) => set({ startsAt })} onBlur={judgeTimes}
          min={toHHMM(bounds.startsAt.min)} max={toHHMM(bounds.startsAt.max)} stepMinutes={SLOT_MINUTES} />
        <TimeField label="Fin" required value={values.endsAt} error={err("endsAt") ?? timeErrors.endsAt} onClearError={() => required.clear("endsAt")}
          onChange={(endsAt) => set({ endsAt })} onBlur={judgeTimes}
          min={toHHMM(bounds.endsAt.min)} max={toHHMM(bounds.endsAt.max)} stepMinutes={SLOT_MINUTES} />
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
