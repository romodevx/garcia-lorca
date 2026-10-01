/**
 * The month the Egresos screen looks at, as aaaa-mm-dd strings — calendar
 * days, never instants, so no time zone can move them.
 */

const MONTHS = ["enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"];

const pad = (n: number): string => String(n).padStart(2, "0");

function parts(day: string): [number, number] {
  const [y, m] = day.split("-").map(Number);
  return [y ?? 1970, m ?? 1];
}

/** The first and last day of the month that holds `day`. */
export function monthRange(day: string): { from: string; to: string } {
  const [y, m] = parts(day);
  const last = new Date(Date.UTC(y, m, 0)).getUTCDate();
  return { from: `${y}-${pad(m)}-01`, to: `${y}-${pad(m)}-${pad(last)}` };
}

/** The first day of the month `by` months away from the one that holds `day`. */
export function shiftMonth(day: string, by: number): string {
  const [y, m] = parts(day);
  const shifted = new Date(Date.UTC(y, m - 1 + by, 1));
  return `${shifted.getUTCFullYear()}-${pad(shifted.getUTCMonth() + 1)}-01`;
}

/** "Octubre de 2026". */
export function monthLabel(day: string): string {
  const [y, m] = parts(day);
  const name = MONTHS[m - 1] ?? "";
  return `${name.charAt(0).toUpperCase()}${name.slice(1)} de ${y}`;
}
