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
import { TRIMESTER_NAMES, trimesterSpans } from "./termMonths";

/** The most periods a grade may have (the server's limit too). */
export const MAX_PERIODS = 12;

/** How long a period past the three trimesters is proposed to last. */
const DEFAULT_PERIOD_MONTHS = 3;

/**
 * A period being edited. `key` names the row on screen; `id` is the saved
 * period it edits ("" for a new row). The id travels with the row, so removing
 * the first period removes THAT period even though the rest are renumbered.
 */
export interface PeriodDraft {
  key: string;
  id: string;
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

/** The trimester's name for the first three rows; "4.º periodo" and on after them. */
function periodName(index: number): string {
  return TRIMESTER_NAMES[index] ?? `${index + 1}.º periodo`;
}

/**
 * Where a new row starting on `startsOn` ends: the end of its third of the
 * cycle, so three rows cover the cycle exactly. When the earlier rows already
 * ran past that third (or it is a fourth row), three months, clamped.
 */
function proposedEnd(index: number, startsOn: string, cycle: CycleSpan): string {
  const thirdEnd = trimesterSpans(cycle)[index]?.endsOn;
  if (thirdEnd !== undefined && thirdEnd >= startsOn) return thirdEnd;
  const threeMonths = addDays(addMonths(startsOn, DEFAULT_PERIOD_MONTHS), -1);
  return threeMonths > cycle.endsOn ? cycle.endsOn : threeMonths;
}

/**
 * A new row with everything a person would type anyway: the trimester's name,
 * and dates from the day after the last period to the end of its third of the
 * cycle. A cycle already covered leaves the dates to the person.
 */
export function nextDraft(rows: readonly PeriodDraft[], cycle: CycleSpan, key: string): PeriodDraft {
  const index = rows.length;
  const name = periodName(index);
  const lastEnd = rows.reduce((max, r) => (r.endsOn > max ? r.endsOn : max), "");
  const startsOn = lastEnd === "" ? cycle.startsOn : addDays(lastEnd, 1);
  if (startsOn > cycle.endsOn) return { key, id: "", name, startsOn: "", endsOn: "" };
  return { key, id: "", name, startsOn, endsOn: proposedEnd(index, startsOn, cycle) };
}

/** The server's periods of one grade as editable rows, in ordinal order. */
export function draftsOf(periods: readonly EvaluationPeriod[]): PeriodDraft[] {
  return [...periods]
    .sort((a, b) => a.ordinal - b.ordinal)
    .map((p) => ({ key: p.id, id: p.id, name: p.name, startsOn: p.startsOn, endsOn: p.endsOn }));
}

/**
 * The rows as the server takes them: numbered 1…n in date order, so "2.º
 * trimestre" is always the second one on the calendar.
 */
export function toInputs(rows: readonly PeriodDraft[]): EvaluationPeriodInput[] {
  return [...rows]
    .sort((a, b) => a.startsOn.localeCompare(b.startsOn))
    .map((r, i) => ({
      ...(r.id === "" ? {} : { id: r.id }),
      ordinal: i + 1,
      name: r.name.trim(),
      startsOn: r.startsOn,
      endsOn: r.endsOn,
    }));
}
