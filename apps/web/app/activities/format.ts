/**
 * How an activity reads on screen (T17): its week, its periodicity and its
 * price options. Pure, so the table, the form and the enrolment page say it
 * the same way.
 */
import { formatCents } from "../_lib/money";
import type { ActivityPriceOption, ActivitySlot, Periodicity } from "./api";

/** Index = weekday (1 = Monday … 7 = Sunday). */
export const WEEKDAY_NAMES = ["", "Lunes", "Martes", "Miércoles", "Jueves", "Viernes", "Sábado", "Domingo"] as const;
const WEEKDAY_SHORT = ["", "Lun", "Mar", "Mié", "Jue", "Vie", "Sáb", "Dom"] as const;

export const PERIODICITY_LABEL: Readonly<Record<Periodicity, string>> = {
  monthly: "Mensual",
  weekly: "Semanal",
  per_cycle: "Una vez por ciclo",
};

/** "Lun, Mié y Vie" */
function joinDays(days: readonly string[]): string {
  if (days.length <= 1) return days.join("");
  return `${days.slice(0, -1).join(", ")} y ${days[days.length - 1] ?? ""}`;
}

/**
 * "Mar y Jue 15:00–16:30 · Sáb 09:00–11:00": the days that share hours go
 * together, in week order.
 */
export function scheduleSummary(slots: readonly ActivitySlot[]): string {
  if (slots.length === 0) return "Sin horario";
  const sorted = [...slots].sort((a, b) => a.weekday - b.weekday || a.start.localeCompare(b.start));
  const byHours = new Map<string, number[]>();
  for (const s of sorted) {
    const hours = `${s.start}–${s.end}`;
    byHours.set(hours, [...(byHours.get(hours) ?? []), s.weekday]);
  }
  return [...byHours]
    .map(([hours, days]) => `${joinDays(days.map((d) => WEEKDAY_SHORT[d] ?? "?"))} ${hours}`)
    .join(" · ");
}

function daysText(n: number): string {
  return n === 1 ? "1 día" : `${n} días`;
}

/** "1 día/semana — $600.00" or "Cuota fija — $900.00". */
export function optionLabel(o: Pick<ActivityPriceOption, "daysPerWeek" | "amountCents">): string {
  const what = o.daysPerWeek === null ? "Cuota fija" : `${daysText(o.daysPerWeek)}/semana`;
  return `${what} — ${formatCents(o.amountCents)}`;
}

/** Fewest days first, the flat fee last. */
export function sortOptions<T extends Pick<ActivityPriceOption, "daysPerWeek">>(options: readonly T[]): T[] {
  const rank = (o: T): number => o.daysPerWeek ?? Number.MAX_SAFE_INTEGER;
  return [...options].sort((a, b) => rank(a) - rank(b));
}

/** "1 día $600.00 · 2 días $700.00 · Fija $900.00" for a table cell. */
export function optionsSummary(options: readonly ActivityPriceOption[]): string {
  return sortOptions(options)
    .map((o) => `${o.daysPerWeek === null ? "Fija" : daysText(o.daysPerWeek)} ${formatCents(o.amountCents)}`)
    .join(" · ");
}
