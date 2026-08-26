/**
 * Typed fetch helpers for the gateway's students + enrollment endpoints
 * (`/api/v1/students/*`, `/api/v1/enrollments/*`), gated behind the
 * `module_students` flag + cookie auth. The session lives ONLY in the HttpOnly
 * cookie the gateway set, so every call uses `credentials:"include"`; nothing
 * secret ever travels in the body or response.
 *
 * Reads require `students:read`, writes (enroll/transfer/withdraw)
 * `students:enroll` (the owner role holds both). These helpers mirror
 * `app/config/api.ts`: a `url()` trailing-slash guard, throw on any non-2xx, and
 * parse the gateway's `{students:[…]}`/`{enrollments:[…]}` envelopes. Shapes match
 * the gateway REST contract exactly.
 *
 * The enroll payload NEVER carries `cycleId`: the students service derives the
 * cycle from the group so it can never drift — the same invariant the gateway and
 * usecase enforce. This client is UX only; the gateway is the security boundary.
 */

// --- entity types (match the gateway JSON contract) -----------------------

import { authFetch } from "../auth/authFetch";

export type AdmissionStatus = "pending" | "accepted" | "enrolled" | "rejected" | "withdrawn";

export interface Student {
  id: string;
  curp: string;
  firstName: string;
  lastName: string;
  level: string;
  gradeGroup: string;
  guardianName: string;
  admissionStatus: AdmissionStatus;
}

export type EnrollmentStatus = "active" | "transferred" | "withdrawn";

export interface Enrollment {
  id: string;
  studentId: string;
  /** Server-derived from the group's cycle — never sent by the client. */
  cycleId: string;
  groupId: string;
  /** Empty when the enrollment is not scoped to a term. */
  termId: string;
  status: EnrollmentStatus;
  enrolledAt: string;
}

/** The client-supplied edges of the enroll knot. cycleId is derived server-side. */
export interface EnrollInput {
  studentId: string;
  groupId: string;
  /** Optional; omitted from the body when absent. */
  termId?: string;
}

export interface TransferInput {
  enrollmentId: string;
  toGroupId: string;
  reason: string;
}

/** Optional list filters; empty values are dropped from the query string. */
export interface EnrollmentFilters {
  studentId?: string;
  cycleId?: string;
  groupId?: string;
}

// --- transport helpers ----------------------------------------------------

const BASE_PATH = "/api/v1";

function url(baseURL: string, path: string): string {
  return `${baseURL.replace(/\/+$/, "")}${path}`;
}

/** POST a JSON body and return the parsed entity. Throws on any non-2xx. */
/**
 * A students failure the UI can act on.
 *
 * The status is the point. Creating a pupil fails in exactly two ways a person
 * causes — a CURP the school already has, and one that is not a CURP — and they
 * need different advice. Throwing a bare Error left the drawer with one sentence
 * for both ("verifica el CURP"), which it also showed when the server fell over,
 * so a person could re-type a perfectly good CURP forever.
 */
export class StudentsError extends Error {
  readonly status: number;
  /** The server's own words, when it sent any. */
  readonly detail: string;

  constructor(status: number, detail: string) {
    super(detail !== "" ? detail : `HTTP ${status}`);
    this.name = "StudentsError";
    this.status = status;
    this.detail = detail;
  }

  /** The school already has this pupil. Re-typing the same CURP never helps. */
  get isConflict(): boolean {
    return this.status === 409;
  }

  /** The input itself is wrong; changing it will help. */
  get isInvalid(): boolean {
    return this.status === 400;
  }
}

async function studentsFailure(res: Response): Promise<StudentsError> {
  const body = await res.text().catch(() => "");
  return new StudentsError(res.status, body.trim());
}

async function postJSON<T>(baseURL: string, path: string, body: unknown): Promise<T> {
  const res = await authFetch(url(baseURL, `${BASE_PATH}${path}`), {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw await studentsFailure(res);
  }
  return (await res.json()) as T;
}

/** GET a single resource and return it. Throws on any non-2xx. */
async function getJSON<T>(baseURL: string, path: string): Promise<T> {
  const res = await authFetch(url(baseURL, `${BASE_PATH}${path}`), { credentials: "include" });
  if (!res.ok) {
    throw new Error(`students ${path}: HTTP ${res.status}`);
  }
  return (await res.json()) as T;
}

/** GET a list endpoint and unwrap the single-key envelope (`{ <key>: [...] }`). */
async function getList<T>(baseURL: string, path: string, key: string): Promise<T[]> {
  const res = await authFetch(url(baseURL, `${BASE_PATH}${path}`), { credentials: "include" });
  if (!res.ok) {
    throw new Error(`students ${path}: HTTP ${res.status}`);
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

// --- students -------------------------------------------------------------

export function listStudents(baseURL: string, status?: string): Promise<Student[]> {
  return getList<Student>(baseURL, `/students${query({ status })}`, "students");
}

export function getStudent(baseURL: string, id: string): Promise<Student> {
  return getJSON<Student>(baseURL, `/students/${id}`);
}

/** The applicant's identity + intended placement, as the gateway create contract expects. */
export interface CreateStudentInput {
  curp: string;
  firstName: string;
  lastName: string;
  level: string;
  gradeGroup: string;
  guardianName: string;
}

/**
 * Registers a new applicant. Admission starts at "pending" (the server owns the
 * state machine); the returned Student then becomes enrollable. Needs
 * students:create — the gateway is the security boundary, this is UX only.
 */
export function createStudent(baseURL: string, input: CreateStudentInput): Promise<Student> {
  return postJSON<Student>(baseURL, "/students", input);
}

// --- enrollments ----------------------------------------------------------

/**
 * Places a student into a group. The body carries only the client-supplied edges
 * ({studentId, groupId}, plus termId when given) — cycleId is intentionally
 * absent so the server derives it from the group and it can never drift.
 */
export function enroll(baseURL: string, input: EnrollInput): Promise<Enrollment> {
  const body: Record<string, string> = { studentId: input.studentId, groupId: input.groupId };
  if (input.termId !== undefined && input.termId !== "") {
    body.termId = input.termId;
  }
  return postJSON<Enrollment>(baseURL, "/enrollments", body);
}

export function listEnrollments(baseURL: string, filters: EnrollmentFilters = {}): Promise<Enrollment[]> {
  const qs = query({ studentId: filters.studentId, cycleId: filters.cycleId, groupId: filters.groupId });
  return getList<Enrollment>(baseURL, `/enrollments${qs}`, "enrollments");
}

export function transfer(baseURL: string, input: TransferInput): Promise<Enrollment> {
  return postJSON<Enrollment>(baseURL, "/enrollments/transfer", input);
}

export function withdraw(baseURL: string, enrollmentId: string): Promise<Enrollment> {
  return postJSON<Enrollment>(baseURL, "/enrollments/withdraw", { enrollmentId });
}
