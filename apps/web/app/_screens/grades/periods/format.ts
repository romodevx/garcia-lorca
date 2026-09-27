import { monthChips } from "../../../grades/monthly";
import type { GradingPeriod } from "../../../grades/periods";

const SCHOOL_TIME_ZONE = "America/Mexico_City";
const DAY = new Intl.DateTimeFormat("es-MX", { day: "numeric", month: "short", year: "numeric", timeZone: SCHOOL_TIME_ZONE });
const MONTH_PARTS = new Intl.DateTimeFormat("en-CA", { year: "numeric", month: "2-digit", timeZone: SCHOOL_TIME_ZONE });
const NO_DATE = "—";

/** "PRIMARIA · 1.er trimestre" — how a period is named everywhere on this tab. */
export function periodTitle(p: Pick<GradingPeriod, "levelName" | "termName">): string {
  return `${p.levelName} · ${p.termName}`;
}

/** "Sep · Oct · Nov". */
export function monthsLabel(months: readonly string[]): string {
  return monthChips(months).map((c) => c.label).join(" · ");
}

/** "Abr, May, Jun". */
export function monthsList(months: readonly string[]): string {
  return monthChips(months).map((c) => c.label).join(", ");
}

/** "1 dic 2026", on the school's calendar; "—" when there is no date. */
export function dayLabel(iso: string | null | undefined): string {
  if (iso === null || iso === undefined || iso === "") return NO_DATE;
  return DAY.format(new Date(iso));
}

/** Today's month on the school's calendar, "YYYY-MM". */
export function schoolMonth(now: Date): string {
  const parts = MONTH_PARTS.formatToParts(now);
  const part = (type: string): string => parts.find((p) => p.type === type)?.value ?? "";
  return `${part("year")}-${part("month")}`;
}

/**
 * The term's months that have not finished: the server's list when it sent
 * one, else the months after `currentMonth` ("YYYY-MM" strings order like the
 * months they name).
 */
export function unfinishedMonths(months: readonly string[], fromServer: readonly string[] | undefined, currentMonth: string): string[] {
  if (fromServer !== undefined && fromServer.length > 0) return [...fromServer];
  return months.filter((m) => m > currentMonth);
}

/** "412 de 450 capturadas". */
export function progressLabel(captured: number, expected: number): string {
  return `${captured} de ${expected} capturadas`;
}
