/**
 * The local calendar day as aaaa-mm-dd — the office's day, not UTC's. At 19:00
 * in Mexico City UTC is already tomorrow, and what is typed then belongs to
 * today (see the "school day is UTC day" note on finance).
 */
export function localISODate(at: Date = new Date()): string {
  const pad = (n: number): string => String(n).padStart(2, "0");
  return `${at.getFullYear()}-${pad(at.getMonth() + 1)}-${pad(at.getDate())}`;
}

function calendarDay(isoDate: string): Date {
  const [y, m, d] = isoDate.split("-").map(Number);
  return new Date(y ?? 0, (m ?? 1) - 1, d ?? 1);
}

/** "3 oct" for an aaaa-mm-dd date, read as the calendar day it names. */
export function shortDayLabel(isoDate: string): string {
  return calendarDay(isoDate).toLocaleDateString("es-MX", { day: "numeric", month: "short" }).replace(".", "");
}

/** "3 oct 2026" for an aaaa-mm-dd date, read as the calendar day it names. */
export function dayWithYearLabel(isoDate: string): string {
  return calendarDay(isoDate).toLocaleDateString("es-MX", { day: "numeric", month: "short", year: "numeric" }).replace(".", "");
}
