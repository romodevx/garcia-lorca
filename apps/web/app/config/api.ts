/**
 * Typed fetch helpers for the gateway's academic-catalog endpoints
 * (`/api/v1/config/*`), gated behind the `module_config` flag + cookie auth. The
 * session lives ONLY in the HttpOnly cookie the gateway set, so every call uses
 * `credentials:"include"`; nothing secret ever travels in the body or response.
 *
 * Reads require `config:read`, writes `config:write` (the owner role holds both).
 * These helpers mirror `app/auth/api.ts`: a `url()` trailing-slash guard, throw on
 * any non-2xx, and parse the gateway's `{cycles:[…]}`/`{levels:[…]}`/… envelopes.
 * The shapes match the gateway REST contract exactly; dates are `YYYY-MM-DD`.
 */

// --- entity types (match the gateway JSON contract) -----------------------

import type { DeferredInit } from "../_lib/deferredInit";
import { matchesSearch } from "../_design/textSearch";
import { authFetch } from "../auth/authFetch";
import type { AttendanceMode } from "../attendance/sheet";
import { sortBySchoolOrder } from "./schoolOrder";

export type CycleStatus = "draft" | "active" | "closed";

export interface Cycle {
  id: string;
  name: string;
  status: CycleStatus;
  startsOn: string;
  endsOn: string;
}

export interface CreateCycleInput {
  name: string;
  /** Omit to let the server default to "draft". */
  status?: CycleStatus;
  startsOn: string;
  endsOn: string;
}

export type EnrollmentGranularity = "cycle" | "term";

export interface Level {
  id: string;
  name: string;
  sepCode: string;
  enrollmentGranularity: EnrollmentGranularity;
  /** How the level takes attendance; absent from older gateways (read as "daily"). */
  attendanceMode?: AttendanceMode;
  /** How the level grades; absent from older gateways (read as the 5–10 default). */
  grading?: LevelGrading;
  /** Its place in the school's order (first = 1); the server lists levels by it. */
  sortOrder?: number;
}

/** A level's numeric grading: preescolar usually has none (`enabled:false`). */
export interface LevelGrading {
  enabled: boolean;
  min: number;
  max: number;
  /** 0, 1 or 2. */
  decimals: number;
  passing: number;
}

export interface CreateLevelInput {
  name: string;
  sepCode: string;
  enrollmentGranularity: EnrollmentGranularity;
}

export interface GradeLevel {
  id: string;
  levelId: string;
  name: string;
  ordinal: number;
}

export interface CreateGradeLevelInput {
  levelId: string;
  name: string;
  ordinal: number;
}

export interface Subject {
  id: string;
  gradeLevelId: string;
  name: string;
}

export interface CreateSubjectInput {
  gradeLevelId: string;
  name: string;
}

export interface Term {
  id: string;
  cycleId: string;
  levelId: string;
  name: string;
  ordinal: number;
  startsOn: string;
  endsOn: string;
  /**
   * The cycle's months ("YYYY-MM") this term grades — configured per cycle and
   * level. Absent from older gateways (read as none).
   */
  months?: string[];
}

export interface CopyTermMonthsInput {
  cycleId: string;
  fromLevelId: string;
  toLevelIds: string[];
}

export interface CreateTermInput {
  cycleId: string;
  levelId: string;
  name: string;
  ordinal: number;
  startsOn: string;
  endsOn: string;
}

export interface Group {
  id: string;
  cycleId: string;
  gradeLevelId: string;
  name: string;
  capacity: number;
}

export interface CreateGroupInput {
  cycleId: string;
  gradeLevelId: string;
  name: string;
  capacity: number;
}

// --- transport helpers ----------------------------------------------------

const BASE_PATH = "/api/v1/config";

function url(baseURL: string, path: string): string {
  return `${baseURL.replace(/\/+$/, "")}${path}`;
}

/** POST a JSON body and return the created entity. Throws on any non-2xx. */
/**
 * A config failure the UI can act on.
 *
 * The status is the point. Creating a cycle fails in exactly two ways a person
 * causes — a name that already exists, and an end date before the start — and
 * they need different advice. Throwing a bare Error left the catalog with one
 * sentence for both, and for the duplicate that sentence was "intenta de nuevo",
 * which is advice that can never work.
 */
