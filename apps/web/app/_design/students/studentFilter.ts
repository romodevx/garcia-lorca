/**
 * The school's way of narrowing a list of pupils (T4): level → grade → group,
 * chained, and a search by surname or first name.
 *
 * Pure on purpose. The students list, the activities picker (T17) and the
 * scholarships picker (T18) all narrow pupils the same way; the React hook and
 * the chips are thin shells over these functions, and every rule is tested here.
 */
import { matchesSearch } from "../textSearch";
import { compareStudents, studentNameSearchKeys, type StudentNameParts } from "./studentName";

/** The chip that stands for "has none": a pupil with no level, grade or group. */
export const NONE = "none";

/** What is chosen. "" means "any"; NONE means "the ones without". */
export interface StudentFilterState {
  levelId: string;
  gradeId: string;
  groupId: string;
  /** The search box, as typed. */
  q: string;
}

export const EMPTY_FILTER: StudentFilterState = { levelId: "", gradeId: "", groupId: "", q: "" };

/**
 * A pupil as the filter sees it: the name, and where they sit this cycle.
 * `""` in a placement id means the pupil has none (a deleted level, no group).
 */
export interface FilterableStudent extends StudentNameParts {
  id: string;
  levelId: string;
  gradeLevelId: string;
  groupId: string;
  /** More texts the search box matches (CURP, tutor), beyond the name. */
  searchText?: readonly string[];
}

/** The school's structure, as configured: the chips come from here. */
export interface FilterCatalog {
  levels: readonly { id: string; name: string }[];
  grades: readonly { id: string; levelId: string; name: string; ordinal: number }[];
  groups: readonly { id: string; gradeLevelId: string; name: string }[];
}

/** One chip: what it says and how many pupils a click on it shows. */
export interface FacetOption {
  id: string;
  label: string;
  count: number;
}

/** The three rows of chips. A row is empty until its parent is chosen. */
export interface StudentFacets {
  levels: FacetOption[];
  grades: FacetOption[];
  groups: FacetOption[];
}

const NO_LEVEL_LABEL = "Sin nivel";
const NO_GRADE_LABEL = "Sin grado";
const NO_GROUP_LABEL = "Sin grupo";
/** A chosen id the catalog no longer has: an old link, a deleted group. */
const GONE_LABEL = "(ya no existe)";

/** Choosing a level forgets grade and group; choosing it again turns it off. */
export function chooseLevel(f: StudentFilterState, levelId: string): StudentFilterState {
  return { ...f, levelId: f.levelId === levelId ? "" : levelId, gradeId: "", groupId: "" };
}

/** Choosing a grade forgets the group; choosing it again turns it off. */
export function chooseGrade(f: StudentFilterState, gradeId: string): StudentFilterState {
  return { ...f, gradeId: f.gradeId === gradeId ? "" : gradeId, groupId: "" };
}

/** Choosing the chosen group again turns it off. */
export function chooseGroup(f: StudentFilterState, groupId: string): StudentFilterState {
  return { ...f, groupId: f.groupId === groupId ? "" : groupId };
}

/** Whether anything narrows the list (a blank search does not). */
export function isFilterActive(f: StudentFilterState): boolean {
  return f.levelId !== "" || f.gradeId !== "" || f.groupId !== "" || f.q.trim() !== "";
}

/** "" = any; NONE = the pupil has no value; otherwise an exact id. */
function fits(chosen: string, actual: string): boolean {
  if (chosen === "") return true;
  if (chosen === NONE) return actual === "";
  return chosen === actual;
}

function matchesQuery(s: FilterableStudent, q: string): boolean {
  return matchesSearch(q, ...studentNameSearchKeys(s), ...(s.searchText ?? []));
}

/** The pupils the filter keeps, surname first in Spanish order. */
export function applyStudentFilter<T extends FilterableStudent>(items: readonly T[], f: StudentFilterState): T[] {
  return items
    .filter((s) => fits(f.levelId, s.levelId) && fits(f.gradeId, s.gradeLevelId) && fits(f.groupId, s.groupId) && matchesQuery(s, f.q))
    .sort(compareStudents);
}

