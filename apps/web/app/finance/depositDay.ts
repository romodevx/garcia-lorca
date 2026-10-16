import { schoolISODate } from "../_lib/schoolDay";

/**
 * The day a deposit (or a correction snapshot) is dated: its school-local day
 * (paidOn, ADR 0035). A snapshot written before migration 0028 has none; its
 * paidAt is read by the migration's own rule: exactly midnight UTC is a day
 * picked at the counter (its UTC date), any other instant was live and is
 * dated on the school's day.
 *
 * Its own module, not api.ts: screens whose tests mock the API still get the
 * real rule.
 */
export function depositDay(d: { paidAt?: string; paidOn?: string }): string {
  if (d.paidOn !== undefined && d.paidOn !== "") return d.paidOn;
  return dayOfPaidAt(d.paidAt ?? "");
}

function dayOfPaidAt(paidAt: string): string {
  const at = new Date(paidAt);
  if (Number.isNaN(at.getTime())) return paidAt.slice(0, 10);
  return isMidnightUTC(at) ? at.toISOString().slice(0, 10) : schoolISODate(at);
}

function isMidnightUTC(at: Date): boolean {
  return at.getUTCHours() === 0 && at.getUTCMinutes() === 0 && at.getUTCSeconds() === 0 && at.getUTCMilliseconds() === 0;
}
