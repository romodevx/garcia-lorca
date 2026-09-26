/**
 * Typed fetch helpers for the gateway's attendance endpoints
 * (`/api/v1/attendance*`), gated behind the `module_attendance` flag + cookie
 * auth. The session lives ONLY in the HttpOnly cookie the gateway set, so every
 * call uses `credentials:"include"`; nothing secret ever travels in the body or
 * response.
 *
 * Reads require `attendance:read`, saving a day `attendance:write` (the owner
 * role holds both). These helpers mirror `app/grades/api.ts`: a `url()`
 * trailing-slash guard and throw on any non-2xx.
 *
 * The model is DEFAULT-PRESENT: saving a list sends ONLY the exceptions and the
 * server reconciles the whole list, so an omitted enrollment means "present"
 * and an empty `marks` array legitimately means "everybody present". A mark's
 * status is therefore the exception union — a present student cannot be
 * expressed as a mark. The sheet vocabulary lives in `./sheet`.
 */

// --- entity types (match the gateway JSON contract) -----------------------

import { authFetch } from "../auth/authFetch";
import {
  attendanceFailure,
  sheetBody,
  sheetSearch,
  type AttendanceSheet,
  type AttendanceSheetQuery,
  type SaveSheetInput,
} from "./sheet";

export type AttendanceStatus = "present" | "absent" | "late" | "excused";

/** The statuses that are worth storing; "present" is the absence of a mark. */
export type AttendanceException = Exclude<AttendanceStatus, "present">;

export interface AttendanceRecord {
  id: string;
  enrollmentId: string;
  date: string;
  status: AttendanceStatus;
  note: string;
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

// --- attendance -----------------------------------------------------------

/** One enrollment's stored exceptions over an inclusive date range. */
export function listAttendance(baseURL: string, q: AttendanceQuery): Promise<AttendanceRecord[]> {
  const qs = query({ enrollmentId: q.enrollmentId, from: q.from, to: q.to });
  return getList<AttendanceRecord>(baseURL, `/attendance${qs}`, "records");
}

// --- the attendance sheet (per-level mode) --------------------------------

/** The list of a group·day (or group·subject·day), names and session included. */
export async function getAttendanceSheet(baseURL: string, q: AttendanceSheetQuery): Promise<AttendanceSheet> {
  const res = await authFetch(url(baseURL, `${BASE_PATH}/attendance/roster${sheetSearch(q)}`), { credentials: "include" });
  if (!res.ok) {
    throw await attendanceFailure(res);
  }
  return (await res.json()) as AttendanceSheet;
}

/** Saves the list: only the exceptions travel; the server reconciles the rest to present. */
export async function saveAttendanceSheet(baseURL: string, input: SaveSheetInput): Promise<void> {
  const res = await authFetch(url(baseURL, `${BASE_PATH}/attendance`), {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(sheetBody(input)),
  });
  if (!res.ok) {
    throw await attendanceFailure(res);
  }
}