function countBy(items: readonly FilterableStudent[], key: (s: FilterableStudent) => string): Map<string, number> {
  const counted = new Map<string, number>();
  for (const s of items) {
    const k = key(s);
    counted.set(k, (counted.get(k) ?? 0) + 1);
  }
  return counted;
}

/**
 * The configured options with their counts, and "sin …" only when it holds
 * somebody. The CHOSEN chip always shows, even at zero or unknown: a filter
 * that is on must keep a pressed chip to turn it off.
 */
function facet(
  options: readonly { id: string; label: string }[],
  counted: Map<string, number>,
  noneLabel: string,
  chosen: string,
): FacetOption[] {
  const out = options.map((o) => ({ id: o.id, label: o.label, count: counted.get(o.id) ?? 0 }));
  const none = counted.get("") ?? 0;
  // "Sin …" only beside real options: alone it would say nothing useful.
  if ((none > 0 && options.length > 0) || chosen === NONE) out.push({ id: NONE, label: noneLabel, count: none });
  if (chosen !== "" && !out.some((o) => o.id === chosen)) out.push({ id: chosen, label: GONE_LABEL, count: 0 });
  return out;
}

/**
 * The chips to show and their counts. Each count is taken under the search and
 * the chips above it, so the number on a chip is what a click on it shows.
 */
export function studentFacets(items: readonly FilterableStudent[], catalog: FilterCatalog, f: StudentFilterState): StudentFacets {
  const searched = items.filter((s) => matchesQuery(s, f.q));
  const levels = facet(catalog.levels.map((l) => ({ id: l.id, label: l.name })), countBy(searched, (s) => s.levelId), NO_LEVEL_LABEL, f.levelId);
  if (f.levelId === "" || f.levelId === NONE) return { levels, grades: [], groups: [] };

  const inLevel = searched.filter((s) => s.levelId === f.levelId);
  const gradeOptions = catalog.grades
    .filter((g) => g.levelId === f.levelId)
    .sort((a, b) => a.ordinal - b.ordinal)
    .map((g) => ({ id: g.id, label: g.name }));
  // A level with no grades configured has nothing to choose: no row at all,
  // unless a grade is already chosen (an old link) and must be turned off.
  if (gradeOptions.length === 0 && f.gradeId === "") return { levels, grades: [], groups: [] };
  const grades = facet(gradeOptions, countBy(inLevel, (s) => s.gradeLevelId), NO_GRADE_LABEL, f.gradeId);
  if (f.gradeId === "" || f.gradeId === NONE) return { levels, grades, groups: [] };

  const inGrade = inLevel.filter((s) => s.gradeLevelId === f.gradeId);
  const groupOptions = catalog.groups
    .filter((g) => g.gradeLevelId === f.gradeId)
    .map((g) => ({ id: g.id, label: g.name }))
    .sort((a, b) => a.label.localeCompare(b.label, "es"));
  if (groupOptions.length === 0 && f.groupId === "") return { levels, grades, groups: [] };
  return { levels, grades, groups: facet(groupOptions, countBy(inGrade, (s) => s.groupId), NO_GROUP_LABEL, f.groupId) };
}

/** The address-bar keys, in Spanish like the paths. */
const KEYS = { levelId: "nivel", gradeId: "grado", groupId: "grupo", q: "q" } as const;

/**
 * Writes the filter into the page's query string, keeping the page's other keys
 * (the status tab). Empty values are left out so a clean list has a clean URL.
 */
export function filterToParams(f: StudentFilterState, base: URLSearchParams = new URLSearchParams()): URLSearchParams {
  const out = new URLSearchParams(base);
  for (const [field, key] of Object.entries(KEYS) as [keyof StudentFilterState, string][]) {
    const value = f[field];
    if (value === "") out.delete(key);
    else out.set(key, value);
  }
  return out;
}

/**
 * Reads the filter back. A grade without its level, or a group without its
 * grade, is a hand-edited URL: the chain cannot show it, so it is dropped.
 */
export function filterFromParams(params: URLSearchParams): StudentFilterState {
  const levelId = params.get(KEYS.levelId) ?? "";
  const gradeId = levelId === "" ? "" : (params.get(KEYS.gradeId) ?? "");
  const groupId = gradeId === "" ? "" : (params.get(KEYS.groupId) ?? "");
  return { levelId, gradeId, groupId, q: params.get(KEYS.q) ?? "" };
}
