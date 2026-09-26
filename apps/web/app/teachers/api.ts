/**
 * The staff roster's client.
 *
 * These four calls used to live in the catalogue's client, under
 * /api/v1/config/…, because Docentes was a panel inside Configuración. It is a
 * module of its own now, so it answers on its own path and behind its own
 * permission (teachers:read / teachers:write) — which is the point of the move: a
 * school can hand somebody the staff roster without handing them the cycles,
 * levels, grades and groups as well.
 *
 * The same config service still answers. What moved is the door, not the room.
 */
import { authFetch } from "../auth/authFetch";
import { ConfigError } from "../config/api";

const BASE_PATH = "/api/v1";

export interface Teacher {
  id: string;
  firstName: string;
  lastName: string;
  email: string;
  /** Empty until the teacher is linked to a login. */
  userId: string;
}

export interface CreateTeacherInput {
  firstName: string;
  lastName: string;
  email: string;
  /** Reserved for the IAM link; omitted by the UI today. */
  userId?: string;
}

export interface TeachingAssignment {
  id: string;
  /** Server-derived from the group's cycle — never sent by the client. */
  cycleId: string;
  groupId: string;
  subjectId: string;
  teacherId: string;
}

/** The client-supplied edges of the knot. cycleId is derived server-side. */
export interface CreateTeachingAssignmentInput {
  groupId: string;
  subjectId: string;
  teacherId: string;
}

/** Optional list filters; empty values are dropped from the query string. */
export interface TeachingAssignmentFilters {
  cycleId?: string;
  groupId?: string;
  teacherId?: string;
}

/** One (group, subject) slot of a teaching load. */
export interface TeachingLoadItem {
  groupId: string;
  subjectId: string;
}

export interface AssignTeachingLoadInput {
  teacherId: string;
  /** 1..500 slots — a bound on one transaction, not on a teacher's load. */
  items: readonly TeachingLoadItem[];
  /** Hand slots another teacher holds to this one. Explicit, never implied. */
  replaceConflicts: boolean;
}

/** A requested slot another teacher already holds, left untouched. */
export interface TeachingLoadConflict extends TeachingLoadItem {
  assignmentId: string;
  teacherId: string;
  /** "first last" of the teacher holding it. */
  teacherName: string;
}

/** What happened to each requested slot. Every list is present. */
export interface TeachingLoadResult {
  created: TeachingAssignment[];
  reassigned: TeachingAssignment[];
  /** Slots that were already this teacher's. */
  skipped: TeachingLoadItem[];
  conflicts: TeachingLoadConflict[];
}

function url(baseURL: string, path: string): string {
  return `${baseURL.replace(/\/+$/, "")}${BASE_PATH}${path}`;
}

async function failure(res: Response): Promise<ConfigError> {
  const body = await res.text().catch(() => "");
  return new ConfigError(res.status, body.trim());
}

async function sendJSON<T>(method: "POST" | "PATCH", baseURL: string, path: string, body: unknown): Promise<T> {
  const res = await authFetch(url(baseURL, path), {
    method,
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw await failure(res);
  }
  return (await res.json()) as T;
}

function postJSON<T>(baseURL: string, path: string, body: unknown): Promise<T> {
  return sendJSON<T>("POST", baseURL, path, body);
}

/** GET a list endpoint and unwrap the single-key envelope (`{ <key>: [...] }`). */
async function getList<T>(baseURL: string, path: string, key: string): Promise<T[]> {
  const res = await authFetch(url(baseURL, path), { credentials: "include" });
  if (!res.ok) {
    throw await failure(res);
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

// --- teachers -------------------------------------------------------------

export function createTeacher(baseURL: string, input: CreateTeacherInput): Promise<Teacher> {
  return postJSON<Teacher>(baseURL, "/teachers", input);
}

export function listTeachers(baseURL: string): Promise<Teacher[]> {
  return getList<Teacher>(baseURL, "/teachers", "teachers");
}

// --- teaching assignments (the knot) --------------------------------------

export function createTeachingAssignment(
  baseURL: string,
  input: CreateTeachingAssignmentInput,
): Promise<TeachingAssignment> {
  return postJSON<TeachingAssignment>(baseURL, "/teaching-assignments", input);
}

export function listTeachingAssignments(
  baseURL: string,
  filters: TeachingAssignmentFilters = {},
): Promise<TeachingAssignment[]> {
  const qs = query({ cycleId: filters.cycleId, groupId: filters.groupId, teacherId: filters.teacherId });
  return getList<TeachingAssignment>(baseURL, `/teaching-assignments${qs}`, "teachingAssignments");
}

// --- carga académica (a teacher's whole load) ------------------------------

/**
 * Gives one teacher many slots in one transaction. The answer is always 200 and
 * says per slot what happened; a slot another teacher holds comes back as a
 * conflict unless `replaceConflicts` was asked for.
 */
export async function assignTeachingLoad(
  baseURL: string,
  input: AssignTeachingLoadInput,
): Promise<TeachingLoadResult> {
  const res = await postJSON<Partial<TeachingLoadResult>>(baseURL, "/teaching-assignments/bulk", input);
  return {
    created: res.created ?? [],
    reassigned: res.reassigned ?? [],
    skipped: res.skipped ?? [],
    conflicts: res.conflicts ?? [],
  };
}

/** Frees a slot; anyone can be given it afterwards. */
export async function deleteTeachingAssignment(baseURL: string, id: string): Promise<void> {
  const res = await authFetch(url(baseURL, `/teaching-assignments/${encodeURIComponent(id)}`), {
    method: "DELETE",
    credentials: "include",
  });
  if (!res.ok) {
    throw await failure(res);
  }
}

/** Hands a slot to another teacher, keeping its id. */
export function reassignTeachingAssignment(
  baseURL: string,
  id: string,
  teacherId: string,
): Promise<TeachingAssignment> {
  return sendJSON<TeachingAssignment>("PATCH", baseURL, `/teaching-assignments/${encodeURIComponent(id)}`, { teacherId });
}
