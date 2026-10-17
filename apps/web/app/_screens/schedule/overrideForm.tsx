"use client";

import { useEffect, useState, type ReactElement } from "react";

import { DateField, Field, inputStyle, SelectField, TextField, useRequired, type RequiredState } from "../../_design/form";
import { ChipRow } from "../../_design/students/StudentFilters";
import { API_URL } from "../../api-url";
import { createOverride, listAvailableTeachers, type AvailableTeacher, type OverrideKind, type ScheduleBlock, type ScheduleBoard, type TeacherMatch } from "../../schedule/api";
import { WEEKDAYS, type ScheduleIndex } from "../../schedule/catalog";
import { nextDateOnWeekday, weekdayOf } from "../../schedule/time";
import { chipOption } from "./parts";

interface Values {
  date: string;
  kind: OverrideKind;
  teacherId: string;
  swapBlockId: string;
  reason: string;
}

export interface OverrideForm {
  values: Values;
  set: (patch: Partial<Values>) => void;
  required: RequiredState;
  dateError: string | undefined;
  judgeDate: () => void;
  available: AvailableTeacher[] | null;
  valid: () => Values | null;
}

const KINDS = [chipOption("substitute", "Suplencia"), chipOption("swap", "Intercambio"), chipOption("cancel", "Cancelar la clase")];
const MATCH_LABEL: Record<TeacherMatch, string> = { subject: "Imparte la materia", grade: "Da clase en el grado", other: "Disponible" };
const MATCH_ORDER: readonly TeacherMatch[] = ["subject", "grade", "other"];

const dayName = (weekday: number): string => (WEEKDAYS.find((d) => d.weekday === weekday)?.label ?? "").toLocaleLowerCase("es-MX");

/**
 * `open`: the section is on screen — only then is it worth asking who is free.
 * `initial` prefills what the caller already knows (T19: covering an absence
 * already knows why).
 */
export function useOverrideForm(block: ScheduleBlock, board: ScheduleBoard, today: string, open: boolean, initial: Partial<Values> = {}): OverrideForm {
  const [values, setValues] = useState<Values>(() => ({ date: nextDateOnWeekday(today, block.weekday), kind: "substitute", teacherId: "", swapBlockId: "", reason: "", ...initial }));
  const [dateError, setDateError] = useState<string | undefined>(undefined);
  const [available, setAvailable] = useState<AvailableTeacher[] | null>(null);
  const extra = values.kind === "substitute" ? ["teacherId"] : values.kind === "swap" ? ["swapBlockId"] : [];
  const required = useRequired(["date", ...extra, "reason"]);
  const { startsOn, endsOn } = board.cycle;
  // The form is noValidate: the field's min/max only guide, this judges (T13).
  const wrongDay = (date: string): string | undefined => {
    if (date === "") return undefined;
    if (date < startsOn || date > endsOn) return `Elige un día del ciclo: del ${startsOn} al ${endsOn}.`;
    return weekdayOf(date) !== block.weekday ? `Esta clase es los ${dayName(block.weekday)}: elige un ${dayName(block.weekday)}.` : undefined;
  };

  useEffect(() => {
    if (!open || values.kind !== "substitute" || values.date === "" || weekdayOf(values.date) !== block.weekday) return;
    let live = true;
    setAvailable(null);
    listAvailableTeachers(API_URL, {
      cycleId: board.cycle.id, groupId: block.groupId, subjectId: block.subjectId, date: values.date,
      startsAt: block.startsAt, endsAt: block.endsAt, excludeBlockId: block.id,
    })
      .then((got) => live && setAvailable(got.filter((t) => t.teacherId !== block.teacherId)))
      .catch(() => live && setAvailable([]));
    return () => {
      live = false;
    };
  }, [open, values.kind, values.date, block, board.cycle.id]);

  const valid = (): Values | null => {
    const ok = required.check({ ...values });
    const problem = wrongDay(values.date);
    setDateError(problem);
    return ok && problem === undefined ? values : null;
  };

  return {
    values,
    set: (patch) => setValues({ ...values, ...patch }),
    required,
    dateError,
    judgeDate: () => setDateError(wrongDay(values.date)),
    available,
    valid,
  };
}

