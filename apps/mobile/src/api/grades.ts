import { apiFetch } from "./client";
import { ApiError } from "./errors";

/**
 * A guardian's view of one child's grades per evaluation period of the child's
 * grade, for the active cycle (`GET /api/v1/family/students/{id}/grades/periods`,
 * T15). The gateway scopes the read to the caller's own children: an unlinked id
 * answers 404. A grade shows up as soon as the teacher captures it.
 */

/** The school's grading scale for the child's level. */
export interface GradeScale {
  min: number;
  max: number;
  decimals: number;
  /** Lowest passing grade; anything strictly below fails. */
  passing: number;
}

/** An evaluation period of the child's grade (both dates inclusive, YYYY-MM-DD). */
export interface EvaluationPeriod {
  id: string;
  ordinal: number;
  name: string;
  startsOn: string;
  endsOn: string;
}

/** One subject and its grade per period (period id → value); a missing key = not captured yet. */
export interface PeriodSubject {
  subjectId: string;
  subjectName: string;
  grades: Record<string, number>;
}

export interface PeriodGrades {
  cycleName: string;
  /** The grade's evaluation periods, by ordinal. */
  periods: EvaluationPeriod[];
  /**
   * The level's scale, or null when there is none to judge by: a level that
   * stopped grading numerically omits it (grades do not record the scale they
   * were captured under), and an empty 0–0 scale is read the same way.
   */
  scale: GradeScale | null;
  /** False for levels without numeric grades (preescolar). */
  gradingEnabled: boolean;
  subjects: PeriodSubject[];
}

const DEFAULT_SCALE: GradeScale = { min: 0, max: 10, decimals: 1, passing: 6 };

/** The wire scale, or null when absent or empty (max not above min: nothing to judge by). */
export function normalizeScale(scale: Partial<GradeScale> | null | undefined): GradeScale | null {
  if (scale === null || scale === undefined) return null;
  const full = { ...DEFAULT_SCALE, ...scale };
  return full.max > full.min ? full : null;
}

interface WireSubject {
  subjectId?: string;
  subjectName?: string;
  grades?: Record<string, number> | null;
}

interface WirePeriodGrades {
  cycleName?: string;
  periods?: EvaluationPeriod[] | null;
  scale?: Partial<GradeScale> | null;
  gradingEnabled?: boolean | null;
  subjects?: WireSubject[] | null;
}

function normalizeSubject(subject: WireSubject): PeriodSubject {
  return {
    subjectId: subject.subjectId ?? "",
    subjectName: subject.subjectName ?? "",
    grades: subject.grades ?? {},
  };
}

/** Periods by ordinal, whatever order the wire used. */
export function byOrdinal<T extends { ordinal: number }>(periods: readonly T[]): T[] {
  return [...periods].sort((a, b) => a.ordinal - b.ordinal);
}

/** Reads one child's grades per evaluation period. Throws {@link ApiError} on non-2xx. */
export async function getPeriodGrades(studentId: string): Promise<PeriodGrades> {
  const res = await apiFetch(`/api/v1/family/students/${encodeURIComponent(studentId)}/grades/periods`);
  if (!res.ok) {
    throw new ApiError(res.status, "failed to load period grades");
  }
  const data = (await res.json()) as WirePeriodGrades;
  return {
    cycleName: data.cycleName ?? "",
    periods: byOrdinal(data.periods ?? []),
    scale: normalizeScale(data.scale),
    gradingEnabled: data.gradingEnabled ?? true,
    subjects: (data.subjects ?? []).map(normalizeSubject),
  };
}
