/**
 * Typed fetch helpers for Horarios (T16): `/api/v1/schedule*` and the
 * teacher's own `/api/v1/teaching/schedule*`. Behind `module_schedule` (and
 * `module_config`) + cookie auth; reads need `schedule:read`, writes
 * `schedule:write`, the teacher's own timetable `teaching:read`. This client is
 * UX only — the gateway is the boundary.
 *
 * Times travel as "HH:MM" strings (5-minute steps, the end exclusive);
 * weekday 1 = Monday … 5 = Friday. A refusal is a sentence shown as is, with
 * its machine code in `X-Error-Code`.
 */
import { authFetch } from "../auth/authFetch";

export type ClassStatus = "regular" | "substitute" | "swapped" | "cancelled" | "uncovered";
export type OverrideKind = "substitute" | "swap" | "cancel";
export type TeacherMatch = "subject" | "grade" | "other";
export type ScheduleAction =
  | "block_created" | "block_updated" | "block_deleted" | "override_created" | "override_deleted"
  | "absence_created" | "absence_updated" | "absence_cancelled";
/** Incapacidad, Vacaciones, Asunto personal, Otro (T19). */
export type AbsenceKind = "sick" | "vacation" | "personal" | "other";

const CLASS_STATUSES: readonly ClassStatus[] = ["regular", "substitute", "swapped", "cancelled", "uncovered"];
const MATCHES: readonly TeacherMatch[] = ["subject", "grade", "other"];

export interface ScheduleCycle {
  id: string;
  name: string;
  startsOn: string;
  endsOn: string;
  active: boolean;
}

export interface ScheduleSettings {
  levelId: string;
  gridStart: string;
  gridEnd: string;
}

export interface ScheduleBlock {
  id: string;
  cycleId: string;
  assignmentId: string;
  groupId: string;
  subjectId: string;
  teacherId: string;
  weekday: number;
  startsAt: string;
  endsAt: string;
}

export interface ScheduleBoard {
  cycle: ScheduleCycle;
  cycles: ScheduleCycle[];
  started: boolean;
  settings: ScheduleSettings[];
  levels: { id: string; name: string }[];
  grades: { id: string; levelId: string; name: string; ordinal: number }[];
  groups: { id: string; gradeLevelId: string; name: string }[];
  subjects: { id: string; gradeLevelId: string; name: string }[];
  teachers: { id: string; firstName: string; lastName: string }[];
  assignments: { id: string; groupId: string; subjectId: string; teacherId: string }[];
  blocks: ScheduleBlock[];
}

/** A class as it really happens on a date: the base plus that day's change. */
export interface ScheduleClass {
  blockId: string;
  groupId: string;
  subjectId: string;
  teacherId: string;
  startsAt: string;
  endsAt: string;
  status: ClassStatus;
  baseTeacherId: string;
  baseStartsAt: string;
  baseEndsAt: string;
  overrideId: string | null;
  reason: string | null;
  /** Why an "uncovered" class has nobody (T19); null otherwise. */
  absence: TeacherAbsence | null;
}

export interface ScheduleDay {
  date: string;
  weekday: number;
  /** The school-calendar period closing the date ("Independencia"); a closed date has no classes. */
  nonWorking: string | null;
  classes: ScheduleClass[];
}

/**
 * A teacher away on startsOn..endsOn (T19), all day or startsAt–endsAt of each
 * date. `kind`, `note` and who registered it reach ONLY schedule:write holders
 * (the gateway shapes them away): everybody else reads "Ausente".
 */
export interface TeacherAbsence {
  id: string;
  cycleId: string;
  teacherId: string;
  startsOn: string;
  endsOn: string;
  allDay: boolean;
  startsAt: string | null;
  endsAt: string | null;
  cancelled: boolean;
  kind?: AbsenceKind;
  note?: string;
  createdByName?: string;
  createdAt?: string;
  cancelledAt?: string;
  cancelledByName?: string;
}

export interface AbsenceInput {
  /** Omitted: the cycle whose dates contain startsOn. */
  cycleId?: string;
  teacherId: string;
  kind: AbsenceKind;
  startsOn: string;
  endsOn: string;
  allDay: boolean;
  startsAt?: string;
  endsAt?: string;
  note?: string;
}

export interface AbsenceSaved {
  absence: TeacherAbsence;
  /** "Quedan N clases sin cubrir en esas fechas". */
  uncoveredCount: number;
}

export interface UncoveredClass {
  date: string;
  class: ScheduleClass;
}

export interface BlockInput {
  assignmentId: string;
  weekday: number;
  startsAt: string;
  endsAt: string;
  /** Required once the cycle has started. */
  reason?: string;
}

export interface OverrideInput {
  blockId: string;
  date: string;
  kind: OverrideKind;
  teacherId?: string;
  swapBlockId?: string;
  reason: string;
  /** T19: a substitute or cancellation of the same class that date to replace (its substitute is absent). */
  replaceOverrideId?: string;
}

