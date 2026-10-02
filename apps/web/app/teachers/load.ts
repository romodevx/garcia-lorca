/**
 * Carga académica: the pure rules behind a teacher's load page.
 *
 * Subjects live per grade level — Matemáticas of 1° and Matemáticas of 2° are
 * different rows. People think of "Matemáticas", not of a row id, so the page
 * lets them pick a subject BY NAME and this module resolves, per group, the row
 * of that group's grade that carries the name. Only grades that carry it are
 * offered (T13): a group of another grade is never a choice, and a row is never
 * invented here.
 *
 * No React, no I/O: everything is a projection of the cycle's catalogue.
 */
import type { GradeLevel, LabelledGroup, Level, Subject } from "../config/api";
import { schoolOrder } from "../config/schoolOrder";
import type { Teacher, TeachingAssignment } from "./api";

const LOCALE = "es-MX";
const byName = (a: string, b: string): number => a.localeCompare(b, LOCALE);
/**
 * Two spellings of one subject are one subject: the catalog captures names in
 * uppercase now, and older rows ("Matemáticas") are mixed case.
 */
const nameKey = (name: string): string => name.trim().toLocaleUpperCase(LOCALE);

/** Everything the page reads for one cycle. */
export interface LoadCatalogue {
  levels: readonly Level[];
  grades: readonly GradeLevel[];
  groups: readonly LabelledGroup[];
  /** Subjects of every grade level that has a group in the cycle. */
  subjects: readonly Subject[];
  /** The WHOLE cycle's assignments — who else holds a slot matters. */
  assignments: readonly TeachingAssignment[];
  teachers: readonly Teacher[];
}

/** One slot of a teacher's load, named for people. */
export interface LoadSlot {
  assignmentId: string;
  groupId: string;
  subjectId: string;
  groupLabel: string;
  subjectName: string;
}

export interface SubjectLoad {
  subjectName: string;
  slots: LoadSlot[];
}

/** What choosing a group for the picked subject would mean. */
export type GroupAvailability =
  | { kind: "free"; subjectId: string }
  | { kind: "held"; subjectId: string; teacherName: string }
  | { kind: "mine"; subjectId: string };

export type SelectableAvailability = Extract<GroupAvailability, { kind: "free" | "held" }>;

export interface GroupChoice {
  group: LabelledGroup;
  availability: GroupAvailability;
}

export interface GradeChoices {
  grade: GradeLevel;
  groups: GroupChoice[];
}

export interface LevelChoices {
  level: Level;
  grades: GradeChoices[];
}

/** Free slots and slots another teacher holds can be picked; the rest cannot. */
export function isSelectable(availability: GroupAvailability): availability is SelectableAvailability {
  return availability.kind === "free" || availability.kind === "held";
}

/** Each subject name once, sorted the way a Mexican reader expects. */
export function subjectNames(subjects: readonly Subject[]): string[] {
  const byKey = new Map<string, string>();
  for (const s of subjects) if (!byKey.has(nameKey(s.name))) byKey.set(nameKey(s.name), s.name);
  return [...byKey.values()].sort(byName);
}

export function teacherName(teacher: Teacher): string {
  return `${teacher.firstName} ${teacher.lastName}`;
}

/** One teacher's slots, grouped by subject name. */
export function currentLoad(teacherId: string, cat: LoadCatalogue): SubjectLoad[] {
  const groupById = new Map(cat.groups.map((g) => [g.id, g]));
  const subjectById = new Map(cat.subjects.map((s) => [s.id, s]));
  const order = schoolOrder(cat);
  const bySubject = new Map<string, { group: LabelledGroup; slot: LoadSlot }[]>();

  for (const a of cat.assignments) {
    const group = groupById.get(a.groupId);
    if (a.teacherId !== teacherId || group === undefined) continue;
    const subjectName = subjectById.get(a.subjectId)?.name ?? "Asignatura";
    const slot: LoadSlot = { assignmentId: a.id, groupId: a.groupId, subjectId: a.subjectId, groupLabel: group.label, subjectName };
    const key = nameKey(subjectName);
    bySubject.set(key, [...(bySubject.get(key) ?? []), { group, slot }]);
  }

  return [...bySubject.values()]
    .map((entries) => ({
      subjectName: entries[0]?.slot.subjectName ?? "",
      slots: [...entries].sort((x, y) => order(x.group, y.group)).map((e) => e.slot),
    }))
    .sort((a, b) => byName(a.subjectName, b.subjectName));
}

/**
 * The groups grid for one subject name: level → grade → group, each with its
 * availability. Only the grades that have a subject of that name appear, so
 * every group offered resolves to the subject row of its own grade.
 */
export function groupChoices(teacherId: string, subjectName: string, cat: LoadCatalogue): LevelChoices[] {
  const subjectOfGrade = new Map(
    cat.subjects.filter((s) => nameKey(s.name) === nameKey(subjectName)).map((s) => [s.gradeLevelId, s.id]),
  );
  const holder = new Map(cat.assignments.map((a) => [`${a.groupId}|${a.subjectId}`, a.teacherId]));
  const nameOf = new Map(cat.teachers.map((t) => [t.id, teacherName(t)]));

  const availability = (group: LabelledGroup, subjectId: string): GroupAvailability => {
    const held = holder.get(`${group.id}|${subjectId}`);
    if (held === undefined) return { kind: "free", subjectId };
    if (held === teacherId) return { kind: "mine", subjectId };
    return { kind: "held", subjectId, teacherName: nameOf.get(held) ?? "otro docente" };
  };

  return cat.levels
    .map((level) => ({
      level,
      grades: cat.grades
        .filter((grade) => grade.levelId === level.id)
        .sort((a, b) => a.ordinal - b.ordinal)
        .flatMap((grade) => {
          const subjectId = subjectOfGrade.get(grade.id);
          if (subjectId === undefined) return [];
          const groups = cat.groups
            .filter((g) => g.gradeLevelId === grade.id)
            .sort((a, b) => byName(a.name, b.name))
            .map((group) => ({ group, availability: availability(group, subjectId) }));
          return [{ grade, groups }];
        })
        .filter((g) => g.groups.length > 0),
    }))
    .filter((l) => l.grades.length > 0);
}