export class ConfigError extends Error {
  readonly status: number;
  /** The server's own words, when it sent any. */
  readonly detail: string;
  /** The refusal's machine code (`X-Error-Code`), e.g. SCALE_TIGHTENING. */
  readonly code: string | undefined;

  constructor(status: number, detail: string, code?: string) {
    super(detail !== "" ? detail : `HTTP ${status}`);
    this.name = "ConfigError";
    this.status = status;
    this.detail = detail;
    this.code = code;
  }

  /** The thing already exists. Retrying the same input never helps. */
  get isConflict(): boolean {
    return this.status === 409;
  }

  /** The input itself is wrong; changing it will help. */
  get isInvalid(): boolean {
    return this.status === 400;
  }
}

async function configFailure(res: Response): Promise<ConfigError> {
  const body = await res.text().catch(() => "");
  return new ConfigError(res.status, body.trim(), res.headers.get("X-Error-Code") ?? undefined);
}

async function postJSON<T>(baseURL: string, path: string, body: unknown): Promise<T> {
  const res = await authFetch(url(baseURL, `${BASE_PATH}${path}`), {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw await configFailure(res);
  }
  return (await res.json()) as T;
}

/** Sends a JSON body with `method` and returns the JSON answer. Throws on any non-2xx. */
async function sendJSON<T>(baseURL: string, method: "PUT" | "PATCH", path: string, body: unknown): Promise<T> {
  const res = await authFetch(url(baseURL, `${BASE_PATH}${path}`), {
    method,
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw await configFailure(res);
  }
  return (await res.json()) as T;
}

/** DELETEs one resource (204). Throws on any non-2xx. */
async function deleteResource(baseURL: string, path: string): Promise<void> {
  const res = await authFetch(url(baseURL, `${BASE_PATH}${path}`), { method: "DELETE", credentials: "include" });
  if (!res.ok) {
    throw await configFailure(res);
  }
}

/** GET a list endpoint and unwrap the single-key envelope (`{ <key>: [...] }`). */
async function getList<T>(baseURL: string, path: string, key: string): Promise<T[]> {
  const res = await authFetch(url(baseURL, `${BASE_PATH}${path}`), { credentials: "include" });
  if (!res.ok) {
    throw await configFailure(res);
  }
  const data = (await res.json()) as Record<string, T[] | undefined>;
  return data[key] ?? [];
}

/** Builds `?a=1&b=2`, skipping empty values, so optional filters simply drop. */
function query(params: Record<string, string | undefined>): string {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== "") {
      search.set(key, value);
    }
  }
  const qs = search.toString();
  return qs === "" ? "" : `?${qs}`;
}

// --- school calendar (M16) -------------------------------------------------

/** What the school calls a closed day. Two kinds, because a public holiday is
 * not a vacation the school chose — the billing rules treat them identically,
 * the activity calendar does not. */
export type NonWorkingKind = "holiday" | "vacation";

/** A day, or a run of days, the school does not operate. Both ends inclusive. */
export interface NonWorkingPeriod {
  id: string;
  cycleId: string;
  label: string;
  kind: NonWorkingKind;
  startsOn: string;
  endsOn: string;
}

export interface CreateNonWorkingPeriodInput {
  cycleId: string;
  label: string;
  kind: NonWorkingKind;
  /** "2026-09-16". A calendar day, never a timestamp. */
  startsOn: string;
  /** Omitted for a single day — the gateway fills it in from startsOn. */
  endsOn?: string;
}

/** Adds a holiday, vacation or suspension to a cycle. */
export function createNonWorkingPeriod(baseURL: string, input: CreateNonWorkingPeriodInput): Promise<NonWorkingPeriod> {
  return postJSON<NonWorkingPeriod>(baseURL, "/calendar", {
    cycleId: input.cycleId,
    label: input.label,
    kind: input.kind,
    startsOn: input.startsOn,
    ...(input.endsOn !== undefined && input.endsOn !== "" ? { endsOn: input.endsOn } : {}),
  });
}

/** A cycle's closed days, earliest first. */
export function listNonWorkingPeriods(baseURL: string, cycleId: string): Promise<NonWorkingPeriod[]> {
  return getList<NonWorkingPeriod>(baseURL, `/calendar${query({ cycleId })}`, "periods");
}

/**
 * Removes one period. A day entered by mistake has to be removable, or the
 * calendar stops being trusted and the school goes back to a sheet on the wall.
 */
