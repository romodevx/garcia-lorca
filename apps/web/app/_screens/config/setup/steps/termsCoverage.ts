import type { Level, Term } from "../../../../config/api";

/*
 * How far each level's periods go in the cycle: what the level chip of step 4
 * says, and which level the step opens on so nobody lands on an empty view.
 */

/** A term from an older gateway carries no months and counts as none. */
export function leavesMonthsOut(levelTerms: readonly Term[], months: readonly string[]): boolean {
  const taken = new Set(levelTerms.flatMap((t) => t.months ?? []));
  return months.some((m) => !taken.has(m));
}

/** The short line under a level chip: "Sin periodos", "Faltan meses" or "3 periodos". */
export function levelTermsLine(levelTerms: readonly Term[], months: readonly string[]): string {
  if (levelTerms.length === 0) return "Sin periodos";
  if (leavesMonthsOut(levelTerms, months)) return "Faltan meses";
  return levelTerms.length === 1 ? "1 periodo" : `${levelTerms.length} periodos`;
}

/** The first level still to do (no periods, or a month without one); else the first level. */
export function levelToOpen(levels: readonly Level[], cycleTerms: readonly Term[], months: readonly string[]): string | undefined {
  const pending = levels.find((l) => {
    const levelTerms = cycleTerms.filter((t) => t.levelId === l.id);
    return levelTerms.length === 0 || leavesMonthsOut(levelTerms, months);
  });
  return (pending ?? levels[0])?.id;
}
