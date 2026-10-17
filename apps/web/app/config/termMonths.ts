/**
 * Which months of a cycle each term (trimester) of a level grades. Pure rules,
 * no I/O: the Configuración grid and its list alternative both apply them, and
 * the grid's optimistic save follows the plan `savePlan` returns.
 *
 * Months are "YYYY-MM" strings, which order like the months they name.
 */

import type { CreateTermInput, Cycle } from "./api";

/** termId → its months, in calendar order. */
export type MonthAssignment = Readonly<Record<string, readonly string[]>>;

const MONTHS_PER_YEAR = 12;
const TERMS_PER_CYCLE = 3;
const MS_PER_DAY = 86_400_000;

/** What a trimester is called, by its position in the cycle. */
export const TRIMESTER_NAMES = ["1.er trimestre", "2.º trimestre", "3.er trimestre"] as const;

function monthKey(year: number, monthIndex: number): string {
  return `${year}-${String(monthIndex + 1).padStart(2, "0")}`;
}

/** Every month from the cycle's start date to its end date, both included. */
export function cycleMonths(startsOn: string, endsOn: string): string[] {
  const [startYear = 0, startMonth = 1] = startsOn.split("-").map(Number);
  const last = endsOn.slice(0, 7);
  const months: string[] = [];
  for (let i = 0; ; i++) {
    const index = startMonth - 1 + i;
    const key = monthKey(startYear + Math.floor(index / MONTHS_PER_YEAR), index % MONTHS_PER_YEAR);
    if (key > last) return months;
    months.push(key);
  }
}

/**
 * One tap on (term, month): a free month joins the term, a month held by
 * another term moves over, and a month the term already has leaves it.
 */
export function assignMonth(current: MonthAssignment, termId: string, month: string): MonthAssignment {
  const alreadyHere = current[termId]?.includes(month) === true;
  const next: Record<string, readonly string[]> = {};
  for (const [id, months] of Object.entries(current)) {
    next[id] = months.filter((m) => m !== month);
  }
  if (!alreadyHere) next[termId] = [...(next[termId] ?? []), month].sort();
  return next;
}

export interface TermMonthsSave {
  termId: string;
  months: string[];
}

const sameMonths = (a: readonly string[], b: readonly string[]): boolean =>
  a.length === b.length && a.every((m, i) => m === b[i]);

/**
 * The PUTs that turn `before` into `after`. Terms that only LOST months go
 * first: the server refuses a month still held by another term.
 */
export function savePlan(before: MonthAssignment, after: MonthAssignment): TermMonthsSave[] {
  const changed = Object.keys(after).filter((id) => !sameMonths(before[id] ?? [], after[id] ?? []));
  const gains = (id: string): boolean => (after[id] ?? []).some((m) => !(before[id] ?? []).includes(m));
  const ordered = [...changed.filter((id) => !gains(id)), ...changed.filter(gains)];
  return ordered.map((termId) => ({ termId, months: [...(after[termId] ?? [])] }));
}

/** The cycle's months no term grades yet. */
export function unassignedMonths(months: readonly string[], assignment: MonthAssignment): string[] {
  const taken = new Set(Object.values(assignment).flat());
  return months.filter((m) => !taken.has(m));
}

function addDays(isoDate: string, days: number): string {
  return new Date(Date.parse(`${isoDate}T00:00:00Z`) + days * MS_PER_DAY).toISOString().slice(0, 10);
}

/** A stretch of days, both ends included. */
export interface DaySpan {
  startsOn: string;
  endsOn: string;
}

/** The cycle cut in three contiguous thirds; the last ends on the cycle's last day. */
export function trimesterSpans(cycle: DaySpan): DaySpan[] {
  const totalDays = Math.round((Date.parse(cycle.endsOn) - Date.parse(cycle.startsOn)) / MS_PER_DAY) + 1;
  const boundary = (k: number): number => Math.round((totalDays * k) / TERMS_PER_CYCLE);
  return TRIMESTER_NAMES.map((_, k) => ({
    startsOn: addDays(cycle.startsOn, boundary(k)),
    endsOn: addDays(cycle.startsOn, boundary(k + 1) - 1),
  }));
}

/**
 * Three trimesters splitting the cycle in contiguous thirds — the quick start
 * for a level with no terms. The months are assigned afterwards, in the grid.
 */
export function trimesterInputs(cycle: Pick<Cycle, "id" | "startsOn" | "endsOn">, levelId: string): CreateTermInput[] {
  const spans = trimesterSpans(cycle);
  return TRIMESTER_NAMES.map((name, k) => ({
    cycleId: cycle.id,
    levelId,
    name,
    ordinal: k + 1,
    startsOn: spans[k]?.startsOn ?? cycle.startsOn,
    endsOn: spans[k]?.endsOn ?? cycle.endsOn,
  }));
}
