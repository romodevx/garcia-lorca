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

function url(baseURL: string, path: string): string {
  return `${baseURL.replace(/\/+$/, "")}${BASE_PATH}${path}`;
}

async function failure(res: Response): Promise<ConfigError> {
  const body = await res.text().catch(() => "");
  return new ConfigError(res.status, body.trim());
}

async function postJSON<T>(baseURL: string, path: string, body: unknown): Promise<T> {
  const res = await authFetch(url(baseURL, path), {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw await failure(res);
  }
  return (await res.json()) as T;
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
