import { dayWithYearLabel } from "../../../_lib/localDate";
import type { CloseResult, GradingPeriod } from "../../../grades/periods";

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

/** "1 alumno sin calificaciones no recibirá boleta." — `verb` is [singular, plural]. */
function skippedSentence(skipped: number, [one, many]: readonly [string, string]): string {
  const pupils = skipped === 1 ? "1 alumno" : `${skipped} alumnos`;
  return `${pupils} sin calificaciones ${skipped === 1 ? one : many} boleta.`;
}

/** "Se publicarán 24 boletas. 1 alumno sin calificaciones no recibirá boleta." — said before closing. */
export function closePreview(reportCards: number, skipped: number): string {
  const published =
    reportCards === 0 ? "No se publicará ninguna boleta." : reportCards === 1 ? "Se publicará 1 boleta." : `Se publicarán ${reportCards} boletas.`;
  return skipped > 0 ? `${published} ${skippedSentence(skipped, ["no recibirá", "no recibirán"])}` : published;
}

/** "Se publicaron 24 boletas" (+ who got none) — said after closing. */
export function publishedMessage(result: Pick<CloseResult, "reportCards" | "skippedWithoutGrades">): string {
  const { reportCards, skippedWithoutGrades: skipped } = result;
  const published =
    reportCards === 0 ? "No se publicó ninguna boleta" : reportCards === 1 ? "Se publicó 1 boleta" : `Se publicaron ${reportCards} boletas`;
  return skipped > 0 ? `${published}. ${skippedSentence(skipped, ["no recibió", "no recibieron"])}` : published;
}