export async function saveOverride(form: OverrideForm, block: ScheduleBlock, replaceOverrideId?: string): Promise<string | null> {
  const v = form.valid();
  if (v === null) return null;
  await createOverride(API_URL, {
    blockId: block.id, date: v.date, kind: v.kind, reason: v.reason.trim(),
    ...(replaceOverrideId !== undefined ? { replaceOverrideId } : {}),
    ...(v.kind === "substitute" ? { teacherId: v.teacherId } : {}),
    ...(v.kind === "swap" ? { swapBlockId: v.swapBlockId } : {}),
  });
  return `Cambio registrado para el ${v.date}`;
}

/** Date, kind, the substitute or the class to swap with, and the reason. */
export function OverrideFields({ form, block, board, index }: { form: OverrideForm; block: ScheduleBlock; board: ScheduleBoard; index: ScheduleIndex }): ReactElement {
  const { values, set, required, dateError, available } = form;
  const swaps = board.blocks
    .filter((b) => b.groupId === block.groupId && b.weekday === block.weekday && b.id !== block.id)
    .map((b) => ({ value: b.id, label: `${index.subjectName(b.subjectId)} — ${b.startsAt} a ${b.endsAt}` }));
  return (
    <div style={{ display: "grid", gap: 16, gridTemplateColumns: "minmax(0, 1fr)" }}>
      <DateField label="Fecha" required value={values.date} min={board.cycle.startsOn} max={board.cycle.endsOn}
        error={required.errors.date ?? dateError} onClearError={() => required.clear("date")}
        onChange={(date) => set({ date })} onBlur={form.judgeDate} />
      <ChipRow label="Tipo" options={KINDS} chosen={values.kind} onChoose={(kind) => set({ kind: kind as OverrideKind })} showCounts={false} />
      {values.kind === "substitute" && <SubstitutePicker form={form} available={available} />}
      {values.kind === "swap" && (
        <SelectField label="Intercambiar con" required value={values.swapBlockId} error={required.errors.swapBlockId} onClearError={() => required.clear("swapBlockId")}
          help="Otra clase del mismo grupo ese mismo día: las dos cambian de hora sólo en esa fecha."
          onChange={(swapBlockId) => set({ swapBlockId })} options={swaps} />
      )}
      <TextField label="Motivo" required value={values.reason} error={required.errors.reason} onClearError={() => required.clear("reason")}
        onChange={(reason) => set({ reason })} />
    </div>
  );
}

/** Free teachers, best fits first, grouped by why they fit. */
function SubstitutePicker({ form, available }: { form: OverrideForm; available: AvailableTeacher[] | null }): ReactElement {
  const { values, set, required } = form;
  const error = required.errors.teacherId;
  const help = available !== null && available.length === 0 ? "Nadie está libre a esa hora ese día." : "Quien no tiene clase a esa hora ese día; primero quien imparte la materia.";
  return (
    <Field label="Suplente" required error={error} help={help}>
      {(id, describedBy) => (
        <select id={id} aria-describedby={describedBy} aria-invalid={error !== undefined ? true : undefined} aria-required
          value={values.teacherId} style={inputStyle(error !== undefined)}
          onChange={(e) => { set({ teacherId: e.target.value }); required.clear("teacherId"); }}>
          <option value="" />
          {MATCH_ORDER.map((match) => {
            const teachers = (available ?? []).filter((t) => t.match === match);
            if (teachers.length === 0) return null;
            return (
              <optgroup key={match} label={MATCH_LABEL[match]}>
                {teachers.map((t) => <option key={t.teacherId} value={t.teacherId}>{`${t.firstName} ${t.lastName}`}</option>)}
              </optgroup>
            );
          })}
        </select>
      )}
    </Field>
  );
}
