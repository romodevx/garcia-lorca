/**
 * The director's grade client (`/api/v1/grades/*`), gated behind the
 * `module_grades` flag + `grades:read`/`grades:write`. The session lives only
 * in the HttpOnly cookie the gateway set (`credentials:"include"`).
 *
 * Grades are captured per EVALUATION PERIOD of the group's grade (T15): one
 * sheet per assignment·period, one whole-number value per enrollment, saved
 * cell by cell. The contract is shared with the teacher's self-scoped client
 * (see `periodSheet.ts`). UX only — the gateway is the security boundary.
 */

import type { GradeSheetClient } from "./periodSheet";
import { createGradeSheetClient } from "./periodSheetHttp";

/** The school-wide period sheet: load, save a cell, clear a cell. */
export function gradeSheetClient(baseURL: string): GradeSheetClient {
  return createGradeSheetClient(`${baseURL.replace(/\/+$/, "")}/api/v1/grades`);
}
