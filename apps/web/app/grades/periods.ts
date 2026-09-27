/**
 * The director's grading periods (trimesters) and their boletas, over
 * `/api/v1/grades/periods` and `/api/v1/grades/report-cards` (module_grades;
 * reads need `grades:read`, closing and reopening `grades:close`). Refusals
 * are text/plain with an `X-Error-Code`, surfaced as `GradeRequestError`.
 * UX only — the gateway is the security boundary.
 */

import { authFetch } from "../auth/authFetch";
import { gradeFailure } from "./monthly";

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
  months: string[];
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
  subjectName: string;
  teacherName: string;
  missingCount: number;
}

export interface PeriodCaptureProgress {
  termId: string;
  months: string[];
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
  issuedAt: string;
  generalAverage: number | null;
}

/** Codes a close can be refused with. */
export const MISSING_GRADES = "MISSING_GRADES";
export const TERM_WITHOUT_MONTHS = "TERM_WITHOUT_MONTHS";

export interface PeriodsClient {
  list: (cycleId: string) => Promise<GradingPeriod[]>;
  progress: (termId: string) => Promise<PeriodCaptureProgress>;
  close: (termId: string, force: boolean) => Promise<CloseResult>;
  reopen: (termId: string) => Promise<{ status: "open" }>;
  reportCards: (termId: string, groupId: string) => Promise<ReportCard[]>;
  reportCardPdf: (reportCardId: string) => Promise<Blob>;
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
    reportCardPdf: async (reportCardId) => (await send(`/report-cards/${id(reportCardId)}/pdf`)).blob(),
  };
}

/** "José Núñez" → "jose-nunez": no accents, no spaces, safe in any file system. */
function slug(text: string): string {
  return text
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

/** `boleta-<alumno>-<trimestre>.pdf`. */
export function boletaFileName(studentName: string, termName: string): string {
  return `boleta-${slug(studentName)}-${slug(termName)}.pdf`;
}
