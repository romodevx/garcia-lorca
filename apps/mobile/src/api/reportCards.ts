import { apiFetch } from "./client";
import { ApiError } from "./errors";
import { normalizeScale, type GradeScale } from "./grades";

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
  /** Empty when the server does not send it (the family list omits it today). */
  cycleName: string;
  version: number;
  /** RFC 3339, or null when not stamped. */
  issuedAt: string | null;
  /** Null when no subject has a grade. */
  generalAverage: number | null;
  /** False when the term closed with nothing captured. */
  hasGrades: boolean;
}

/** One subject row of a boleta: its grade per month ("YYYY-MM") and the term average. */
export interface ReportCardLine {
  subjectName: string;
  months: Record<string, number>;
  average: number | null;
}

export interface ReportCard {
  id: string;
  version: number;
  issuedAt: string | null;
  termName: string;
  cycleName: string;
  /** The term's months, oldest first. */
  months: string[];
  lines: ReportCardLine[];
  generalAverage: number | null;
  scale: GradeScale | null;
  /** False when the term closed with nothing captured. */
  hasGrades: boolean;
}

/** The gateway emits exact decimals as JSON numbers; a string is tolerated. */
type WireNumber = number | string | null | undefined;

interface WireSummary {
  id?: string;
  termId?: string;
  termName?: string;
  cycleName?: string;
  version?: number;
  issuedAt?: string | null;
  generalAverage?: WireNumber;
  hasGrades?: boolean;
}

interface WireLine {
  subjectName?: string;
  months?: Record<string, WireNumber> | null;
  average?: WireNumber;
}

interface WireReportCard {
  id?: string;
  version?: number;
  issuedAt?: string | null;
  termName?: string;
  cycleName?: string;
  months?: string[] | null;
  lines?: WireLine[] | null;
  generalAverage?: WireNumber;
  scale?: Partial<GradeScale> | null;
  hasGrades?: boolean;
}

const FIRST_VERSION = 1;

function toNumber(value: WireNumber): number | null {
  if (value === null || value === undefined || value === "") return null;
  const n = Number(value);
  return Number.isFinite(n) ? n : null;
}

function toMonthValues(months: Record<string, WireNumber> | null | undefined): Record<string, number> {
  const out: Record<string, number> = {};
  for (const [month, raw] of Object.entries(months ?? {})) {
    const value = toNumber(raw);
    if (value !== null) out[month] = value;
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
    cycleName: c.cycleName ?? "",
    version: c.version ?? FIRST_VERSION,
    issuedAt: c.issuedAt ?? null,
    generalAverage: toNumber(c.generalAverage),
    hasGrades: c.hasGrades ?? toNumber(c.generalAverage) !== null,
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
    cycleName: data.cycleName ?? "",
    months: data.months ?? [],
    lines: (data.lines ?? []).map((l) => ({
      subjectName: l.subjectName ?? "",
      months: toMonthValues(l.months),
      average: toNumber(l.average),
    })),
    generalAverage: toNumber(data.generalAverage),
    scale: normalizeScale(data.scale),
    hasGrades: data.hasGrades ?? false,
  };
}
