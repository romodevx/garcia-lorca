import { dayWithYearLabel } from "../../../_lib/localDate";
import type { GradingPeriod } from "../../../grades/periods";

const SCHOOL_TIME_ZONE = "America/Mexico_City";
const DAY = new Intl.DateTimeFormat("es-MX", { day: "numeric", month: "short", year: "numeric", timeZone: SCHOOL_TIME_ZONE });
const NO_DATE = "—";

/** "PRIMARIA · 1.er trimestre" — how a period is named everywhere on this tab. */
export function periodTitle(p: Pick<GradingPeriod, "levelName" | "termName">): string {
  return `${p.levelName} · ${p.termName}`;
}

/** "24 ago 2026 – 30 nov 2026"; undefined when either day is missing. */
export function dateRangeLabel(startsOn: string | null, endsOn: string | null): string | undefined {
  if (startsOn === null || endsOn === null) return undefined;
  return `${dayWithYearLabel(startsOn)} – ${dayWithYearLabel(endsOn)}`;
}

/** "1 dic 2026", on the school's calendar; "—" when there is no date. */
export function dayLabel(iso: string | null | undefined): string {
  if (iso === null || iso === undefined || iso === "") return NO_DATE;
  return DAY.format(new Date(iso));
}

/** "412 de 450 capturadas". */
export function progressLabel(captured: number, expected: number): string {
  return `${captured} de ${expected} capturadas`;
}
