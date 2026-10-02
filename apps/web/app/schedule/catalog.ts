/**
 * Names and filters over a ScheduleBoard (T16). The board carries its own
 * catalog, so the screen never needs a /config call: someone with
 * schedule:read and no config:read still sees "3.º A" and "Ana Ruiz".
 */
import type { ScheduleBoard } from "./api";

/** Monday … Friday, as the grid's columns and the wire's weekday (1–5). */
export const WEEKDAYS: readonly { weekday: number; label: string }[] = [
  { weekday: 1, label: "Lunes" },
  { weekday: 2, label: "Martes" },
  { weekday: 3, label: "Miércoles" },
  { weekday: 4, label: "Jueves" },
  { weekday: 5, label: "Viernes" },
];

const GONE_SUBJECT = "Asignatura sin registro";
const GONE_TEACHER = "Docente sin registro";
const GONE_GROUP = "Grupo sin registro";

export interface ScheduleIndex {
  subjectName: (id: string) => string;
  teacherName: (id: string) => string;
  /** "3.º A": the grade's name and the group's letter ("1.º A · Secundaria" when another level has a "1.º A"). */
  groupLabel: (id: string) => string;
  levelOfGroup: (id: string) => string;
  gradeOfGroup: (id: string) => string;
}

/** O(1) lookups over the board's catalog, built once per board. */
export function indexBoard(board: ScheduleBoard): ScheduleIndex {
  const subjects = new Map(board.subjects.map((s) => [s.id, s.name]));
  const teachers = new Map(board.teachers.map((t) => [t.id, `${t.firstName} ${t.lastName}`.trim()]));
  const grades = new Map(board.grades.map((g) => [g.id, g]));
  const groups = new Map(board.groups.map((g) => [g.id, g]));
  const gradeOf = (id: string): string => groups.get(id)?.gradeLevelId ?? "";
  const levels = new Map(board.levels.map((l) => [l.id, l.name]));
  const baseLabel = (groupId: string): string => {
    const group = groups.get(groupId);
    if (group === undefined) return GONE_GROUP;
    const grade = grades.get(group.gradeLevelId);
    return grade === undefined ? group.name : `${grade.name} ${group.name}`;
  };
  // "1.º A" may exist in Primaria and in Secundaria: those labels carry the level.
  const seen = new Map<string, number>();
  for (const g of board.groups) seen.set(baseLabel(g.id), (seen.get(baseLabel(g.id)) ?? 0) + 1);
  return {
    subjectName: (id) => subjects.get(id) ?? GONE_SUBJECT,
    teacherName: (id) => teachers.get(id) ?? GONE_TEACHER,
    groupLabel: (id) => {
      const label = baseLabel(id);
      if ((seen.get(label) ?? 0) < 2) return label;
      const level = levels.get(grades.get(gradeOf(id))?.levelId ?? "");
      return level === undefined ? label : `${label} · ${level}`;
    },
    gradeOfGroup: gradeOf,
    levelOfGroup: (id) => grades.get(gradeOf(id))?.levelId ?? "",
  };
}

interface ClassLike {
  subjectId: string;
  groupId: string;
  teacherId: string;
  weekday: number;
  startsAt: string;
  endsAt: string;
}

/** What a screen reader says for a class: "Matemáticas, 3.º A, Ana Ruiz, lunes 08:00 a 08:50". */
export function blockLabel(index: ScheduleIndex, c: ClassLike): string {
  const day = (WEEKDAYS.find((d) => d.weekday === c.weekday)?.label ?? "").toLocaleLowerCase("es-MX");
  return `${index.subjectName(c.subjectId)}, ${index.groupLabel(c.groupId)}, ${index.teacherName(c.teacherId)}, ${day} ${c.startsAt} a ${c.endsAt}`;
}

/** What the week shows: "" in a field means any. */
export interface WeekFilter {
  levelId: string;
  gradeId: string;
  groupId: string;
  teacherId: string;
}

export const NO_WEEK_FILTER: WeekFilter = { levelId: "", gradeId: "", groupId: "", teacherId: "" };

/** The classes the filter keeps: the group's (or the grade's, or the level's) and the teacher's. */
export function filterBlocks(board: ScheduleBoard, f: WeekFilter): ScheduleBoard["blocks"] {
  const index = indexBoard(board);
  return board.blocks.filter((b) => {
    if (f.teacherId !== "" && b.teacherId !== f.teacherId) return false;
    if (f.groupId !== "") return b.groupId === f.groupId;
    if (f.gradeId !== "") return index.gradeOfGroup(b.groupId) === f.gradeId;
    if (f.levelId !== "") return index.levelOfGroup(b.groupId) === f.levelId;
    return true;
  });
}

/** The group's teaching assignments as select options — the only things a class is built from. */
export function assignmentsOfGroup(board: ScheduleBoard, index: ScheduleIndex, groupId: string): { value: string; label: string }[] {
  return board.assignments
    .filter((a) => a.groupId === groupId)
    .map((a) => ({ value: a.id, label: `${index.subjectName(a.subjectId)} — ${index.teacherName(a.teacherId)}` }))
    .sort((a, b) => a.label.localeCompare(b.label, "es-MX"));
}
