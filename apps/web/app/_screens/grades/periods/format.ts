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

/**
 * "Se retirará 1 boleta ya publicada a un alumno que ya no tiene
 * calificaciones." — a re-close withdraws, with no replacement, the boleta
 * of a pupil left without grades; `verb` is [singular, plural]. Empty at 0.
 */
function withdrawnSentence(withdrawn: number, [one, many]: readonly [string, string]): string {
  if (withdrawn === 0) return "";
  return withdrawn === 1
    ? `${one} 1 boleta ya publicada a un alumno que ya no tiene calificaciones.`
    : `${many} ${withdrawn} boletas ya publicadas a alumnos que ya no tienen calificaciones.`;
}

function joinSentences(...parts: string[]): string {
  return parts.filter((p) => p !== "").join(" ");
}

/** "Se publicarán 24 boletas. 1 alumno sin calificaciones no recibirá boleta." (+ what it withdraws) — said before closing. */
export function closePreview(reportCards: number, skipped: number, withdrawn: number): string {
  const published =
    reportCards === 0 ? "No se publicará ninguna boleta." : reportCards === 1 ? "Se publicará 1 boleta." : `Se publicarán ${reportCards} boletas.`;
  return joinSentences(
    published,
    skipped > 0 ? skippedSentence(skipped, ["no recibirá", "no recibirán"]) : "",
    withdrawnSentence(withdrawn, ["Se retirará", "Se retirarán"]),
  );
}

/** "Se publicaron 24 boletas" (+ who got none, + what it withdrew) — said after closing. */
export function publishedMessage(result: Pick<CloseResult, "reportCards" | "skippedWithoutGrades" | "withdrawnReportCards">): string {
  const { reportCards, skippedWithoutGrades: skipped, withdrawnReportCards: withdrawn } = result;
  const published =
    reportCards === 0 ? "No se publicó ninguna boleta" : reportCards === 1 ? "Se publicó 1 boleta" : `Se publicaron ${reportCards} boletas`;
  const rest = joinSentences(
    skipped > 0 ? skippedSentence(skipped, ["no recibió", "no recibieron"]) : "",
    withdrawnSentence(withdrawn, ["Se retiró", "Se retiraron"]),
  );
  return rest === "" ? published : `${published}. ${rest}`;
}
