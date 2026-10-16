import { schoolISODate } from "./schoolDay";

/**
 * The office's calendar day as aaaa-mm-dd — the school's day (Morelia), not
 * UTC's nor the device's. At 19:00 in Mexico City UTC is already tomorrow, and
 * what is typed then belongs to today (ADR 0035). One rule: schoolISODate.
 */
export function localISODate(at: Date = new Date()): string {
  return schoolISODate(at);
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
