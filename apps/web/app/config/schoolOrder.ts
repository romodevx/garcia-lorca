/**
 * The order a Mexican school reads its groups in: level (as the catalogue
 * lists them), then grade ordinal, then group letter (es-MX collation).
 * Pure — shared by every picker that lists groups.
 */

import type { GradeLevel, LabelledGroup, Level } from "./api";

const LOCALE = "es-MX";
const UNKNOWN_RANK = Number.MAX_SAFE_INTEGER;

export interface SchoolCatalogue {
  levels: readonly Level[];
  grades: readonly GradeLevel[];
}

/** Comparator: level → grade → letter; a group whose grade/level is unknown sorts last. */
export function schoolOrder(cat: SchoolCatalogue): (a: LabelledGroup, b: LabelledGroup) => number {
  const levelIndex = new Map(cat.levels.map((l, i) => [l.id, i]));
  const gradeById = new Map(cat.grades.map((g) => [g.id, g]));
  const rank = (g: LabelledGroup): [number, number] => {
    const grade = gradeById.get(g.gradeLevelId);
    return [levelIndex.get(grade?.levelId ?? "") ?? UNKNOWN_RANK, grade?.ordinal ?? 0];
  };
  return (a, b) => {
    const [la, ga] = rank(a);
    const [lb, gb] = rank(b);
    return la - lb || ga - gb || a.name.localeCompare(b.name, LOCALE, { numeric: true });
  };
}

/** A sorted copy; the input is left untouched. */
export function sortBySchoolOrder<G extends LabelledGroup>(groups: readonly G[], cat: SchoolCatalogue): G[] {
  return [...groups].sort(schoolOrder(cat));
}
