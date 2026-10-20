import { apiFetch } from "./client";
import { ApiError } from "./errors";
import { byOrdinal, normalizeScale, type GradeScale } from "./grades";

/**
 * A guardian's view of one child's issued trimester report cards (boletas):
 * `GET /api/v1/family/students/{id}/report-cards` (latest version per term,
 * newest term first), one card's JSON, and its PDF path. The gateway scopes
 * every read to the caller's own children: any other id answers 404.
 */

/** One issued boleta in the list. */
export interface ReportCardSummary {
  id: string;
  termId: string;
  termName: string;
  /** The term's order in its cycle (1, 2, 3…); 0 for a card issued before terms carried one. */
  termOrdinal: number;
  /** The cycle's name ("2026-2027"); empty only when the server omits it. */
  cycleName: string;
  version: number;
  /** RFC 3339, or null when not stamped. */
  issuedAt: string | null;
  /** Null when no subject has a grade. */
  generalAverage: number | null;
  /** False when the term closed with nothing captured. */
  hasGrades: boolean;
  /** The boleta carries letters (preescolar): no average. False from an older gateway. */
  gradingLetters: boolean;
}

/** One column of a T15 boleta: an evaluation period of the pupil's grade. */
export interface ReportCardPeriod {
  id: string;
  ordinal: number;
  name: string;
}

/**
 * One subject row of a boleta: its grade per column and the term average. A
 * T15 card fills `periods` (period id → grade); a card issued before T15 keeps
 * its month columns in `months` ("YYYY-MM" → grade).
 */
export interface ReportCardLine {
  /** Stable subject id; empty only when the server omits it. */
  subjectId: string;
  subjectName: string;
  periods: Record<string, number>;
  /** Period id → letter on a letters boleta; {} otherwise. */
  letters: Record<string, string>;
  months: Record<string, number>;
  average: number | null;
}

export interface ReportCard {
  id: string;
  version: number;
  issuedAt: string | null;
  termName: string;
  /** See {@link ReportCardSummary.termOrdinal}. */
  termOrdinal: number;
  cycleName: string;
  /** The card's evaluation-period columns, by ordinal; [] on a card issued before T15. */
  periods: ReportCardPeriod[];
  /** The columns of a card issued before T15 (monthly capture), oldest first; [] since T15. */
  months: string[];
  lines: ReportCardLine[];
  generalAverage: number | null;
  scale: GradeScale | null;
  /** False when the term closed with nothing captured. */
  hasGrades: boolean;
  /** Lines carry letters, there are no averages (preescolar). */
  gradingLetters: boolean;
}

/** The gateway emits exact decimals as JSON numbers; a string is tolerated. */
type WireNumber = number | string | null | undefined;

interface WireSummary {
  id?: string;
  termId?: string;
  termName?: string;
  termOrdinal?: number;
  cycleName?: string;
  version?: number;
  issuedAt?: string | null;
  generalAverage?: WireNumber;
  hasGrades?: boolean;
  gradingLetters?: boolean;
}

interface WireLine {
  subjectId?: string;
  subjectName?: string;
  periods?: Record<string, WireNumber> | null;
  letters?: Record<string, string> | null;
  months?: Record<string, WireNumber> | null;
  average?: WireNumber;
}

interface WireReportCard {
  id?: string;
  version?: number;
  issuedAt?: string | null;
  termName?: string;
  termOrdinal?: number;
  cycleName?: string;
  periods?: ReportCardPeriod[] | null;
  months?: string[] | null;
  lines?: WireLine[] | null;
  generalAverage?: WireNumber;
  scale?: Partial<GradeScale> | null;
  hasGrades?: boolean;
  gradingLetters?: boolean;
}

const FIRST_VERSION = 1;
/** A card issued before terms carried an ordinal. */
const NO_ORDINAL = 0;

function toNumber(value: WireNumber): number | null {
  if (value === null || value === undefined || value === "") return null;
  const n = Number(value);
  return Number.isFinite(n) ? n : null;
}

function toValues(byColumn: Record<string, WireNumber> | null | undefined): Record<string, number> {
  const out: Record<string, number> = {};
  for (const [column, raw] of Object.entries(byColumn ?? {})) {
    const value = toNumber(raw);
    if (value !== null) out[column] = value;
  }
  return out;
}

function studentPath(studentId: string): string {
  return `/api/v1/family/students/${encodeURIComponent(studentId)}/report-cards`;
}

function reportCardPath(studentId: string, reportCardId: string): string {
  return `${studentPath(studentId)}/${encodeURIComponent(reportCardId)}`;
}

/** The authenticated path of a boleta's PDF. */
export function reportCardPdfPath(studentId: string, reportCardId: string): string {
  return `${reportCardPath(studentId, reportCardId)}/pdf`;
}

/** Lists the child's issued boletas. Throws {@link ApiError} on non-2xx. */
export async function listReportCards(studentId: string): Promise<ReportCardSummary[]> {
  const res = await apiFetch(studentPath(studentId));
  if (!res.ok) {
    throw new ApiError(res.status, "failed to load report cards");
  }
  const data = (await res.json()) as { reportCards?: WireSummary[] | null };
  return (data.reportCards ?? []).map((c) => ({
    id: c.id ?? "",
    termId: c.termId ?? "",
    termName: c.termName ?? "",
    termOrdinal: c.termOrdinal ?? NO_ORDINAL,
    cycleName: c.cycleName ?? "",
    version: c.version ?? FIRST_VERSION,
    issuedAt: c.issuedAt ?? null,
    generalAverage: toNumber(c.generalAverage),
    hasGrades: c.hasGrades ?? toNumber(c.generalAverage) !== null,
    gradingLetters: c.gradingLetters ?? false,
  }));
}

/** Reads one boleta as JSON. Throws {@link ApiError} on non-2xx. */
export async function getReportCard(studentId: string, reportCardId: string): Promise<ReportCard> {
  const res = await apiFetch(reportCardPath(studentId, reportCardId));
  if (!res.ok) {
    throw new ApiError(res.status, "failed to load report card");
  }
  const data = (await res.json()) as WireReportCard;
  return {
    id: data.id ?? reportCardId,
    version: data.version ?? FIRST_VERSION,
    issuedAt: data.issuedAt ?? null,
    termName: data.termName ?? "",
    termOrdinal: data.termOrdinal ?? NO_ORDINAL,
    cycleName: data.cycleName ?? "",
    periods: byOrdinal(data.periods ?? []),
    months: data.months ?? [],
    lines: (data.lines ?? []).map((l) => ({
      subjectId: l.subjectId ?? "",
      subjectName: l.subjectName ?? "",
      periods: toValues(l.periods),
      letters: l.letters ?? {},
      months: toValues(l.months),
      average: toNumber(l.average),
    })),
    generalAverage: toNumber(data.generalAverage),
    scale: normalizeScale(data.scale),
    hasGrades: data.hasGrades ?? false,
    gradingLetters: data.gradingLetters ?? false,
  };
}
