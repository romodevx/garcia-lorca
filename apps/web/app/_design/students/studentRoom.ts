import type { Student } from "../../students/api";

const SEPARATOR = " · ";

/**
 * A pupil's classroom as a list row names it: "3° PRIMARIA · B" when the
 * gateway resolved it (GET /students carries `groupLabel`, CC-211), else the
 * level and what the file stores, which may be just the letter.
 */
export function studentRoomLabel(s: Pick<Student, "level" | "gradeGroup" | "groupLabel">): string {
  if (s.groupLabel !== undefined && s.groupLabel !== "") return s.groupLabel;
  return [s.level, s.gradeGroup].filter((part) => part !== "").join(SEPARATOR);
}