export async function deleteNonWorkingPeriod(baseURL: string, id: string, init: DeferredInit = {}): Promise<void> {
  const res = await authFetch(url(baseURL, `${BASE_PATH}/calendar/${id}`), {
    ...init,
    method: "DELETE",
    credentials: "include",
  });
  if (!res.ok) {
    throw await configFailure(res);
  }
}

// --- school agenda (M16) ---------------------------------------------------

/**
 * Something the school DOES: a festival, a parents' meeting, an exam week.
 *
 * It is not a closed day, and the difference is not cosmetic. A closed day
 * moves a due date and stops a surcharge; an event never may. A festival on the
 * 10th does not mean nobody could pay on the 10th.
 */
export interface SchoolEvent {
  id: string;
  cycleId: string;
  title: string;
  description: string;
  startsOn: string;
  endsOn: string;
  /** At most one is set; neither means the whole school. */
  levelId?: string;
  groupId?: string;
}

export interface CreateSchoolEventInput {
  cycleId: string;
  title: string;
  description?: string;
  startsOn: string;
  /** Omitted for a one-day event — the gateway fills it in. */
  endsOn?: string;
  levelId?: string;
  groupId?: string;
}

/** Records something the school does. */
export function createSchoolEvent(baseURL: string, input: CreateSchoolEventInput): Promise<SchoolEvent> {
  return postJSON<SchoolEvent>(baseURL, "/events", {
    cycleId: input.cycleId,
    title: input.title,
    description: input.description ?? "",
    startsOn: input.startsOn,
    ...(input.endsOn !== undefined && input.endsOn !== "" ? { endsOn: input.endsOn } : {}),
    ...(input.levelId !== undefined && input.levelId !== "" ? { levelId: input.levelId } : {}),
    ...(input.groupId !== undefined && input.groupId !== "" ? { groupId: input.groupId } : {}),
  });
}

/** A cycle's agenda, earliest first. */
export function listSchoolEvents(baseURL: string, cycleId: string): Promise<SchoolEvent[]> {
  return getList<SchoolEvent>(baseURL, `/events${query({ cycleId })}`, "events");
}

/** Removes a date entered by mistake. */
export async function deleteSchoolEvent(baseURL: string, id: string, init: DeferredInit = {}): Promise<void> {
  const res = await authFetch(url(baseURL, `${BASE_PATH}/events/${id}`), {
    ...init,
    method: "DELETE",
    credentials: "include",
  });
  if (!res.ok) {
    throw await configFailure(res);
  }
}

// --- cycles ---------------------------------------------------------------

export function createCycle(baseURL: string, input: CreateCycleInput): Promise<Cycle> {
  return postJSON<Cycle>(baseURL, "/cycles", input);
}

/**
 * Makes a cycle the school's live year, standing down whichever other was
 * active. Everything downstream asks "which cycle is active" — the roster
 * header, the group dropdowns, the tuition table, the billing run.
 */
export function activateCycle(baseURL: string, id: string): Promise<Cycle> {
  return postJSON<Cycle>(baseURL, `/cycles/${encodeURIComponent(id)}/activate`, {});
}

/** Gives a cycle a new name; its dates and status stay. */
export function renameCycle(baseURL: string, id: string, name: string): Promise<Cycle> {
  return sendJSON<Cycle>(baseURL, "PATCH", `/cycles/${encodeURIComponent(id)}`, { name });
}

export function listCycles(baseURL: string): Promise<Cycle[]> {
  return getList<Cycle>(baseURL, "/cycles", "cycles");
}

// --- levels ---------------------------------------------------------------

export function createLevel(baseURL: string, input: CreateLevelInput): Promise<Level> {
  return postJSON<Level>(baseURL, "/levels", input);
}

export function listLevels(baseURL: string): Promise<Level[]> {
  return getList<Level>(baseURL, "/levels", "levels");
}

/**
 * Puts the levels in school order: the WHOLE list, first level first. A list
 * that is not exactly the school's levels is 400 LEVEL_ORDER_MISMATCH.
 */
export async function reorderLevels(baseURL: string, levelIds: readonly string[]): Promise<Level[]> {
  const data = await sendJSON<{ levels?: Level[] }>(baseURL, "PUT", "/levels/order", { levelIds });
  return data.levels ?? [];
}