export interface AvailableTeacher {
  teacherId: string;
  firstName: string;
  lastName: string;
  match: TeacherMatch;
}

export interface AvailableTeachersQuery {
  cycleId: string;
  groupId: string;
  subjectId: string;
  date?: string;
  weekday?: number;
  startsAt: string;
  endsAt: string;
  excludeBlockId?: string;
}

export interface ScheduleEvent {
  id: string;
  at: string;
  actorId: string;
  actorName: string;
  action: ScheduleAction;
  blockId: string | null;
  overrideId: string | null;
  absenceId?: string | null;
  date: string | null;
  reason: string;
  summary: string;
}

/** A refused timetable request: the sentence to show and its machine code. */
export class ScheduleError extends Error {
  readonly status: number;
  /** `X-Error-Code` (SCHEDULE_TEACHER_BUSY, SCHEDULE_REASON_REQUIRED, …) when sent. */
  readonly code: string | undefined;

  constructor(status: number, sentence: string, code?: string) {
    super(sentence !== "" ? sentence : `HTTP ${status}`);
    this.name = "ScheduleError";
    this.status = status;
    this.code = code;
  }
}

// --- transport -------------------------------------------------------------

const ID = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;

/** An id that is not one never reaches the network (it would be a path). */
function checkedId(id: string): string {
  if (!ID.test(id)) throw new ScheduleError(400, "Ese registro no existe.");
  return encodeURIComponent(id);
}

function query(params: Record<string, string | number | undefined>): string {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== "") search.set(key, String(value));
  }
  const qs = search.toString();
  return qs === "" ? "" : `?${qs}`;
}

async function failure(res: Response): Promise<ScheduleError> {
  const body = (await res.text().catch(() => "")).trim();
  return new ScheduleError(res.status, body, res.headers.get("X-Error-Code") ?? undefined);
}

async function raw(baseURL: string, path: string, init: RequestInit = {}): Promise<Response> {
  const res = await authFetch(`${baseURL.replace(/\/+$/, "")}${path}`, { credentials: "include", ...init });
  if (!res.ok) throw await failure(res);
  return res;
}

async function read<T>(baseURL: string, path: string): Promise<T> {
  return (await (await raw(baseURL, path)).json()) as T;
}

