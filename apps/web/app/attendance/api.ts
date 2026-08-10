/**
 * Typed fetch helpers for the gateway's attendance endpoints
 * (`/api/v1/attendance*`), gated behind the `module_attendance` flag + cookie
 * auth. The session lives ONLY in the HttpOnly cookie the gateway set, so every
 * call uses `credentials:"include"`; nothing secret ever travels in the body or
 * response.
 *
 * Reads require `attendance:read`, saving a day `attendance:write` (the owner
 * role holds both). These helpers mirror `app/grades/api.ts`: a `url()`
 * trailing-slash guard, throw on any non-2xx, and parse the gateway's
 * `{roster:[…]}`/`{records:[…]}` envelopes.
 *
 * The model is DEFAULT-PRESENT: saving a day sends ONLY the exceptions and the
 * server reconciles the whole group·day, so an omitted enrollment means
 * "present" and an empty `marks` array legitimately means "everybody present".
 * `Mark.status` is therefore typed as the exception union — a present student
 * cannot be expressed as a mark. Attendance carries no student PII: rows are
 * enrollment/student ids and names are resolved from the students list.
 */

// --- entity types (match the gateway JSON contract) -----------------------

import { authFetch } from "../auth/authFetch";

export type AttendanceStatus = "present" | "absent" | "late" | "excused";

/** The statuses that are worth storing; "present" is the absence of a mark. */
export type AttendanceException = Exclude<AttendanceStatus, "present">;

export interface RosterRow {
  enrollmentId: string;
  studentId: string;
  /** Reads back as "present" when the day has no stored mark for the row. */
  status: AttendanceStatus;
}

/** One non-present exception of a group·day. */
export interface Mark {
  enrollmentId: string;
  status: AttendanceException;
  /** Optional free-text reason; omitted from the body when empty. */
  note?: string;
}

export interface AttendanceRecord {
  id: string;
  enrollmentId: string;
  date: string;
  status: AttendanceStatus;
  note: string;
}

export interface DailyRosterQuery {
  groupId: string;
  /** `YYYY-MM-DD`. */
  date: string;
}

/** The COMPLETE set of exceptions of one group·day: the rest is reset to present. */
export interface SaveDailyInput {
  groupId: string;
  date: string;
  marks: readonly Mark[];
}

export interface SaveDailyResult {
  absentCount: number;
  markedCount: number;
}

export interface AttendanceQuery {
  enrollmentId: string;
  /** Inclusive `YYYY-MM-DD` bounds; empty values drop from the query string. */
  from?: string;
  to?: string;
}

// --- transport helpers ----------------------------------------------------

const BASE_PATH = "/api/v1";

function url(baseURL: string, path: string): string {
  return `${baseURL.replace(/\/+$/, "")}${path}`;
}

/** POST a JSON body and return the parsed result. Throws on any non-2xx. */
async function postJSON<T>(baseURL: string, path: string, body: unknown): Promise<T> {
  const res = await authFetch(url(baseURL, `${BASE_PATH}${path}`), {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw new Error(`attendance ${path}: HTTP ${res.status}`);
  }
  return (await res.json()) as T;
}

/** GET a list endpoint and unwrap the single-key envelope (`{ <key>: [...] }`). */
async function getList<T>(baseURL: string, path: string, key: string): Promise<T[]> {
  const res = await authFetch(url(baseURL, `${BASE_PATH}${path}`), { credentials: "include" });
  if (!res.ok) {
    throw new Error(`attendance ${path}: HTTP ${res.status}`);
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

/** Drops an empty note so the body carries only what the teacher actually typed. */
function toMarkBody(mark: Mark): Record<string, string> {
  const note = mark.note ?? "";
  const body: Record<string, string> = { enrollmentId: mark.enrollmentId, status: mark.status };
  if (note !== "") {
    body.note = note;
  }
  return body;
}

// --- attendance -----------------------------------------------------------

/** The day's pass list for a group: one row per active enrollment (no names). */
export function getDailyRoster(baseURL: string, q: DailyRosterQuery): Promise<RosterRow[]> {
  const qs = query({ groupId: q.groupId, date: q.date });
  return getList<RosterRow>(baseURL, `/attendance/roster${qs}`, "roster");
}

/**
 * Reconcile one group·day. `marks` carries ONLY the non-present exceptions —
 * every enrollment left out is stored as present — so the payload stays as small
 * as the teacher's corrections.
 */
export function saveDailyAttendance(baseURL: string, input: SaveDailyInput): Promise<SaveDailyResult> {
  const body = {
    groupId: input.groupId,
    date: input.date,
    marks: input.marks.map(toMarkBody),
  };
  return postJSON<SaveDailyResult>(baseURL, "/attendance", body);
}

/** One enrollment's stored exceptions over an inclusive date range. */
export function listAttendance(baseURL: string, q: AttendanceQuery): Promise<AttendanceRecord[]> {
  const qs = query({ enrollmentId: q.enrollmentId, from: q.from, to: q.to });
  return getList<AttendanceRecord>(baseURL, `/attendance${qs}`, "records");
}
