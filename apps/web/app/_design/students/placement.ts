/**
 * Where each pupil sits this cycle: level, grade and group (T4).
 *
 * The pupil's file only names the level and the group's bare letter ("A"), and
 * every grade has an "A". The grade and the group come from the pupil's ACTIVE
 * enrolment in the cycle being shown, joined to that cycle's groups and the
 * school's grades. One list of enrolments for the whole cycle, joined here, so
 * the list, the activities picker and the scholarships picker place pupils the
 * same way.
 */

/** Where a pupil sits; "" when they have none. */
export interface Placement {
  levelId: string;
  gradeLevelId: string;
  groupId: string;
}

interface PlacementSources {
  /** The cycle's enrolments (any status: only active ones count). */
  enrollments: readonly { studentId: string; groupId: string; status: string }[];
  /** The cycle's groups. */
  groups: readonly { id: string; gradeLevelId: string }[];
  /** The school's grades, of every level. */
  grades: readonly { id: string; levelId: string }[];
}

/**
 * Places every pupil. A pupil with no active enrolment, or one in a group this
 * cycle does not list, keeps the level on their file and no grade or group.
 * When the group IS known its grade's level wins: the chips are chained, and a
 * pupil whose file disagrees with their group would otherwise vanish from both.
 */
export function placeStudents(
  students: readonly { id: string; levelId: string }[],
  { enrollments, groups, grades }: PlacementSources,
): Map<string, Placement> {
  const groupById = new Map(groups.map((g) => [g.id, g]));
  const levelOfGrade = new Map(grades.map((g) => [g.id, g.levelId]));
  const groupOfStudent = new Map<string, string>();
  for (const e of enrollments) {
    if (e.status === "active" && !groupOfStudent.has(e.studentId)) groupOfStudent.set(e.studentId, e.groupId);
  }

  const placed = new Map<string, Placement>();
  for (const s of students) {
    const group = groupById.get(groupOfStudent.get(s.id) ?? "");
    const gradeLevel = group === undefined ? undefined : levelOfGrade.get(group.gradeLevelId);
    placed.set(
      s.id,
      group === undefined
        ? { levelId: s.levelId, gradeLevelId: "", groupId: "" }
        : { levelId: gradeLevel ?? s.levelId, gradeLevelId: group.gradeLevelId, groupId: group.id },
    );
  }
  return placed;
}
