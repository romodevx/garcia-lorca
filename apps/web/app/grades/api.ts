/**
 * Typed fetch helpers for the gateway's grade-capture endpoints
 * (`/api/v1/grades*`), gated behind the `module_grades` flag + cookie auth. The
 * session lives ONLY in the HttpOnly cookie the gateway set, so every call uses
 * `credentials:"include"`; nothing secret ever travels in the body or response.
 *
 * Reads require `grades:read`, writes `grades:write` (the owner role holds both).
 * These helpers mirror `app/config/api.ts` + `app/students/api.ts`: a `url()`
 * trailing-slash guard, throw on any non-2xx, and parse the gateway's
 * `{roster:[…]}`/`{gradeEntries:[…]}` envelopes. Shapes match the gateway REST
 * contract exactly.
 *
 * The capture payload carries ONLY the client-supplied edges of the knot
 * ({assignmentId, enrollmentId, termId, value}) — `subjectId`/`teacherId` are
 * DERIVED server-side from the assignment so they can never drift. `value` is a
 * plain number (0–10). This client is UX only; the gateway is the security
 * boundary.
 */

// --- entity types (match the gateway JSON contract) -----------------------

export interface GradeEntry {
  id: string;
  enrollmentId: string;
  /** Server-derived from the assignment — never sent by the client. */
  subjectId: string;
  termId: string;
  /** Server-derived from the assignment — never sent by the client. */
  teacherId: string;
  value: number;
}

export interface RosterRow {
  enrollmentId: string;
  studentId: string;
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

export interface RosterQuery {
  assignmentId: string;
  termId: string;
}

export interface GradeEntriesQuery {
  enrollmentId: string;
  termId: string;
}

// --- transport helpers ----------------------------------------------------

const BASE_PATH = "/api/v1";

function url(baseURL: string, path: string): string {
  return `${baseURL.replace(/\/+$/, "")}${path}`;
}

/** POST a JSON body and return the parsed entity. Throws on any non-2xx. */
async function postJSON<T>(baseURL: string, path: string, body: unknown): Promise<T> {
  const res = await fetch(url(baseURL, `${BASE_PATH}${path}`), {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw new Error(`grades ${path}: HTTP ${res.status}`);
  }
  return (await res.json()) as T;
}

/** GET a list endpoint and unwrap the single-key envelope (`{ <key>: [...] }`). */
async function getList<T>(baseURL: string, path: string, key: string): Promise<T[]> {
  const res = await fetch(url(baseURL, `${BASE_PATH}${path}`), { credentials: "include" });
  if (!res.ok) {
    throw new Error(`grades ${path}: HTTP ${res.status}`);
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

// --- grades ---------------------------------------------------------------

/**
 * Capture (or edit) a student's numeric grade. The body carries only the
 * client-supplied edges — subjectId/teacherId are intentionally ABSENT so the
 * server derives them from the assignment and they can never drift. `value` is
 * sent as a number.
 */
export function captureGrade(baseURL: string, input: CaptureGradeInput): Promise<GradeEntry> {
  const body = {
    assignmentId: input.assignmentId,
    enrollmentId: input.enrollmentId,
    termId: input.termId,
    value: input.value,
  };
  return postJSON<GradeEntry>(baseURL, "/grades", body);
}

/** The roster for an assignment·term: one row per enrolled student (value optional). */
export function getRoster(baseURL: string, q: RosterQuery): Promise<RosterRow[]> {
  const qs = query({ assignmentId: q.assignmentId, termId: q.termId });
  return getList<RosterRow>(baseURL, `/grades/roster${qs}`, "roster");
}

/** The grade entries for one enrollment·term (optional filters). */
export function listGradeEntries(baseURL: string, q: GradeEntriesQuery): Promise<GradeEntry[]> {
  const qs = query({ enrollmentId: q.enrollmentId, termId: q.termId });
  return getList<GradeEntry>(baseURL, `/grades${qs}`, "gradeEntries");
}