/** PATCH one level with a partial body; the server answers with the whole level. */
async function patchLevel(baseURL: string, id: string, body: Partial<Pick<Level, "attendanceMode" | "grading">>): Promise<Level> {
  const res = await authFetch(url(baseURL, `${BASE_PATH}/levels/${encodeURIComponent(id)}`), {
    method: "PATCH",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw await configFailure(res);
  }
  return (await res.json()) as Level;
}

/** Sets how a level takes attendance: one list per day, or one per class. */
export function updateLevelAttendanceMode(baseURL: string, id: string, attendanceMode: AttendanceMode): Promise<Level> {
  return patchLevel(baseURL, id, { attendanceMode });
}

/** Sets how a level grades: whether it does at all, its scale and passing mark. */
export function updateLevelGrading(baseURL: string, id: string, grading: LevelGrading): Promise<Level> {
  return patchLevel(baseURL, id, { grading });
}

// --- level deletion + trash -------------------------------------------------

/** How much hangs off a level — what deleting it takes away (or brought back). */
export interface LevelDeletionCounts {
  gradeLevels: number;
  groups: number;
  subjects: number;
  terms: number;
  teachingAssignments: number;
  students: number;
  enrollments: number;
  attendanceMarks: number;
  monthlyGrades: number;
  reportCards: number;
  tuitionPlans: number;
  uniformItems: number;
  schoolEvents: number;
  /** Newer kinds (ADR 0021, confirmation review): absent from an older server. */
  enrollmentTransfers?: number;
  attendanceSessions?: number;
  gradeEntries?: number;
  /** Grades by evaluation period (T15). */
  periodGrades?: number;
  periodClosures?: number;
}

/**
 * What deleting a level would do, and the token that proves the director read
 * THIS summary: if anything changes before the delete, the server answers
 * PREVIEW_STALE instead of deleting what nobody saw.
 */
export interface LevelDeletionPreview {
  levelId: string;
  levelName: string;
  /** No grades and no pupils: the level goes entirely. */
  empty: boolean;
  counts: LevelDeletionCounts;
  /** Money is never deleted with a level. */
  keeps: { charges: number; payments: number };
  confirmToken: string;
  expiresAt: string;
}

export interface DeleteLevelInput {
  confirmName: string;
  confirmToken: string;
  password: string;
}

export interface LevelDeletionResult {
  batchId: string;
  deletedAt: string;
  restorableUntil: string;
  counts: LevelDeletionCounts;
}

/** One deleted level waiting in the trash for its 7 days. */
export interface LevelTrashItem {
  batchId: string;
  levelId: string;
  levelName: string;
  deletedAt: string;
  restorableUntil: string;
  deletedByName: string;
  counts: LevelDeletionCounts;
  status: string;
}

/** Rows the restore could not bring back, and why (a pupil already re-enrolled…). */
export interface RestoreSkipped {
  kind: string;
  count: number;
  reason: string;
}

export interface LevelRestoreResult {
  restored: Partial<LevelDeletionCounts>;
  skipped: RestoreSkipped[];
}

/** GET /levels/{id}/deletion-preview — the summary the director confirms. */
export async function getLevelDeletionPreview(baseURL: string, levelId: string): Promise<LevelDeletionPreview> {
  const res = await authFetch(url(baseURL, `${BASE_PATH}/levels/${encodeURIComponent(levelId)}/deletion-preview`), { credentials: "include" });
  if (!res.ok) {
    throw await configFailure(res);
  }
  return (await res.json()) as LevelDeletionPreview;
}

/**
 * POST /levels/{id}/delete. Refusals carry their code: PREVIEW_STALE (409),
 * NAME_MISMATCH (400), REAUTH_FAILED (401 — the password, not the session).
 */
export function deleteLevel(baseURL: string, levelId: string, input: DeleteLevelInput): Promise<LevelDeletionResult> {
  return postJSON<LevelDeletionResult>(baseURL, `/levels/${encodeURIComponent(levelId)}/delete`, input);
}

/** GET /level-trash — deleted levels still inside their restore window. */
export function listLevelTrash(baseURL: string): Promise<LevelTrashItem[]> {
  return getList<LevelTrashItem>(baseURL, "/level-trash", "items");
}

/** POST /level-trash/{batchId}/restore. LEVEL_NAME_TAKEN (409) and TRASH_EXPIRED (410) refuse. */
export async function restoreLevel(baseURL: string, batchId: string): Promise<LevelRestoreResult> {
  const out = await postJSON<Partial<LevelRestoreResult>>(baseURL, `/level-trash/${encodeURIComponent(batchId)}/restore`, {});
  return { restored: out.restored ?? {}, skipped: out.skipped ?? [] };
}

// --- grade levels ---------------------------------------------------------

export function createGradeLevel(baseURL: string, input: CreateGradeLevelInput): Promise<GradeLevel> {
  return postJSON<GradeLevel>(baseURL, "/grade-levels", input);
}

export function listGradeLevels(baseURL: string, levelId?: string): Promise<GradeLevel[]> {
  return getList<GradeLevel>(baseURL, `/grade-levels${query({ levelId })}`, "gradeLevels");
}

/** Renames a grade in place; a name the level already has is 409 GRADE_NAME_TAKEN. */
export function renameGradeLevel(baseURL: string, id: string, name: string): Promise<GradeLevel> {
  return sendJSON<GradeLevel>(baseURL, "PATCH", `/grade-levels/${encodeURIComponent(id)}`, { name });
}

/** Deletes a grade that holds nothing; otherwise 409 GRADE_LEVEL_IN_USE says what blocks it. */
export function deleteGradeLevel(baseURL: string, id: string): Promise<void> {
  return deleteResource(baseURL, `/grade-levels/${encodeURIComponent(id)}`);
}

// --- subjects -------------------------------------------------------------

export function createSubject(baseURL: string, input: CreateSubjectInput): Promise<Subject> {
  return postJSON<Subject>(baseURL, "/subjects", input);
}

export function listSubjects(baseURL: string, gradeLevelId: string): Promise<Subject[]> {
  return getList<Subject>(baseURL, `/subjects${query({ gradeLevelId })}`, "subjects");
}

/** Renames a subject in place: same id and grade, so grades and assignments follow it. */
export async function renameSubject(baseURL: string, id: string, name: string): Promise<Subject> {
  const res = await authFetch(url(baseURL, `${BASE_PATH}/subjects/${encodeURIComponent(id)}`), {
    method: "PATCH",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
  });
  if (!res.ok) {
    throw await configFailure(res);
  }
  return (await res.json()) as Subject;
}

/** Deletes a subject. One that was ever in a teacher's load answers 409 (`isConflict`). */
export async function deleteSubject(baseURL: string, id: string): Promise<void> {
  const res = await authFetch(url(baseURL, `${BASE_PATH}/subjects/${encodeURIComponent(id)}`), {
    method: "DELETE",
    credentials: "include",
  });
  if (!res.ok) {
    throw await configFailure(res);
  }
}

/** What adding one subject to several grades did, grade by grade. */
export interface SubjectsInGrades {
  created: Subject[];
  /** Grades that already had a subject with this name (409): nothing to do there. */
  existing: string[];
  /** Grades where the request failed for another reason. */
  failed: string[];
}

/**
 * Adds the subject to each grade — one row per grade, since subjects are per
 * grade (T13). Each grade stands alone: one that already has the name does not
 * stop the others, and the caller is told which was which.
 */
export async function createSubjectInGrades(baseURL: string, name: string, gradeLevelIds: readonly string[]): Promise<SubjectsInGrades> {
  const results = await Promise.allSettled(gradeLevelIds.map((gradeLevelId) => createSubject(baseURL, { gradeLevelId, name })));
  const out: SubjectsInGrades = { created: [], existing: [], failed: [] };
  results.forEach((r, i) => {
    const gradeId = gradeLevelIds[i] as string;
    if (r.status === "fulfilled") out.created.push(r.value);
    else if (r.reason instanceof ConfigError && r.reason.isConflict) out.existing.push(gradeId);
    else out.failed.push(gradeId);
  });
  return out;
}

// --- terms ----------------------------------------------------------------

export function createTerm(baseURL: string, input: CreateTermInput): Promise<Term> {
  return postJSON<Term>(baseURL, "/terms", input);
}

export function listTerms(baseURL: string, cycleId: string, levelId?: string): Promise<Term[]> {
  return getList<Term>(baseURL, `/terms${query({ cycleId, levelId })}`, "terms");
}

/**
 * Replaces the months a term grades. A month belongs to one term only: the
 * server refuses one still held by another (409 MONTH_IN_OTHER_TERM) or one
 * outside the cycle (400 MONTH_OUTSIDE_CYCLE).
 */
export async function setTermMonths(baseURL: string, termId: string, months: readonly string[]): Promise<Term> {
  const res = await authFetch(url(baseURL, `${BASE_PATH}/terms/${encodeURIComponent(termId)}/months`), {
    method: "PUT",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ months }),
  });
  if (!res.ok) {
    throw await configFailure(res);
  }
  return (await res.json()) as Term;
}

