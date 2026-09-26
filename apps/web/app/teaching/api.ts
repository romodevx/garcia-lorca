/**
 * Typed fetch helpers for the gateway's TEACHER-scoped grade endpoints
 * (`/api/v1/teaching/*`), gated behind the `module_grades` flag + the caller's
 * `teaching:read`/`teaching:write` permissions. Mirrors `app/grades/api.ts`'s
 * style (a `url()` trailing-slash guard, single-key envelope unwrap, throw on any
 * non-2xx), but every endpoint is SELF-SCOPED to the signed-in teacher: it only
 * ever exposes that teacher's own assignments, their cycles' terms, and the
 * rosters of those assignments — never the school-wide `/grades/*` or `/config/*`
 * surfaces (which a teacher is not permitted to read).
 *
 * Because a teacher cannot fetch the students list, the roster row already
 * carries `studentName`; names are NOT resolved by a separate lookup. Auth rides
 * on the HttpOnly cookie (web) / Bearer (mobile) via `authFetch` + `credentials`.
 * This client is UX only; the gateway (403/404) is the security boundary.
 */

import { authFetch } from "../auth/authFetch";
// The self-scoped attendance surface shares the sheet vocabulary with the admin
// client. `attendance/sheet` is pure (types, body builder, error parsing), so
// reusing it creates no dependency on the admin `/attendance/*` client a teacher
// may not call.
import {
  attendanceFailure,
  sheetBody,
  sheetSearch,
  type AttendanceMode,
  type AttendanceSheet,
  type AttendanceSheetQuery,
  type SaveSheetInput,
} from "../attendance/sheet";

import { postCardScan, type CardScanResult, type ScanCardInput } from "../attendance/scan";
import { API_URL } from "../api-url";

// --- entity types (match the gateway JSON contract) -----------------------

/** One of the caller-teacher's own assignments, WITH readable group/subject names. */
export interface TeachingAssignment {
  id: string;
  cycleId: string;
  groupId: string;
  subjectId: string;
  teacherId: string;
  groupName: string;
  subjectName: string;
  /** The group's grade ("1° PRIMARIA"); groups are named by letter only. */
  gradeLevelName?: string;
}

/** A term of one of the caller's cycles. */
export interface TeachingTerm {
  id: string;
  name: string;
  cycleId: string;
}

/** One roster row for an assignment·term — name included (teacher can't list students). */
export interface TeachingRosterRow {
  enrollmentId: string;
  studentId: string;
  studentName: string;
  value: number;
  /** False when the student has no grade yet for this assignment·term. */
  hasValue: boolean;
}

/** The client-supplied edges of the capture knot. subject/teacher derived server-side. */
export interface CaptureGradeInput {
  assignmentId: string;
  enrollmentId: string;
  termId: string;
  /** Numeric grade in the range [0, 10]. */
  value: number;
}

export interface TeachingGradeEntry {
  id: string;
  enrollmentId: string;
  /** Server-derived from the assignment — never sent by the client. */
  subjectId: string;
  termId: string;
  /** Server-derived from the assignment — never sent by the client. */
  teacherId: string;
  value: number;
}

// --- transport helpers ----------------------------------------------------

const BASE_PATH = "/api/v1";

function url(path: string): string {
  // API_URL is already normalised (no trailing slash) by app/api-url.ts.
  return `${API_URL}${BASE_PATH}${path}`;
}

/** GET a list endpoint and unwrap the single-key envelope (`{ <key>: [...] }`). */
async function getList<T>(path: string, key: string): Promise<T[]> {
  const res = await authFetch(url(path), { credentials: "include" });
  if (!res.ok) {
    throw new Error(`teaching ${path}: HTTP ${res.status}`);
  }
  const data = (await res.json()) as Record<string, T[] | undefined>;
  return data[key] ?? [];
}

/** POST a JSON body and return the parsed entity. Throws on any non-2xx. */
async function postJSON<T>(path: string, body: unknown): Promise<T> {
  const res = await authFetch(url(path), {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw new Error(`teaching ${path}: HTTP ${res.status}`);
  }
  return (await res.json()) as T;
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

// --- teaching (self-scoped) -----------------------------------------------

/** The caller-teacher's own assignments (group·subject), names included. */
export function listAssignments(): Promise<TeachingAssignment[]> {
  return getList<TeachingAssignment>("/teaching/assignments", "assignments");
}

/** The terms of the caller's cycles. */
export function listTerms(): Promise<TeachingTerm[]> {
  return getList<TeachingTerm>("/teaching/terms", "terms");
}

/** The roster for one assignment·term — one row per enrolled student, name included. */
export function getRoster(assignmentId: string, termId: string): Promise<TeachingRosterRow[]> {
  const qs = query({ assignmentId, termId });
  return getList<TeachingRosterRow>(`/teaching/roster${qs}`, "roster");
}

/**
 * Capture (or edit) a student's numeric grade. The body carries only the
 * client-supplied edges — subjectId/teacherId are intentionally ABSENT so the
 * server derives them from the assignment and they can never drift.
 */
export function captureGrade(input: CaptureGradeInput): Promise<TeachingGradeEntry> {
  return postJSON<TeachingGradeEntry>("/teaching/grades", {
    assignmentId: input.assignmentId,
    enrollmentId: input.enrollmentId,
    termId: input.termId,
    value: input.value,
  });
}

// --- teaching attendance (self-scoped) ------------------------------------

/**
 * Something the teacher can take attendance for: a group (daily levels) or one
 * of their group·subject classes (per-class levels).
 */
export interface AttendanceClass {
  groupId: string;
  /** Ready to show: "1° SECUNDARIA · A". */
  groupLabel: string;
  /** groupLabel's halves — the group's letter and its grade — when the gateway sends them. */
  groupName?: string;
  gradeLevelName?: string;
  levelName: string;
  mode: AttendanceMode;
  subjectId?: string;
  subjectName?: string;
  assignmentId?: string;
}

/** The caller's attendance classes: one per daily group, one per per-class assignment. */
export async function listAttendanceClasses(): Promise<AttendanceClass[]> {
  const res = await authFetch(url("/teaching/attendance/classes"), { credentials: "include" });
  if (!res.ok) {
    throw await attendanceFailure(res);
  }
  const data = (await res.json()) as AttendanceClass[] | { classes?: AttendanceClass[] };
  return Array.isArray(data) ? data : (data.classes ?? []);
}

/** The list of one of the caller's groups (or classes) for a day. */
export async function getTeachingAttendanceSheet(q: AttendanceSheetQuery): Promise<AttendanceSheet> {
  const res = await authFetch(url(`/teaching/attendance/roster${sheetSearch(q)}`), { credentials: "include" });
  if (!res.ok) {
    throw await attendanceFailure(res);
  }
  return (await res.json()) as AttendanceSheet;
}

/**
 * Saves one of the caller's lists. Only the exceptions travel — every
 * enrollment left out is stored as present — and a rejection carries the
 * server's own reason (the correction window, the list's mode).
 */
export async function saveTeachingAttendance(input: SaveSheetInput): Promise<void> {
  const res = await authFetch(url("/teaching/attendance"), {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(sheetBody(input)),
  });
  if (!res.ok) {
    throw await attendanceFailure(res);
  }
}

/** Resolves one scanned student card against one of the caller's lists. */
export function scanTeachingCard(input: ScanCardInput): Promise<CardScanResult> {
  return postCardScan(url("/teaching/attendance/scan"), input);
}
