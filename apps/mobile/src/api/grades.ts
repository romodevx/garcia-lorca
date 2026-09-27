import { apiFetch } from "./client";
import { ApiError } from "./errors";

/**
 * A guardian's view of one child's monthly grades for the active cycle
 * (`GET /api/v1/family/students/{id}/grades/monthly`). The gateway scopes the
 * read to the caller's own children: an unlinked id answers 404. A grade shows
 * up as soon as the teacher captures it.
 */

/** The school's grading scale for the child's level. */
export interface GradeScale {
  min: number;
  max: number;
  decimals: number;
  /** Lowest passing grade; anything strictly below fails. */
  passing: number;
}

/** One subject and its grade per month ("YYYY-MM" → value); a missing key = not captured yet. */
export interface MonthlySubject {
  subjectId: string;
  subjectName: string;
  grades: Record<string, number>;
}

export interface MonthlyGrades {
  cycleName: string;
  /** The cycle's months, oldest first ("YYYY-MM"). */
  months: string[];
  scale: GradeScale;
  /** False for levels without numeric grades (preescolar). */
  gradingEnabled: boolean;
  subjects: MonthlySubject[];
}

const DEFAULT_SCALE: GradeScale = { min: 0, max: 10, decimals: 1, passing: 6 };

interface WireSubject {
  subjectId?: string;
  subjectName?: string;
  grades?: Record<string, number> | null;
}

interface WireMonthlyGrades {
  cycleName?: string;
  months?: string[] | null;
  scale?: Partial<GradeScale> | null;
  gradingEnabled?: boolean | null;
  subjects?: WireSubject[] | null;
}

function normalizeSubject(subject: WireSubject): MonthlySubject {
  return {
    subjectId: subject.subjectId ?? "",
    subjectName: subject.subjectName ?? "",
    grades: subject.grades ?? {},
  };
}

/** Reads one child's monthly grades. Throws {@link ApiError} on non-2xx. */
export async function getMonthlyGrades(studentId: string): Promise<MonthlyGrades> {
  const res = await apiFetch(`/api/v1/family/students/${encodeURIComponent(studentId)}/grades/monthly`);
  if (!res.ok) {
    throw new ApiError(res.status, "failed to load monthly grades");
  }
  const data = (await res.json()) as WireMonthlyGrades;
  return {
    cycleName: data.cycleName ?? "",
    months: data.months ?? [],
    scale: { ...DEFAULT_SCALE, ...(data.scale ?? {}) },
    gradingEnabled: data.gradingEnabled ?? true,
    subjects: (data.subjects ?? []).map(normalizeSubject),
  };
}