/** Copies one level's month-to-term layout onto other levels of the same cycle. */
export async function copyTermMonths(baseURL: string, input: CopyTermMonthsInput): Promise<Term[]> {
  const data = await postJSON<{ terms?: Term[] }>(baseURL, "/terms/copy-months", input);
  return data.terms ?? [];
}

// --- evaluation periods per grade (T14) -----------------------------------

/**
 * One evaluation period (trimester, bimester…) of ONE grade in a cycle. Dates
 * are `YYYY-MM-DD`, both inclusive. `termId` names the level term a migrated
 * period came from (provenance), or null.
 */
export interface EvaluationPeriod {
  id: string;
  cycleId: string;
  gradeLevelId: string;
  ordinal: number;
  name: string;
  startsOn: string;
  endsOn: string;
  termId: string | null;
}

/** One period of a grade's new set; `id` is the saved period the row edits. */
export interface EvaluationPeriodInput {
  id?: string;
  ordinal: number;
  name: string;
  startsOn: string;
  endsOn: string;
}

export interface CopyEvaluationPeriodsInput {
  cycleId: string;
  fromGradeLevelId: string;
  toGradeLevelIds: string[];
}

/** The periods of a cycle, narrowed to a level's grades and/or one grade. */
export function listEvaluationPeriods(
  baseURL: string,
  scope: { cycleId: string; levelId?: string; gradeLevelId?: string },
): Promise<EvaluationPeriod[]> {
  return getList<EvaluationPeriod>(baseURL, `/evaluation-periods${query(scope)}`, "periods");
}

