/**
 * Where a teacher stands in "subject → group → list", read from and written to
 * the query string so the browser's Back returns to the previous step and a
 * reload keeps the place. Pure: no React, no window.
 */

/** Something a teacher can open: a class (grades) or a group (attendance). */
export interface TeachingOption {
  /** What the list is opened with: an assignment id or a group id. */
  id: string;
  groupId: string;
  groupName: string;
  /** Present when the picker goes by subject first. */
  subjectName?: string;
}

/** The explicit choices, as the URL carries them. */
export interface SelectionQuery {
  subject?: string;
  group?: string;
}

const SUBJECT_KEY = "asignatura";
const GROUP_KEY = "grupo";

export type SelectionStep = "empty" | "subject" | "group" | "ready";

export interface ResolvedSelection<T extends TeachingOption> {
  step: SelectionStep;
  /** Distinct subject names, alphabetical. Empty when not picking by subject. */
  subjects: string[];
  /** The groups offered at the group step (those of the chosen subject). */
  groups: T[];
  subject: string | undefined;
  selected: T | undefined;
  /** A step with exactly one choice is taken for the teacher. */
  subjectSkipped: boolean;
  groupSkipped: boolean;
}

export function parseSelection(search: string): SelectionQuery {
  const params = new URLSearchParams(search);
  const subject = params.get(SUBJECT_KEY) ?? "";
  const group = params.get(GROUP_KEY) ?? "";
  return { ...(subject !== "" ? { subject } : {}), ...(group !== "" ? { group } : {}) };
}

export function selectionToSearch(query: SelectionQuery): string {
  const params = new URLSearchParams();
  if (query.subject !== undefined) params.set(SUBJECT_KEY, query.subject);
  if (query.group !== undefined) params.set(GROUP_KEY, query.group);
  const qs = params.toString();
  return qs === "" ? "" : `?${qs}`;
}

function distinctSubjects(options: readonly TeachingOption[]): string[] {
  const names = new Set(options.flatMap((o) => (o.subjectName === undefined ? [] : [o.subjectName])));
  return [...names].sort((a, b) => a.localeCompare(b, "es"));
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
  const subject = !bySubject ? undefined : subjectSkipped ? subjects[0] : subjects.find((s) => s === query.subject);
  const groups = bySubject ? options.filter((o) => subject !== undefined && o.subjectName === subject) : [...options];
  const groupSkipped = groups.length === 1;
  const selected = groupSkipped ? groups[0] : groups.find((g) => g.groupId === query.group);

  const step: SelectionStep =
    options.length === 0 ? "empty" : bySubject && subject === undefined ? "subject" : selected === undefined ? "group" : "ready";

  return { step, subjects, groups, subject, selected, subjectSkipped, groupSkipped };
}
