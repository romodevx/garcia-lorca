import { monthChips } from "../../../grades/monthly";
import type { GradingPeriod } from "../../../grades/periods";

const SCHOOL_TIME_ZONE = "America/Mexico_City";
const DAY = new Intl.DateTimeFormat("es-MX", { day: "numeric", month: "short", year: "numeric", timeZone: SCHOOL_TIME_ZONE });

/** "PRIMARIA · 1.er trimestre" — how a period is named everywhere on this tab. */
export function periodTitle(p: Pick<GradingPeriod, "levelName" | "termName">): string {
  return `${p.levelName} · ${p.termName}`;
}

/** "Sep · Oct · Nov". */
export function monthsLabel(months: readonly string[]): string {
  return monthChips(months).map((c) => c.label).join(" · ");
}

/** "1 dic 2026", on the school's calendar. */
export function dayLabel(iso: string): string {
  return DAY.format(new Date(iso));
}

/** "412 de 450 capturadas". */
export function progressLabel(captured: number, expected: number): string {
  return `${captured} de ${expected} capturadas`;
}
