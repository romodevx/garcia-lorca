import type { FamilyStudent } from "../api";

/**
 * The display projection of the selected child, derived from the real
 * `FamilyStudent` record. The app chrome (header, "Más" profile) reads this
 * instead of the student wire shape, and it is null-safe: an absent child yields
 * an `present: false` placeholder so the chrome renders without crashing while
 * the child list is loading or when the guardian has no linked children.
 */
export interface ChildView {
  /** False when there is no selected child (loading / no links). */
  present: boolean;
  /** "Mateo Herrera Díaz". */
  fullName: string;
  /** First name, used where space is tight. */
  short: string;
  /** Two-letter monogram for the avatar. */
  initials: string;
  /** Grade + group label, e.g. "3° A". */
  group: string;
  /** The level as the school named it, e.g. "Primaria". */
  levelLabel: string;
  /** True for levels the SEP evaluates qualitatively (kínder). */
  qualitative: boolean;
}

const EMPTY_VIEW: ChildView = {
  present: false,
  fullName: "",
  short: "",
  initials: "",
  group: "",
  levelLabel: "",
  qualitative: false,
};

/** Levels graded qualitatively, folded: kínder, and preescolar — its official name. */
const QUALITATIVE_LEVELS = new Set(["kinder", "preescolar"]);

/**
 * True for levels graded qualitatively (logrado/proceso) rather than
 * numerically. The level is the school's own name for it now (not a fixed SEP
 * key), so it is folded — case and accents — before it is compared.
 */
export function isQualitativeLevel(level: string): boolean {
  const folded = level.trim().toLowerCase().normalize("NFD").replace(/[\u0300-\u036f]/g, "");
  return QUALITATIVE_LEVELS.has(folded);
}

function initialsOf(firstName: string, lastName: string): string {
  const first = firstName.trim().charAt(0);
  const last = lastName.trim().charAt(0);
  return `${first}${last}`.toUpperCase();
}

/** Projects a family student (or null) into the null-safe display view-model. */
export function toChildView(student: FamilyStudent | null): ChildView {
  if (student === null) {
    return EMPTY_VIEW;
  }
  return {
    present: true,
    fullName: `${student.firstName} ${student.lastName}`.trim(),
    short: student.firstName,
    initials: initialsOf(student.firstName, student.lastName),
    group: student.gradeGroup,
    levelLabel: student.level,
    qualitative: isQualitativeLevel(student.level),
  };
}
