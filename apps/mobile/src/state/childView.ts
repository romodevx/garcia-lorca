import { formatDisplayName, formatStudentName, studentInitials } from "@garcia-lorca/domain/student-name";

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
  /** "Herrera Díaz Mateo": surnames first, as every school list (T3). */
  fullName: string;
  /** The pupil's paternal surname for the family greeting. */
  short: string;
  /** First name + paternal surname for the app header: "Mateo Herrera". */
  headerName: string;
  /** Grade, level and group, with the level retained when grade is absent. */
  subtitle: string;
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
  headerName: "",
  subtitle: "",
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

/** Formats the child's grade, level and optional group for the family profile. */
export function formatSubtitle(gradeGroup: string, level: string): string {
  const trimmedGroup = gradeGroup.trim();
  const gradeMatch = trimmedGroup.match(/^(\d+°?)\s*(.*)$/);
  const grade = gradeMatch?.[1] ?? "";
  const group = (gradeMatch?.[2] ?? trimmedGroup).trim();
  const levelText = [grade, level.trim()].filter(Boolean).join(" ");
  return [levelText, group && group !== grade ? group : ""].filter(Boolean).join(" · ");
}

/**
 * The school's own name for the group ("3° PRIMARIA · B", ISSUE-T14-108). A
 * label that is only the letter is the gateway's fallback: the letter and the
 * level then say more.
 */
function subtitleOf(student: FamilyStudent): string {
  const label = (student.groupLabel ?? "").trim();
  if (label !== "" && label !== student.gradeGroup.trim()) return label;
  return formatSubtitle(student.gradeGroup, student.level);
}

/** Projects a family student (or null) into the null-safe display view-model. */
export function toChildView(student: FamilyStudent | null): ChildView {
  if (student === null) {
    return EMPTY_VIEW;
  }
  return {
    present: true,
    fullName: formatStudentName(student),
    short: formatDisplayName("", student.lastName),
    headerName: formatDisplayName(student.firstName, student.lastName),
    subtitle: subtitleOf(student),
    initials: studentInitials(student),
    group: student.gradeGroup,
    levelLabel: student.level,
    qualitative: isQualitativeLevel(student.level),
  };
}