/**
 * Replaces one grade's periods in a cycle (the complete set; `[]` clears it).
 * The server refuses overlaps (409 PERIOD_OVERLAP) and dates outside the cycle
 * (400 PERIOD_OUTSIDE_CYCLE) — the screen checks both first.
 */
export async function saveGradeEvaluationPeriods(
  baseURL: string,
  gradeLevelId: string,
  cycleId: string,
  periods: readonly EvaluationPeriodInput[],
): Promise<EvaluationPeriod[]> {
  const res = await authFetch(url(baseURL, `${BASE_PATH}/grade-levels/${encodeURIComponent(gradeLevelId)}/evaluation-periods`), {
    method: "PUT",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ cycleId, periods }),
  });
  if (!res.ok) {
    throw await configFailure(res);
  }
  const data = (await res.json()) as { periods?: EvaluationPeriod[] };
  return data.periods ?? [];
}

/** Gives every target grade the source grade's periods, in one transaction. */
export async function copyEvaluationPeriods(baseURL: string, input: CopyEvaluationPeriodsInput): Promise<EvaluationPeriod[]> {
  const data = await postJSON<{ periods?: EvaluationPeriod[] }>(baseURL, "/evaluation-periods/copy", input);
  return data.periods ?? [];
}

// --- groups ---------------------------------------------------------------

export function createGroup(baseURL: string, input: CreateGroupInput): Promise<Group> {
  return postJSON<Group>(baseURL, "/groups", input);
}

export function listGroups(baseURL: string, cycleId: string, gradeLevelId?: string): Promise<Group[]> {
  return getList<Group>(baseURL, `/groups${query({ cycleId, gradeLevelId })}`, "groups");
}

/** Renames a group in place; a name its grade already has in the cycle is 409 GROUP_NAME_TAKEN. */
export function renameGroup(baseURL: string, id: string, name: string): Promise<Group> {
  return sendJSON<Group>(baseURL, "PATCH", `/groups/${encodeURIComponent(id)}`, { name });
}

/** Deletes a group nothing ever used; otherwise 409 GROUP_IN_USE names each blocker. */
export function deleteGroup(baseURL: string, id: string): Promise<void> {
  return deleteResource(baseURL, `/groups/${encodeURIComponent(id)}`);
}

/** What labelGroups reads of a group; the schedule board's slim copy fits too. */
export type GroupRef = Pick<Group, "id" | "gradeLevelId" | "name">;

