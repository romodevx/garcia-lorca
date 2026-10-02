/**
 * The rules of a grade's evaluation periods as the screen checks them (T14).
 *
 * The server is the authority — it refuses overlaps even between two people
 * saving at once — but a person should hear "se encima con el 1.er trimestre"
 * while still on the field, not after a round trip. These are the same rules:
 * every period named and dated, start ≤ end, inside the cycle, no two sharing
 * a day, at most twelve. Dates are `YYYY-MM-DD` strings, which compare in
 * calendar order as plain text.
 */

import type { EvaluationPeriod, EvaluationPeriodInput } from "./api";

/** The most periods a grade may have (the server's limit too). */
export const MAX_PERIODS = 12;

/** How long a new period is proposed to last: a trimester. */
const DEFAULT_PERIOD_MONTHS = 3;

/** A period being edited. `key` is the period's id, or a local one for a new row. */
export interface PeriodDraft {
  key: string;
  name: string;
  startsOn: string;
  endsOn: string;
}

export type PeriodField = "name" | "startsOn" | "endsOn";

/** What is wrong, and on which field of which row to say it. */
export interface PeriodProblem {
  key: string;
  field: PeriodField;
  message: string;
}

/** Only the cycle's dates matter here. */
export interface CycleSpan {
  startsOn: string;
  endsOn: string;
}

/** "2026-11-30" → "30/11/2026", the way the school writes a date. */
export function formatDay(iso: string): string {
  const [y, m, d] = iso.split("-");
  return `${d}/${m}/${y}`;
}

function addDays(iso: string, days: number): string {
  const d = new Date(`${iso}T00:00:00Z`);
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
}

function addMonths(iso: string, months: number): string {
  const d = new Date(`${iso}T00:00:00Z`);
  d.setUTCMonth(d.getUTCMonth() + months);
  return d.toISOString().slice(0, 10);
}

/** The problems of one row on its own: what a blur can already tell. */
export function rowProblems(row: PeriodDraft, cycle: CycleSpan): PeriodProblem[] {
  const out: PeriodProblem[] = [];
  const say = (field: PeriodField, message: string): void => {
    out.push({ key: row.key, field, message });
  };
  if (row.name.trim() === "") say("name", "Escribe el nombre del periodo.");
  if (row.startsOn === "") say("startsOn", "Elige la fecha de inicio.");
  if (row.endsOn === "") say("endsOn", "Elige la fecha de fin.");
  if (row.startsOn === "" || row.endsOn === "") return out;

  const outside = `Queda fuera del ciclo (${formatDay(cycle.startsOn)} a ${formatDay(cycle.endsOn)}).`;
  if (row.startsOn < cycle.startsOn) say("startsOn", outside);
  if (row.endsOn < row.startsOn) say("endsOn", "El fin es antes que el inicio.");
  else if (row.endsOn > cycle.endsOn) say("endsOn", outside);
  return out;
}

/**
 * Every problem of a grade's set, row problems first. Overlaps are found by
 * sorting on the start and comparing neighbours (O(n log n)); the later
 * period is the one marked, naming the one it runs into.
 */
export function periodProblems(rows: readonly PeriodDraft[], cycle: CycleSpan): PeriodProblem[] {
  const out = rows.flatMap((r) => rowProblems(r, cycle));
  if (rows.length > MAX_PERIODS) {
    const extra = rows[MAX_PERIODS];
    if (extra !== undefined) out.push({ key: extra.key, field: "name", message: `Un grado puede tener hasta ${MAX_PERIODS} periodos.` });
  }
  const broken = new Set(out.map((p) => p.key));
  const dated = rows.filter((r) => !broken.has(r.key)).sort((a, b) => a.startsOn.localeCompare(b.startsOn));
  for (let i = 1; i < dated.length; i++) {
    const prev = dated[i - 1];
    const next = dated[i];
    if (prev !== undefined && next !== undefined && next.startsOn <= prev.endsOn) {
      out.push({ key: next.key, field: "startsOn", message: `Se encima con «${prev.name.trim()}» (termina el ${formatDay(prev.endsOn)}).` });
    }
  }
  return out;
}

/**
 * A new row with everything a person would type anyway: the next number, and
 * dates from the day after the last period to three months later (clamped to
 * the cycle). A cycle already covered leaves the dates to the person.
 */
export function nextDraft(rows: readonly PeriodDraft[], cycle: CycleSpan, key: string): PeriodDraft {
  const name = `PERIODO ${rows.length + 1}`;
  const lastEnd = rows.reduce((max, r) => (r.endsOn > max ? r.endsOn : max), "");
  const startsOn = lastEnd === "" ? cycle.startsOn : addDays(lastEnd, 1);
  if (startsOn > cycle.endsOn) return { key, name, startsOn: "", endsOn: "" };
  const proposedEnd = addDays(addMonths(startsOn, DEFAULT_PERIOD_MONTHS), -1);
  return { key, name, startsOn, endsOn: proposedEnd > cycle.endsOn ? cycle.endsOn : proposedEnd };
}

/** The server's periods of one grade as editable rows, in ordinal order. */
export function draftsOf(periods: readonly EvaluationPeriod[]): PeriodDraft[] {
  return [...periods]
    .sort((a, b) => a.ordinal - b.ordinal)
    .map((p) => ({ key: p.id, name: p.name, startsOn: p.startsOn, endsOn: p.endsOn }));
}

/**
 * The rows as the server takes them: numbered 1…n in date order, so "2.º
 * trimestre" is always the second one on the calendar.
 */
export function toInputs(rows: readonly PeriodDraft[]): EvaluationPeriodInput[] {
  return [...rows]
    .sort((a, b) => a.startsOn.localeCompare(b.startsOn))
    .map((r, i) => ({ ordinal: i + 1, name: r.name.trim(), startsOn: r.startsOn, endsOn: r.endsOn }));
}
