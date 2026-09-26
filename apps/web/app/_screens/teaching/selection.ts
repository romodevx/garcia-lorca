/**
 * Where a teacher stands in "subject → group → list", read from and written to
 * the query string so the browser's Back returns to the previous step and a
 * reload keeps the place. Pure: no React, no window.
 *
 * Every step is keyed by ID, never by name: two subjects may share a name (the
 * same "Matemáticas" in two levels), a subject may have none, and two classes of
 * one subject may sit in groups that read alike. Names are for display only.
 */

/** Something a teacher can open: a class (grades) or a group (attendance). */
export interface TeachingOption {
  /** What the list is opened with: an assignment id or a group id. */
  id: string;
  groupId: string;
  groupName: string;
  /**
   * The group's grade ("1° PRIMARIA"). Groups are named by letter and subjects
   * exist per grade, so without it "A" in two grades reads as the same choice.
   */
  gradeLevelName?: string;
  /** Present when the picker goes by subject first: what the subject step is keyed by. */
  subjectId?: string;
  subjectName?: string;
}

/** The explicit choices, as the URL carries them — all ids. */
export interface SelectionQuery {
  /** The subject id (by-subject mode). */
  subject?: string;
  /** The group id (group-only mode, attendance). */
  group?: string;
  /** The assignment id (by-subject mode, grades): the class itself. */
  assignment?: string;
}

/** A choice at the subject step. */
export interface SubjectChoice {
  id: string;
  /** What is shown; "Asignatura sin nombre" for a blank one. */
  name: string;
  /** The grade the subject belongs to, when known: what tells two "Matemáticas" apart. */
  grade?: string;
}

const SUBJECT_KEY = "asignatura";
const GROUP_KEY = "grupo";
const ASSIGNMENT_KEY = "clase";

/** How a subject without a name is shown — still a choice, never hidden. */
export const NAMELESS_SUBJECT = "Asignatura sin nombre";

export type SelectionStep = "empty" | "subject" | "group" | "ready";

export interface ResolvedSelection<T extends TeachingOption> {
  step: SelectionStep;
  /** Distinct subjects, alphabetical by name. Empty when not picking by subject. */
  subjects: SubjectChoice[];
  /** The options offered at the group step (the chosen subject's classes). */
  groups: T[];
  subject: SubjectChoice | undefined;
  selected: T | undefined;
  /** A step with exactly one choice is taken for the teacher. */
  subjectSkipped: boolean;
  groupSkipped: boolean;
}

/** The subject's name as a person reads it. */
export function subjectLabel(option: Pick<TeachingOption, "subjectName">): string {
  const name = option.subjectName?.trim() ?? "";
  return name === "" ? NAMELESS_SUBJECT : name;
}

const gradeOf = (option: Pick<TeachingOption, "gradeLevelName">): string => option.gradeLevelName?.trim() ?? "";

/** The group as a person reads it: "1° PRIMARIA · A", or just "A" when the grade is unknown. */
export function groupLabel(option: Pick<TeachingOption, "groupName" | "gradeLevelName">): string {
  const grade = gradeOf(option);
  return grade === "" ? option.groupName : `${grade} · ${option.groupName}`;
}

const byLabel = (a: TeachingOption, b: TeachingOption): number =>
  groupLabel(a).localeCompare(groupLabel(b), "es", { numeric: true }) || a.id.localeCompare(b.id);

export function parseSelection(search: string): SelectionQuery {
  const params = new URLSearchParams(search);
  const read = (key: string): string | undefined => {
    const value = params.get(key) ?? "";
    return value === "" ? undefined : value;
  };
  const subject = read(SUBJECT_KEY);
  const group = read(GROUP_KEY);
  const assignment = read(ASSIGNMENT_KEY);
  return {
    ...(subject !== undefined ? { subject } : {}),
    ...(group !== undefined ? { group } : {}),
    ...(assignment !== undefined ? { assignment } : {}),
  };
}

export function selectionToSearch(query: SelectionQuery): string {
  const params = new URLSearchParams();
  if (query.subject !== undefined) params.set(SUBJECT_KEY, query.subject);
  if (query.group !== undefined) params.set(GROUP_KEY, query.group);
  if (query.assignment !== undefined) params.set(ASSIGNMENT_KEY, query.assignment);
  const qs = params.toString();
  return qs === "" ? "" : `?${qs}`;
}

const subjectKey = (option: TeachingOption): string => option.subjectId ?? "";

function distinctSubjects(options: readonly TeachingOption[]): SubjectChoice[] {
  const byId = new Map<string, SubjectChoice>();
  for (const o of options) {
    const id = subjectKey(o);
    const grade = gradeOf(o);
    if (!byId.has(id)) byId.set(id, { id, name: subjectLabel(o), ...(grade !== "" ? { grade } : {}) });
  }
  return [...byId.values()].sort(
    (a, b) =>
      a.name.localeCompare(b.name, "es") ||
      (a.grade ?? "").localeCompare(b.grade ?? "", "es", { numeric: true }) ||
      a.id.localeCompare(b.id),
  );
}

/**
 * Turns the teacher's options and the URL's choices into the current step. A
 * step with a single choice is skipped, and a choice the teacher no longer has
 * (a stale link) counts as not made.
 */
export function resolveSelection<T extends TeachingOption>(
  options: readonly T[],
  query: SelectionQuery,
  bySubject: boolean,
): ResolvedSelection<T> {
  const subjects = bySubject ? distinctSubjects(options) : [];
  const subjectSkipped = !bySubject || subjects.length === 1;
  const subject = !bySubject ? undefined : subjectSkipped ? subjects[0] : subjects.find((s) => s.id === query.subject);
  const groups = (bySubject ? options.filter((o) => subject !== undefined && subjectKey(o) === subject.id) : [...options]).sort(byLabel);
  const groupSkipped = groups.length === 1;
  const matches = (o: T): boolean => (bySubject ? o.id === query.assignment : o.groupId === query.group);
  const selected = groupSkipped ? groups[0] : groups.find(matches);

  const step: SelectionStep =
    options.length === 0 ? "empty" : bySubject && subject === undefined ? "subject" : selected === undefined ? "group" : "ready";

  return { step, subjects, groups, subject, selected, subjectSkipped, groupSkipped };
}
