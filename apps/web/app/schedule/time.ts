/**
 * The arithmetic of the timetable (T16), pure and DOM-free: "HH:MM" ↔ minutes,
 * the 5-minute rows of the week grid, the overlap columns, the subject's color
 * and the school's "now". Every screen of Horarios draws from here.
 */
import type { ColorKey } from "../_design/colors";

/** The grid's resolution: a class starts and ends on a 5-minute step. */
export const SLOT_MINUTES = 5;
/** What a new class lasts unless the grid ends first. */
export const DEFAULT_CLASS_MINUTES = 50;
/** The grid a level shows until someone sets its own (contract default). */
export const DEFAULT_GRID = { gridStart: "07:00", gridEnd: "15:00" } as const;
/** The school's clock. Never the browser's: a laptop set to UTC must not move "now". */
export const SCHOOL_TIME_ZONE = "America/Mexico_City";

const MINUTES_PER_HOUR = 60;
const MINUTES_PER_DAY = 24 * MINUTES_PER_HOUR;
const HHMM = /^([01]\d|2[0-4]):([0-5]\d)$/;

/** "08:05" → 485. Anything that is not a wire time is NaN. */
export function toMinutes(hhmm: string): number {
  const m = HHMM.exec(hhmm);
  if (m === null) return Number.NaN;
  const minutes = Number(m[1]) * MINUTES_PER_HOUR + Number(m[2]);
  return minutes > MINUTES_PER_DAY ? Number.NaN : minutes;
}

/** 485 → "08:05". */
export function toHHMM(minutes: number): string {
  const pad = (n: number): string => String(n).padStart(2, "0");
  return `${pad(Math.floor(minutes / MINUTES_PER_HOUR))}:${pad(minutes % MINUTES_PER_HOUR)}`;
}

/** The 1-based 5-minute row a minute falls on, counted from the grid's start. */
export function rowOf(minute: number, gridStart: number): number {
  return Math.floor((minute - gridStart) / SLOT_MINUTES) + 1;
}

/** Down to the 5-minute step it falls in. */
export function snapToSlot(minute: number): number {
  return Math.floor(minute / SLOT_MINUTES) * SLOT_MINUTES;
}

/**
 * The minutes the week grid spans: the level's grid (or the default),
 * widened to whole hours so a class outside it is still drawn.
 */
export function gridBounds(
  grid: { gridStart: string; gridEnd: string } | undefined,
  blocks: readonly { startsAt: string; endsAt: string }[],
): { start: number; end: number } {
  let start = toMinutes((grid ?? DEFAULT_GRID).gridStart);
  let end = toMinutes((grid ?? DEFAULT_GRID).gridEnd);
  for (const b of blocks) {
    const s = toMinutes(b.startsAt);
    const e = toMinutes(b.endsAt);
    if (s < start) start = Math.floor(s / MINUTES_PER_HOUR) * MINUTES_PER_HOUR;
    if (e > end) end = Math.ceil(e / MINUTES_PER_HOUR) * MINUTES_PER_HOUR;
  }
  return { start, end };
}

/** A new class's end: 50 minutes after it starts, clipped to the grid. */
export function defaultEnd(start: number, gridEnd: number): number {
  return Math.min(start + DEFAULT_CLASS_MINUTES, gridEnd);
}

/** Every 5-minute step from `from` to `to`, both included, as "HH:MM". */
export function timeOptions(from: number, to: number): string[] {
  const out: string[] = [];
  for (let m = from; m <= to; m += SLOT_MINUTES) out.push(toHHMM(m));
  return out;
}

export interface Placed {
  /** 0-based column inside its overlap cluster. */
  column: number;
  /** How many columns the cluster needs: the block's width is 1/columns. */
  columns: number;
}

/**
 * The classic calendar layout: items that overlap in time sit side by side.
 * Sorted by start (longest first on a tie), items are grouped into clusters
 * of transitively overlapping ones; inside a cluster each takes the first
 * column whose last item has ended (ends are exclusive), and every member is
 * as wide as the cluster's column count. O(n log n + n·k), k = columns.
 */
export function layoutColumns(items: readonly { id: string; start: number; end: number }[]): Map<string, Placed> {
  const sorted = [...items].sort((a, b) => a.start - b.start || b.end - a.end);
  const out = new Map<string, Placed>();
  let cluster: { id: string; column: number }[] = [];
  let columnEnds: number[] = [];
  let clusterEnd = -Infinity;

  const close = (): void => {
    for (const c of cluster) out.set(c.id, { column: c.column, columns: columnEnds.length });
    cluster = [];
    columnEnds = [];
    clusterEnd = -Infinity;
  };

  for (const item of sorted) {
    if (item.start >= clusterEnd) close();
    let column = columnEnds.findIndex((end) => end <= item.start);
    if (column === -1) {
      column = columnEnds.length;
      columnEnds.push(item.end);
    } else {
      columnEnds[column] = item.end;
    }
    cluster.push({ id: item.id, column });
    clusterEnd = Math.max(clusterEnd, item.end);
  }
  close();
  return out;
}

