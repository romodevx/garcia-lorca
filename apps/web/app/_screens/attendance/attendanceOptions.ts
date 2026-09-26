/**
 * The teacher's attendance classes as picker options. Pure.
 *
 *   only daily groups ──> group-only picking (`?grupo=`)
 *   any per-class item ─> subject first (`?asignatura=<subjectId|dia>&grupo=`),
 *                         the daily groups gathered under "Lista del día"
 */

import type { AttendanceClass } from "../../teaching/api";
import type { TeachingOption } from "../teaching/selection";

/** The subject-step key of the daily list: it is a list, not a subject. */
export const DAILY_SUBJECT = "dia";
const DAILY_LABEL = "Lista del día";

export interface AttendanceOption extends TeachingOption {
  /** What the server is asked for: the subject of a per-class list, nothing for a daily one. */
  attendanceSubjectId?: string;
}

export interface AttendanceOptions {
  options: AttendanceOption[];
  /** True when the picker must ask for the subject (or the daily list) first. */
  bySubject: boolean;
}

function dailyOption(cls: AttendanceClass, bySubject: boolean): AttendanceOption {
  const base = { groupId: cls.groupId, groupName: cls.groupLabel };
  return bySubject
    ? { ...base, id: `${DAILY_SUBJECT}:${cls.groupId}`, subjectId: DAILY_SUBJECT, subjectName: DAILY_LABEL }
    : { ...base, id: cls.groupId };
}

/**
 * A per-class option carries its grade apart from the letter, so the subject
 * step can tell "MATEMÁTICAS · 1° SECUNDARIA" from "· 2° SECUNDARIA". Without
 * both halves (an older gateway) it falls back to the ready-made label.
 */
function groupParts(cls: AttendanceClass): Pick<AttendanceOption, "groupName" | "gradeLevelName"> {
  const name = cls.groupName?.trim() ?? "";
  const grade = cls.gradeLevelName?.trim() ?? "";
  return name !== "" && grade !== "" ? { groupName: name, gradeLevelName: grade } : { groupName: cls.groupLabel };
}

function perClassOption(cls: AttendanceClass): AttendanceOption {
  const subjectId = cls.subjectId ?? "";
  return {
    id: cls.assignmentId ?? `${subjectId}:${cls.groupId}`,
    groupId: cls.groupId,
    ...groupParts(cls),
    subjectId,
    subjectName: cls.subjectName,
    attendanceSubjectId: subjectId,
  };
}

export function toAttendanceOptions(classes: readonly AttendanceClass[]): AttendanceOptions {
  const bySubject = classes.some((cls) => cls.mode === "per_class");
  const seen = new Set<string>();
  const options: AttendanceOption[] = [];
  for (const cls of classes) {
    const option = cls.mode === "per_class" ? perClassOption(cls) : dailyOption(cls, bySubject);
    if (seen.has(option.id)) continue;
    seen.add(option.id);
    options.push(option);
  }
  return { options, bySubject };
}
