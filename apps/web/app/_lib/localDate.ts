/**
 * The local calendar day as aaaa-mm-dd — the office's day, not UTC's. At 19:00
 * in Mexico City UTC is already tomorrow, and what is typed then belongs to
 * today (see the "school day is UTC day" note on finance).
 */
export function localISODate(at: Date = new Date()): string {
  const pad = (n: number): string => String(n).padStart(2, "0");
  return `${at.getFullYear()}-${pad(at.getMonth() + 1)}-${pad(at.getDate())}`;
}
