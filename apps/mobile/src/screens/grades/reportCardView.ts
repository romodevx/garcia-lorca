import type { ReportCardSummary } from "../../api/reportCards";
import { formatDate } from "../format";
import { formatGrade } from "../gradesView";

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

/** The average as stored (never rounded), or null when nothing was captured. */
export function averageText(card: Pick<ReportCardSummary, "generalAverage" | "hasGrades">): string | null {
  return card.hasGrades && card.generalAverage !== null ? formatGrade(card.generalAverage) : null;
}

/** What a screen reader says for one boleta row. */
export function reportCardLabel(card: ReportCardSummary): string {
  const parts = [[card.termName, card.cycleName].filter(Boolean).join(" ")];
  if (card.version > FIRST_VERSION) {
    parts.push(`versión ${card.version}`, "actualizada");
  }
  const average = averageText(card);
  parts.push(average === null ? "sin calificaciones" : `promedio general ${average}`);
  const issued = card.issuedAt ? formatDate(card.issuedAt, "long") : "";
  if (issued) {
    parts.push(`emitida el ${issued}`);
  }
  return parts.join(", ");
}