/** The names labelGroups adds to a group. */
export interface GroupLabelFields {
  label: string;
  /** The grade's own name ("1°"); absent when the grade is unknown. */
  gradeName?: string;
  /** The grade's level id; absent when the grade is unknown. */
  levelId?: string;
  /** The grade's ordinal; absent when the grade is unknown. */
  ordinal?: number;
}

/** A group named for a picker: its grade and its letter, "1° PRIMARIA · A". */
export interface LabelledGroup extends Group, GroupLabelFields {}

/**
 * The cycle's groups, each labelled with its grade. Every grade has an "A", so
 * a picker that offers the bare name offers "A, A, A, B". When the grades
 * cannot be read the label falls back to the bare name: a worse picker, not a
 * broken one.
 */
export async function listLabelledGroups(baseURL: string, cycleId: string, gradeLevelId?: string): Promise<LabelledGroup[]> {
  const [groups, grades, levels] = await Promise.all([
    listGroups(baseURL, cycleId, gradeLevelId),
    listGradeLevels(baseURL).catch((): GradeLevel[] => []),
    listLevels(baseURL).catch((): Level[] => []),
  ]);
  return labelGroups(groups, grades, levels);
}

/**
 * Labels groups from grades already in hand; an unknown grade keeps the bare name.
 * The config screen asks for the grade "as the school names it", so "1°" under
 * PRIMARIA and "1°" under SECUNDARIA are both normal: when the grade's name does
 * not already carry its level, the level is added ("1° PRIMARIA · A").
 * They come back in school order: level, grade, then name.
 */
export function labelGroups<G extends GroupRef>(
  groups: readonly G[],
  grades: readonly GradeLevel[],
  levels: readonly Pick<Level, "id" | "name" | "sortOrder">[] = [],
): (G & GroupLabelFields)[] {
  const levelName = new Map(levels.map((l) => [l.id, l.name]));
  const gradeById = new Map(grades.map((g) => [g.id, g]));
  // In school order (ISSUE-018): every picker of groups reads them from here.
  return sortBySchoolOrder(groups, { levels, grades }).map((g) => {
    const grade = gradeById.get(g.gradeLevelId);
    if (grade === undefined) return { ...g, label: g.name };
    return {
      ...g,
      label: `${gradeWithLevel(grade.name, levelName.get(grade.levelId))} · ${g.name}`,
      gradeName: grade.name,
      levelId: grade.levelId,
      ordinal: grade.ordinal,
    };
  });
}

function gradeWithLevel(grade: string, level: string | undefined): string {
  if (level === undefined || matchesSearch(level, grade)) return grade;
  return `${grade} ${level}`;
}


// --- the school's details (task 12) ---------------------------------------

/** What a receipt prints about the school. Only the name is required. */
export interface SchoolProfile {
  name: string;
  legalName: string;
  rfc: string;
  cct: string;
  address: string;
  phone: string;
  legend: string;
  /** The account families transfer to (18 digits), shown in the family app; "" when none. */
  clabe: string;
  /** The CLABE's bank as the server names it; output only. */
  clabeBank?: string;
}

/** The details, and whether the school has filled them yet. */
export interface SchoolProfileRead {
  profile: SchoolProfile;
  found: boolean;
}

/** GET /api/v1/config/school-profile. */
export async function getSchoolProfile(baseURL: string): Promise<SchoolProfileRead> {
  const res = await authFetch(url(baseURL, `${BASE_PATH}/school-profile`), { credentials: "include" });
  if (!res.ok) {
    throw await configFailure(res);
  }
  return withClabe((await res.json()) as SchoolProfileRead);
}

/** A gateway older than the CLABE answers without it: the form needs "". */
function withClabe(read: SchoolProfileRead): SchoolProfileRead {
  return { ...read, profile: { ...read.profile, clabe: read.profile.clabe ?? "" } };
}

/** PUT /api/v1/config/school-profile: every field is replaced. */
export async function saveSchoolProfile(baseURL: string, profile: SchoolProfile): Promise<SchoolProfile> {
  const res = await authFetch(url(baseURL, `${BASE_PATH}/school-profile`), {
    method: "PUT",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(profile),
  });
  if (!res.ok) {
    throw await configFailure(res);
  }
  return ((await res.json()) as { profile: SchoolProfile }).profile;
}
