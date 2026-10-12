/**
 * Who an announcement goes to, as the Avisos view lists it (PLAN-redesign-avisos
 * §10). Pure: the lists come from useAnnouncementComposer, already loaded.
 */

import type { AnnouncementScope } from "../../comms/api";
import type { GradeLevel, LabelledGroup, Level } from "../../config/api";
import { sortBySchoolOrder } from "../../config/schoolOrder";
import type { Student } from "../../students/api";
import { formatStudentName } from "../../_design/students/studentName";

/** B1: the four audiences, in this order. */
export const SCOPES: readonly { value: AnnouncementScope; label: string }[] = [
  { value: "student", label: "Un alumno" },
  { value: "group", label: "Grupos" },
  { value: "level", label: "Niveles" },
  { value: "cycle", label: "Toda la escuela" },
];

/** Where the groups of a level nobody can name go. */
const OTHERS_TITLE = "Otros";

/** How many names "Para …" spells out before it counts the rest (D8). */
export const RECIPIENTS_SHOWN = 3;

/** One pickable row: `label` is today's accessible name, `text` what it shows. */
export interface Target {
  id: string;
  label: string;
  text: string;
}

/** A level's groups; `title` is undefined when no group's level is known. */
export interface TargetSection {
  key: string;
  title: string | undefined;
  targets: Target[];
}

/** The grades the groups carry (labelGroups' additive fields), once each. */
function gradesOf(groups: readonly LabelledGroup[]): GradeLevel[] {
  const grades = new Map<string, GradeLevel>();
  for (const g of groups) {
    if (g.levelId === undefined) continue;
    grades.set(g.gradeLevelId, { id: g.gradeLevelId, levelId: g.levelId, ordinal: g.ordinal ?? 0, name: g.gradeName ?? "" });
  }
  return [...grades.values()];
}

function toTarget(g: LabelledGroup): Target {
  return { id: g.id, label: g.label, text: g.gradeName === undefined ? g.label : `${g.gradeName} ${g.name}` };
}

/**
 * The groups filed under their level, in the school's reading order (level,
 * grade, letter: config/schoolOrder). A group whose level is unknown goes
 * last, under "Otros"; when no group's level is known it is one flat list.
 */
export function groupSections(groups: readonly LabelledGroup[], levels: readonly Level[]): TargetSection[] {
  const sorted = sortBySchoolOrder(groups, { levels, grades: gradesOf(groups) });
  const levelName = new Map(levels.map((l) => [l.id, l.name]));
  const sections = new Map<string, TargetSection>();
  for (const g of sorted) {
    const title = levelName.get(g.levelId ?? "") ?? OTHERS_TITLE;
    const section = sections.get(title) ?? { key: g.levelId ?? OTHERS_TITLE, title, targets: [] };
    section.targets.push(toTarget(g));
    sections.set(title, section);
  }
  const list = [...sections.values()];
  const onlyOthers = list.length === 1 && list[0]?.title === OTHERS_TITLE;
  return onlyOthers ? list.map((s) => ({ ...s, title: undefined })) : list;
}

export interface AudienceCatalogue {
  groups: readonly LabelledGroup[];
  levels: readonly Level[];
  students: readonly Student[];
}

/** What each audience calls its members; the whole school has none. */
function namesById(scope: AnnouncementScope, cat: AudienceCatalogue): Map<string, string> {
  if (scope === "group") return new Map(cat.groups.map((g) => [g.id, g.label]));
  if (scope === "level") return new Map(cat.levels.map((l) => [l.id, l.name]));
  if (scope === "student") return new Map(cat.students.map((s) => [s.id, formatStudentName(s)]));
  return new Map();
}

/** The picked recipients by name, in the order they were picked. */
export function recipientNames(scope: AnnouncementScope, picked: readonly string[], cat: AudienceCatalogue): string[] {
  const names = namesById(scope, cat);
  return picked.flatMap((id) => names.get(id) ?? []);
}

/** The names "Para …" spells out, and how many it only counts. */
export function summarizeRecipients(names: readonly string[]): { shown: string[]; more: number } {
  return { shown: names.slice(0, RECIPIENTS_SHOWN), more: Math.max(0, names.length - RECIPIENTS_SHOWN) };
}
