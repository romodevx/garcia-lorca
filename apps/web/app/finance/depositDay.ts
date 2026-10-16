/**
 * The day a deposit (or a correction snapshot) is dated: its school-local day
 * (paidOn, ADR 0035), or — from a server that predates it — the stored date of
 * paidAt, which was then midnight UTC of the day picked.
 *
 * Its own module, not api.ts: screens whose tests mock the API still get the
 * real rule.
 */
export function depositDay(d: { paidAt?: string; paidOn?: string }): string {
  if (d.paidOn !== undefined && d.paidOn !== "") return d.paidOn;
  return (d.paidAt ?? "").slice(0, 10);
}