async function send<T>(baseURL: string, path: string, method: "POST" | "PUT", body: unknown): Promise<T> {
  const res = await raw(baseURL, path, { method, headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
  return (await res.json()) as T;
}

async function remove(baseURL: string, path: string): Promise<void> {
  await raw(baseURL, path, { method: "DELETE" });
}

function oneOf<T extends string>(allowed: readonly T[], value: unknown, fallback: T): T {
  return typeof value === "string" && (allowed as readonly string[]).includes(value) ? (value as T) : fallback;
}

// --- wire → model ------------------------------------------------------------

type WireBoard = Partial<ScheduleBoard> & { cycle: ScheduleCycle };

const toBoard = (w: WireBoard): ScheduleBoard => ({
  cycle: w.cycle,
  cycles: w.cycles ?? [],
  started: w.started === true,
  settings: w.settings ?? [],
  levels: w.levels ?? [],
  grades: w.grades ?? [],
  groups: w.groups ?? [],
  subjects: w.subjects ?? [],
  teachers: w.teachers ?? [],
  assignments: w.assignments ?? [],
  blocks: w.blocks ?? [],
});

type WireClass = Omit<ScheduleClass, "status" | "absence"> & { status: string; absence?: TeacherAbsence | null };
type WireDay = Omit<ScheduleDay, "classes" | "nonWorking"> & { nonWorking?: string | null; classes?: WireClass[] };

const toClass = (c: WireClass): ScheduleClass => ({ ...c, status: oneOf(CLASS_STATUSES, c.status, "regular"), absence: c.absence ?? null });

const toDay = (w: WireDay): ScheduleDay => ({
  date: w.date,
  weekday: w.weekday,
  nonWorking: w.nonWorking ?? null,
  classes: (w.classes ?? []).map(toClass),
});

// --- endpoints -----------------------------------------------------------------

/** The base timetable and its catalog; the ACTIVE cycle when none is named. */
export async function getBoard(baseURL: string, cycleId?: string): Promise<ScheduleBoard> {
  const qs = cycleId === undefined ? "" : `?cycleId=${checkedId(cycleId)}`;
  return toBoard(await read<WireBoard>(baseURL, `/api/v1/schedule${qs}`));
}

/** The calling teacher's own timetable (active cycle). */
export async function getOwnBoard(baseURL: string): Promise<ScheduleBoard> {
  return toBoard(await read<WireBoard>(baseURL, "/api/v1/teaching/schedule"));
}

/** One date as it really is: base classes plus that day's changes. */
export async function getDay(baseURL: string, cycleId: string, date: string): Promise<ScheduleDay> {
  return toDay(await read<WireDay>(baseURL, `/api/v1/schedule/day${query({ cycleId: checkedId(cycleId), date })}`));
}

/** The calling teacher's classes on one date, covers included. */
export async function getOwnDay(baseURL: string, date: string): Promise<ScheduleDay> {
  return toDay(await read<WireDay>(baseURL, `/api/v1/teaching/schedule/day${query({ date })}`));
}

export async function createBlock(baseURL: string, input: BlockInput): Promise<ScheduleBlock> {
  return (await send<{ block: ScheduleBlock }>(baseURL, "/api/v1/schedule/blocks", "POST", input)).block;
}

export async function updateBlock(baseURL: string, id: string, input: BlockInput): Promise<ScheduleBlock> {
  return (await send<{ block: ScheduleBlock }>(baseURL, `/api/v1/schedule/blocks/${checkedId(id)}`, "PUT", input)).block;
}

export async function deleteBlock(baseURL: string, id: string, reason?: string): Promise<void> {
  await remove(baseURL, `/api/v1/schedule/blocks/${checkedId(id)}${query({ reason })}`);
}

export async function createOverride(baseURL: string, input: OverrideInput): Promise<void> {
  await send<unknown>(baseURL, "/api/v1/schedule/overrides", "POST", input);
}

export async function deleteOverride(baseURL: string, id: string, reason?: string): Promise<void> {
  await remove(baseURL, `/api/v1/schedule/overrides/${checkedId(id)}${query({ reason })}`);
}

/** Teachers free in a slot, best fits first (teaches the subject, the grade, anybody). */
export async function listAvailableTeachers(baseURL: string, q: AvailableTeachersQuery): Promise<AvailableTeacher[]> {
  const qs = query({
    cycleId: q.cycleId, groupId: q.groupId, subjectId: q.subjectId, weekday: q.weekday, date: q.date,
    startsAt: q.startsAt, endsAt: q.endsAt, excludeBlockId: q.excludeBlockId,
  });
  const got = await read<{ teachers?: (Omit<AvailableTeacher, "match"> & { match: string })[] }>(baseURL, `/api/v1/schedule/available-teachers${qs}`);
  return (got.teachers ?? []).map((t) => ({ ...t, match: oneOf(MATCHES, t.match, "other") }));
}

export async function putSettings(baseURL: string, levelId: string, grid: { gridStart: string; gridEnd: string }): Promise<ScheduleSettings> {
  return (await send<{ settings: ScheduleSettings }>(baseURL, `/api/v1/schedule/settings/${checkedId(levelId)}`, "PUT", grid)).settings;
}

/** Who changed what, when and why; newest first. */
export async function listEvents(baseURL: string, cycleId: string): Promise<ScheduleEvent[]> {
  return (await read<{ events?: ScheduleEvent[] }>(baseURL, `/api/v1/schedule/events${query({ cycleId: checkedId(cycleId) })}`)).events ?? [];
}

// --- teacher absences (T19) ----------------------------------------------------

/** Register an absence; the answer says how many classes it leaves uncovered. */
export async function createAbsence(baseURL: string, input: AbsenceInput): Promise<AbsenceSaved> {
  return send<AbsenceSaved>(baseURL, "/api/v1/schedule/absences", "POST", input);
}

/** Change an active absence (never its teacher). */
export async function updateAbsence(baseURL: string, id: string, input: AbsenceInput): Promise<AbsenceSaved> {
  return send<AbsenceSaved>(baseURL, `/api/v1/schedule/absences/${checkedId(id)}`, "PUT", input);
}

/** Cancel an absence: it is never deleted, the record and its history stay. */
export async function cancelAbsence(baseURL: string, id: string): Promise<TeacherAbsence> {
  return (await send<{ absence: TeacherAbsence }>(baseURL, `/api/v1/schedule/absences/${checkedId(id)}/cancel`, "POST", {})).absence;
}

/** The absences touching from..to (active ones unless includeCancelled). */
export async function listAbsences(
  baseURL: string,
  q: { cycleId: string; from: string; to: string; teacherId?: string; includeCancelled?: boolean },
): Promise<TeacherAbsence[]> {
  const qs = query({ cycleId: checkedId(q.cycleId), from: q.from, to: q.to, teacherId: q.teacherId, includeCancelled: q.includeCancelled === true ? "true" : undefined });
  return (await read<{ absences?: TeacherAbsence[] }>(baseURL, `/api/v1/schedule/absences${qs}`)).absences ?? [];
}

/** "Clases por cubrir": the classes nobody gives, date by date (at most 31 days). */
export async function listUncovered(baseURL: string, cycleId: string, from: string, to: string): Promise<UncoveredClass[]> {
  const got = await read<{ classes?: { date: string; class: WireClass }[] }>(baseURL, `/api/v1/schedule/uncovered${query({ cycleId: checkedId(cycleId), from, to })}`);
  return (got.classes ?? []).map((u) => ({ date: u.date, class: toClass(u.class) }));
}
