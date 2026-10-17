/**
 * The order a Mexican school reads its groups in: level (the school's own
 * `sortOrder`, else the catalogue's list order), then grade ordinal, then
 * group name (es-MX collation). Pure — `labelGroups` applies it, so every
 * screen that lists groups gets them in this order (ISSUE-018).
 */

import type { GradeLevel, Group, Level } from "./api";

const LOCALE = "es-MX";
const UNKNOWN_RANK = Number.MAX_SAFE_INTEGER;

export interface SchoolCatalogue {
  levels: readonly Pick<Level, "id" | "sortOrder">[];
  grades: readonly Pick<GradeLevel, "id" | "levelId" | "ordinal">[];
}

/** Comparator: level → grade → name; a group whose grade/level is unknown sorts last. */
/** What the order reads of a group. */
type OrderedGroup = Pick<Group, "gradeLevelId" | "name">;

export function schoolOrder(cat: SchoolCatalogue): (a: OrderedGroup, b: OrderedGroup) => number {
  const levelRank = new Map(cat.levels.map((l, i) => [l.id, l.sortOrder ?? i + 1]));
  const gradeById = new Map(cat.grades.map((g) => [g.id, g]));
  const rank = (g: OrderedGroup): { orphan: number; level: number; levelId: string; ordinal: number } => {
    const grade = gradeById.get(g.gradeLevelId);
    const levelId = grade?.levelId ?? "";
    return { orphan: grade === undefined ? 1 : 0, level: levelRank.get(levelId) ?? UNKNOWN_RANK, levelId, ordinal: grade?.ordinal ?? 0 };
  };
  return (a, b) => {
    const ra = rank(a);
    const rb = rank(b);
    return (
      ra.orphan - rb.orphan ||
      ra.level - rb.level ||
      // Levels that could not be read still stay together, not interleaved by grade.
      ra.levelId.localeCompare(rb.levelId) ||
      ra.ordinal - rb.ordinal ||
      a.name.localeCompare(b.name, LOCALE, { numeric: true })
    );
  };
}

/** A sorted copy; the input is left untouched. */
export function sortBySchoolOrder<G extends OrderedGroup>(groups: readonly G[], cat: SchoolCatalogue): G[] {
  return [...groups].sort(schoolOrder(cat));
}
