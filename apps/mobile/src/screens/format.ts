import { money } from "../data/mock";

/** Formats an integer-cents amount as an es-MX peso string (e.g. 240000 → "$2,400.00"). */
export function pesos(cents: number): string {
  return money(cents / 100);
}

/**
 * es-MX day+month label from an ISO date or datetime (e.g. "2026-06-04" →
 * "4 jun"); empty string on an unparseable value. `style` picks the short or long
 * month name.
 */
export function formatDate(iso: string, style: "short" | "long" = "short"): string {
  const date = parseDate(iso);
  if (date === null) {
    return "";
  }
  return date.toLocaleDateString("es-MX", { day: "numeric", month: style });
}

/**
 * es-MX day, month and year from an ISO date (e.g. "2026-10-15" → "15 oct
 * 2026"); empty string on an unparseable value. The day the account is "as of".
 */
export function formatDayWithYear(iso: string): string {
  const date = parseDate(iso);
  if (date === null) {
    return "";
  }
  return date.toLocaleDateString("es-MX", { day: "numeric", month: "short", year: "numeric" }).replace(".", "");
}

/**
 * A balance in words (T5): below zero the school owes the family, so it reads
 * "Saldo a favor" with a positive amount — never a minus sign.
 */
export function balanceLine(cents: number): string {
  return cents < 0 ? `Saldo a favor ${pesos(-cents)}` : `Saldo pendiente ${pesos(cents)}`;
}

/**
 * Parses an ISO value into a local Date. A date-only "YYYY-MM-DD" is read as a
 * LOCAL calendar day (not UTC midnight), so the displayed day never shifts by a
 * timezone offset. Returns null when the value is unparseable.
 */
function parseDate(iso: string): Date | null {
  const dateOnly = /^(\d{4})-(\d{2})-(\d{2})$/.exec(iso);
  const date = dateOnly
    ? new Date(Number(dateOnly[1]), Number(dateOnly[2]) - 1, Number(dateOnly[3]))
    : new Date(iso);
  return Number.isNaN(date.getTime()) ? null : date;
}