/** The design system's palette, minus the neutral grey. */
const SUBJECT_PALETTE: readonly ColorKey[] = ["forest", "purple", "orange", "red", "lime", "yellow"];

/** A subject keeps its color everywhere: a hash of its name (FNV-1a) picks it. */
export function subjectColor(name: string): ColorKey {
  let hash = 0x811c9dc5;
  for (const ch of name.trim().toLocaleLowerCase("es-MX")) {
    hash ^= ch.codePointAt(0) ?? 0;
    hash = Math.imul(hash, 0x01000193) >>> 0;
  }
  return SUBJECT_PALETTE[hash % SUBJECT_PALETTE.length] ?? "forest";
}

const WEEKDAY_OF_SHORT: Record<string, number> = { Mon: 1, Tue: 2, Wed: 3, Thu: 4, Fri: 5, Sat: 6, Sun: 7 };

/** The date, "HH:MM" and ISO weekday (1 = Monday) at the school right now. */
export function nowInSchoolZone(at: Date = new Date()): { date: string; time: string; weekday: number } {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone: SCHOOL_TIME_ZONE,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
    weekday: "short",
  }).formatToParts(at);
  const part = (type: Intl.DateTimeFormatPartTypes): string => parts.find((p) => p.type === type)?.value ?? "";
  return {
    date: `${part("year")}-${part("month")}-${part("day")}`,
    time: `${part("hour")}:${part("minute")}`,
    weekday: WEEKDAY_OF_SHORT[part("weekday")] ?? 1,
  };
}

const DAYS_PER_WEEK = 7;
const MS_PER_DAY = 86_400_000;

function utcDay(iso: string): number {
  const [y, m, d] = iso.split("-").map(Number);
  return Date.UTC(y ?? 0, (m ?? 1) - 1, d ?? 1);
}

/** ISO weekday of a calendar date: 1 = Monday … 7 = Sunday. */
export function weekdayOf(iso: string): number {
  const day = new Date(utcDay(iso)).getUTCDay();
  return day === 0 ? DAYS_PER_WEEK : day;
}

/** The first date on or after `today` that falls on `weekday`. */
export function nextDateOnWeekday(today: string, weekday: number): string {
  const ahead = (weekday - weekdayOf(today) + DAYS_PER_WEEK) % DAYS_PER_WEEK;
  return new Date(utcDay(today) + ahead * MS_PER_DAY).toISOString().slice(0, 10);
}

/** The classes under way at `time` ("HH:MM"): started, not yet ended. */
export function runningAt<T extends { startsAt: string; endsAt: string }>(classes: readonly T[], time: string): T[] {
  const t = toMinutes(time);
  return classes.filter((c) => toMinutes(c.startsAt) <= t && t < toMinutes(c.endsAt));
}

// --- weeks (T19) ----------------------------------------------------------------

const MONTHS = ["enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"];
const DAY_NAMES = ["", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado", "domingo"];
const SCHOOL_DAYS = 5;

/** `iso` moved by `days` (negative goes back). */
export function addDays(iso: string, days: number): string {
  return new Date(utcDay(iso) + days * MS_PER_DAY).toISOString().slice(0, 10);
}

/** The Monday of `iso`'s week. */
export function mondayOf(iso: string): string {
  return addDays(iso, 1 - weekdayOf(iso));
}

/** The Friday of the school week starting on `monday`. */
export function fridayOf(monday: string): string {
  return addDays(monday, SCHOOL_DAYS - 1);
}

function dayAndMonth(iso: string): { day: number; month: string } {
  const [, m, d] = iso.split("-").map(Number);
  return { day: d ?? 1, month: MONTHS[(m ?? 1) - 1] ?? "" };
}

/** "Semana del 5 al 9 de octubre" (the month twice when the week crosses it). */
export function weekLabel(monday: string): string {
  const from = dayAndMonth(monday);
  const to = dayAndMonth(fridayOf(monday));
  return from.month === to.month
    ? `Semana del ${from.day} al ${to.day} de ${to.month}`
    : `Semana del ${from.day} de ${from.month} al ${to.day} de ${to.month}`;
}

/** "lunes 5 de octubre". */
export function spanishDate(iso: string): string {
  const { day, month } = dayAndMonth(iso);
  return `${DAY_NAMES[weekdayOf(iso)] ?? ""} ${day} de ${month}`;
}
