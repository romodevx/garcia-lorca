/**
 * The director's grade client (`/api/v1/grades/*`), gated behind the
 * `module_grades` flag + `grades:read`/`grades:write`. The session lives only
 * in the HttpOnly cookie the gateway set (`credentials:"include"`).
 *
 * Grades are captured per MONTH: one sheet per assignment·month, one value per
 * enrollment, saved cell by cell. The contract is shared with the teacher's
 * self-scoped client (see `monthly.ts`). UX only — the gateway is the
 * security boundary.
 */

import type { GradeSheetClient } from "./monthly";
import { createGradeSheetClient } from "./monthlyHttp";

/** The school-wide monthly sheet: load, save a cell, clear a cell. */
export function gradeSheetClient(baseURL: string): GradeSheetClient {
  return createGradeSheetClient(`${baseURL.replace(/\/+$/, "")}/api/v1/grades`);
}
