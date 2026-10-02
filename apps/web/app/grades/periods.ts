/**
 * The director's grading periods (trimesters) and their boletas, over
 * `/api/v1/grades/periods` and `/api/v1/grades/report-cards` (module_grades;
 * reads need `grades:read`, closing and reopening `grades:close`). Refusals
 * are text/plain with an `X-Error-Code`, surfaced as `GradeRequestError`.
 * UX only — the gateway is the security boundary.
 */

import { formatAverage } from "@garcia-lorca/domain/grade-average";

import { authFetch } from "../auth/authFetch";
import { gradeFailure } from "./requestError";

export type PeriodStatus = "open" | "closed";

export interface PeriodProgress {
  captured: number;
  expected: number;
}

export interface GradingPeriod {
  termId: string;
  termName: string;
  ordinal: number;
  levelId: string;
  levelName: string;
  /** The earliest start of the term's evaluation periods across the level's grades; null when no grade has one. */
  startsOn: string | null;
  /** The latest end of those periods; null when no grade has one. */
  endsOn: string | null;
  status: PeriodStatus;
  closedAt?: string;
  closedByName?: string;
  /** 0 until the first close; each re-close issues the next boleta version. */
  version: number;
  progress: PeriodProgress;
}

export interface MissingCapture {
  groupId: string;
  groupLabel: string;
  subjectId: string;
  subjectName: string;
  teacherName: string;
  missingCount: number;
}

export interface PeriodCaptureProgress {
  termId: string;
  /** A grade's period of this term has not ended yet: its grades cannot be complete. */
  unfinished: boolean;
  /** The latest end of the term's periods across the level's grades; null when none. */
  endsOn: string | null;
  /** Groups whose grade has no evaluation period of this ordinal: closing is refused while any is listed. */
  groupsWithoutPeriod: string[];
  expected: number;
  captured: number;
  missing: MissingCapture[];
}

export interface CloseResult {
  status: "closed";
  version: number;
  reportCards: number;
}

export interface ReportCard {
  id: string;
  studentId: string;
  studentName: string;
  groupLabel: string;
  version: number;
  issuedAt: string | null;
  generalAverage: number | null;
  /**
   * False when no subject has a grade in the term: the boleta is still issued
   * and reads NO_GRADES_NOTICE. Absent from older gateways (read from
   * generalAverage, which is null exactly then).
   */
  hasGrades?: boolean;
}

/** What a boleta without any grade says instead of a table of dashes. */
export const NO_GRADES_NOTICE = "Sin calificaciones capturadas en este periodo";

/** Whether the boleta has any grade (see ReportCard.hasGrades). */
export function reportCardHasGrades(card: Pick<ReportCard, "hasGrades" | "generalAverage">): boolean {
  return card.hasGrades ?? card.generalAverage !== null;
}

/** A boleta's PDF and the name the server gave it, when the browser may read it. */
export interface ReportCardFile {
  blob: Blob;
  fileName?: string;
}

/** Codes a close can be refused with. */
export const MISSING_GRADES = "MISSING_GRADES";
/** A group's grade has no evaluation period of the term's ordinal (T15). */
export const PERIOD_NOT_CONFIGURED = "PERIOD_NOT_CONFIGURED";
export const ALREADY_CLOSED = "ALREADY_CLOSED";
export const LEVEL_NOT_GRADED = "LEVEL_NOT_GRADED";
/** The code a reopen of an open period is refused with. */
export const ALREADY_OPEN = "ALREADY_OPEN";

/** The general average as people read it: "8.66", "8.5", "9" (T15 rule; older boletas keep their one decimal). */
export function generalAverageLabel(average: number | null): string {
  return average === null ? "—" : formatAverage(average);
}

export interface PeriodsClient {
  list: (cycleId: string) => Promise<GradingPeriod[]>;
  progress: (termId: string) => Promise<PeriodCaptureProgress>;
  close: (termId: string, force: boolean) => Promise<CloseResult>;
  reopen: (termId: string) => Promise<{ status: "open" }>;
  reportCards: (termId: string, groupId: string) => Promise<ReportCard[]>;
  reportCardPdf: (reportCardId: string) => Promise<ReportCardFile>;
}

export function periodsClient(baseURL: string): PeriodsClient {
  const root = `${baseURL.replace(/\/+$/, "")}/api/v1/grades`;
  const send = async (path: string, init: RequestInit = {}): Promise<Response> => {
    const res = await authFetch(`${root}${path}`, { credentials: "include", ...init });
    if (!res.ok) throw await gradeFailure(res);
    return res;
  };
  const post = (path: string, body?: unknown): Promise<Response> =>
    send(path, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  const id = encodeURIComponent;

  return {
    list: async (cycleId) => {
      const data = (await (await send(`/periods?${new URLSearchParams({ cycleId }).toString()}`)).json()) as { periods?: GradingPeriod[] };
      return data.periods ?? [];
    },
    progress: async (termId) => (await (await send(`/periods/${id(termId)}/progress`)).json()) as PeriodCaptureProgress,
    close: async (termId, force) => (await (await post(`/periods/${id(termId)}/close`, { force })).json()) as CloseResult,
    reopen: async (termId) => (await (await post(`/periods/${id(termId)}/reopen`)).json()) as { status: "open" },
    reportCards: async (termId, groupId) => {
      const res = await send(`/report-cards?${new URLSearchParams({ termId, groupId }).toString()}`);
      return ((await res.json()) as { reportCards?: ReportCard[] }).reportCards ?? [];
    },
    reportCardPdf: async (reportCardId) => {
      const res = await send(`/report-cards/${id(reportCardId)}/pdf`);
      return { blob: await res.blob(), fileName: fileNameFromDisposition(res.headers.get("Content-Disposition")) };
    },
  };
}

/** "José Núñez" → "jose-nunez": no accents, no spaces, safe in any file system. */
function slug(text: string): string {
  return text
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    // Ordinal indicators ("2.º", "1.ª") read as letters, like "1.er" does.
    .replace(/\u00ba/g, "o")
    .replace(/\u00aa/g, "a")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

/** `boleta-<alumno>-<trimestre>-v<N>.pdf` — the fallback when the server's name cannot be read. */
export function boletaFileName(studentName: string, termName: string, version: number): string {
  return `boleta-${slug(studentName)}-${slug(termName)}-v${version}.pdf`;
}

const EXTENDED_FILENAME = /filename\*\s*=\s*[^']*'[^']*'([^;]+)/i;
const PLAIN_FILENAME = /filename\s*=\s*(?:"([^"]*)"|([^;]+))/i;

function decoded(value: string): string | undefined {
  try {
    return decodeURIComponent(value.trim());
  } catch {
    return undefined;
  }
}

/**
 * The file name in a `Content-Disposition` header (RFC 6266: `filename*` wins
 * over `filename`). A name with a path in it is refused, never trimmed.
 */
export function fileNameFromDisposition(header: string | null): string | undefined {
  if (header === null) return undefined;
  const extended = EXTENDED_FILENAME.exec(header)?.[1];
  const plain = PLAIN_FILENAME.exec(header);
  const name = extended !== undefined ? decoded(extended) : (plain?.[1] ?? plain?.[2])?.trim();
  if (name === undefined || name === "" || /[\\/]/.test(name)) return undefined;
  return name;
}
