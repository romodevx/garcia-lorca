import type { ReportCard, ReportCardLine, ReportCardSummary } from "../../api/reportCards";
import { formatDate } from "../format";
import { formatAverage, monthName, shortMonthLabel } from "../gradesView";

/** Pure presentation helpers for the Boletas list. */

const FIRST_VERSION = 1;

/** "2.º trimestre · 2026-2027", or the term alone when the cycle is unknown. */
export function reportCardTitle(card: Pick<ReportCardSummary, "termName" | "cycleName">): string {
  return card.cycleName ? `${card.termName} · ${card.cycleName}` : card.termName;
}

/** "Versión 2 · Actualizada" for a corrected boleta; null for the first issue. */
export function versionBadge(version: number): string | null {
  return version > FIRST_VERSION ? `Versión ${version} · Actualizada` : null;
}

/** The average as the server sent it (already truncated), or null when nothing was captured. */
export function averageText(card: Pick<ReportCardSummary, "generalAverage" | "hasGrades"> & { gradingLetters?: boolean }): string | null {
  if (card.gradingLetters === true) return LETTERS_CARD;
  return card.hasGrades && card.generalAverage !== null ? formatAverage(card.generalAverage) : null;
}

/** What a letters boleta (preescolar) says where a numeric one says its average. */
export const LETTERS_CARD = "Con letras";

/** One column of the boleta table: what it shows, what is read aloud, and which grades fill it. */
export interface ReportCardColumn {
  key: string;
  header: string;
  spoken: string;
  valueOf: (line: ReportCardLine) => number | undefined;
  /** The column's letter on a letters boleta. */
  letterOf: (line: ReportCardLine) => string | undefined;
}

/**
 * The boleta's columns: its evaluation periods (T15), or — on a card issued
 * before T15, whose `periods` is empty — its months, exactly as before.
 */
export function reportCardColumns(card: Pick<ReportCard, "periods" | "months">): ReportCardColumn[] {
  if (card.periods.length > 0) {
    return card.periods.map((p) => ({ key: p.id, header: p.name, spoken: p.name, valueOf: (line) => line.periods[p.id], letterOf: (line) => line.letters[p.id] }));
  }
  return card.months.map((m) => ({ key: m, header: shortMonthLabel(m), spoken: monthName(m), valueOf: (line) => line.months[m], letterOf: () => undefined }));
}

/** What a screen reader says for one boleta row. */
export function reportCardLabel(card: ReportCardSummary): string {
  const parts = [[card.termName, card.cycleName].filter(Boolean).join(" ")];
  if (card.version > FIRST_VERSION) {
    parts.push(`versión ${card.version}`, "actualizada");
  }
  const average = averageText(card);
  if (card.gradingLetters) parts.push("calificada con letras");
  else parts.push(average === null ? "sin calificaciones" : `promedio general ${average}`);
  const issued = card.issuedAt ? formatDate(card.issuedAt, "long") : "";
  if (issued) {
    parts.push(`emitida el ${issued}`);
  }
  return parts.join(", ");
}
