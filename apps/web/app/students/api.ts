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
  /** The level's name as the school wrote it ("Preescolar"). */
  level: string;
  /** The config level id ("" only for a legacy pupil no level matched). */
  levelId: string;
  gradeGroup: string;
  guardianName: string;
  admissionStatus: AdmissionStatus;
  /** The editable half of the file. Absent from gateways older than T5. */
  profile?: StudentProfile;
}

/** "" means not captured. */
export type Sex = "" | "F" | "M" | "X";

export type PhoneKind = "celular" | "casa" | "trabajo" | "otro";

export interface StudentPhone {
  kind: PhoneKind;
  /** Ten digits. */
  number: string;
}

export interface StudentAddress {
  street: string;
  extNumber: string;
  intNumber: string;
  neighborhood: string;
  postalCode: string;
  municipality: string;
  state: string;
}

/** What the office keeps up to date by hand. Every field is optional. */
export interface StudentProfile {
  /** ISO date ("2019-03-12") or "". */
  birthDate: string;
  sex: Sex;
  email: string;
  phones: StudentPhone[];
  address: StudentAddress;
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
  /** A config level id: the school's own levels, not a fixed SEP list. */
  levelId: string;
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

/** Where a pupil landed: the enrolment, plus the group capacity chose, by name. */
export interface Placement {
  enrollment: Enrollment;
  groupName: string;
}

/**
 * Places a pupil in a GRADE and lets the school's capacity choose the group.
 *
 * What a school knows when a child arrives is the level and the grade; which
 * group they land in is a consequence of how full each one is. The form used to
 * ask for "grado y grupo" as free text, so somebody typed "1° A" and nothing
 * checked whether 1°A had a seat left.
 */
export function enrollIntoGrade(baseURL: string, studentId: string, gradeLevelId: string): Promise<Placement> {
  return postJSON<Placement>(baseURL, "/enrollments/by-grade", { studentId, gradeLevelId });
}

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

// --- contacts (M16) ---------------------------------------------------------

/** How a contact relates to the pupil. */
export type Relationship = "mother" | "father" | "guardian" | "other";

/**
 * Somebody the school can reach about a pupil.
 *
 * A contact WITH an account is a family that can open the app; one WITHOUT is a
 * phone number the office calls. Most of a pupil's contacts are the second kind,
 * which is why recording one no longer requires creating a login.
 */
export interface Contact {
  id: string;
  studentId: string;
  guardianUserId: string;
  relationship: Relationship;
  fullName: string;
  phone: string;
  email: string;
  /** Whether this contact can open the family app. */
  hasAccount: boolean;
  /** Whom the school calls first when something happens. */
  isEmergency?: boolean;
}

export interface AddContactInput {
  studentId: string;
  relationship: Relationship;
  fullName: string;
  phone?: string;
  email?: string;
  isEmergency?: boolean;
}

/** The people the school can call about one pupil. */
export async function listContacts(baseURL: string, studentId: string): Promise<Contact[]> {
  const res = await authFetch(`${baseURL.replace(/\/+$/, "")}/api/v1/students/${studentId}/guardians`, {
    credentials: "include",
  });
  if (!res.ok) {
    throw await studentsFailure(res);
  }
  const data = (await res.json()) as { guardians?: Contact[]; links?: Contact[] };
  return data.guardians ?? data.links ?? [];
}

/**
 * What came of asking for an invitation.
 *
 * `sent` is the only honest thing to read before telling anybody the mail went
 * out, and even then it means "accepted by the channel", not "in their inbox" —
 * see `via`. `url` is the fallback, returned ONLY when nothing was delivered.
 */
export interface Invitation {
  url: string;
  expiresAt: string;
  sent: boolean;
  sendError?: string;
  /** "smtp" (the school's own mail) or "provider" (Google sends its own link). */
  via?: string;
}

export interface ContactAccessResult {
  contact: Contact;
  /** Null when the address already had an account: nothing was sent. */
  invitation: Invitation | null;
}

/**
 * Gives a contact the school already recorded access to the family app.
 *
 * The account lands on THIS contact, so the pupil keeps one list. An address
 * that already has an account is reused — a parent with two children signs in
 * once — and then `invitation` comes back null because there was nothing to send.
 */
export async function inviteContactToApp(
  baseURL: string,
  studentId: string,
  contactId: string,
): Promise<ContactAccessResult> {
  const root = baseURL.replace(/\/+$/, "");
  const res = await authFetch(`${root}/api/v1/students/${studentId}/guardians/${contactId}/invite`, {
    method: "POST",
    credentials: "include",
  });
  if (!res.ok) {
    throw await studentsFailure(res);
  }
  return (await res.json()) as ContactAccessResult;
}

/** Records somebody the school can call, with no account and no login. */
export async function addContact(baseURL: string, input: AddContactInput): Promise<Contact> {
  const res = await authFetch(`${baseURL.replace(/\/+$/, "")}/api/v1/contacts`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      studentId: input.studentId,
      relationship: input.relationship,
      fullName: input.fullName,
      ...(input.phone !== undefined && input.phone !== "" ? { phone: input.phone } : {}),
      ...(input.email !== undefined && input.email !== "" ? { email: input.email } : {}),
      ...(input.isEmergency === true ? { isEmergency: true } : {}),
    }),
  });
  if (!res.ok) {
    throw await studentsFailure(res);
  }
  return (await res.json()) as Contact;
}

// --- the pupil's file (T5) --------------------------------------------------

export interface UpdateStudentInput {
  firstName: string;
  lastName: string;
  curp: string;
  profile: StudentProfile;
}

function studentPath(baseURL: string, studentId: string, rest = ""): string {
  return url(baseURL, `${BASE_PATH}/students/${encodeURIComponent(studentId)}${rest}`);
}

async function sendJSON<T>(target: string, method: "PUT", body: unknown): Promise<T> {
  const res = await authFetch(target, {
    method,
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw await studentsFailure(res);
  }
  return (await res.json()) as T;
}

async function sendEmpty(target: string, method: "DELETE"): Promise<void> {
  const res = await authFetch(target, { method, credentials: "include" });
  if (!res.ok) {
    throw await studentsFailure(res);
  }
}

/** Corrects names, CURP and profile. Level and group change by transfer, never here. */
export function updateStudent(baseURL: string, studentId: string, input: UpdateStudentInput): Promise<Student> {
  return sendJSON<Student>(studentPath(baseURL, studentId), "PUT", input);
}

/** A pupil's medical record: readable only with medical:read. */
export interface MedicalRecord {
  /** "" or one of A+ A- B+ B- AB+ AB- O+ O-. */
  bloodType: string;
  allergies: string;
  conditions: string;
}

export async function getMedical(baseURL: string, studentId: string): Promise<MedicalRecord> {
  const res = await authFetch(studentPath(baseURL, studentId, "/medical"), { credentials: "include" });
  if (!res.ok) {
    throw await studentsFailure(res);
  }
  return (await res.json()) as MedicalRecord;
}

export function updateMedical(baseURL: string, studentId: string, record: MedicalRecord): Promise<MedicalRecord> {
  return sendJSON<MedicalRecord>(studentPath(baseURL, studentId, "/medical"), "PUT", record);
}

/** The pupil's photo, or null when there is none. Fetched with the session, never from a public URL. */
export async function getPhotoBlob(baseURL: string, studentId: string): Promise<Blob | null> {
  const res = await authFetch(studentPath(baseURL, studentId, "/photo"), { credentials: "include" });
  if (res.status === 404) {
    return null;
  }
  if (!res.ok) {
    throw await studentsFailure(res);
  }
  return res.blob();
}

/** Uploads a JPEG or PNG (at most 2 MB); the server decides the type from the bytes. */
export async function uploadPhoto(baseURL: string, studentId: string, image: Blob): Promise<void> {
  const res = await authFetch(studentPath(baseURL, studentId, "/photo"), {
    method: "PUT",
    credentials: "include",
    headers: { "Content-Type": image.type === "" ? "application/octet-stream" : image.type },
    body: image,
  });
  if (!res.ok) {
    throw await studentsFailure(res);
  }
}

export function deletePhoto(baseURL: string, studentId: string): Promise<void> {
  return sendEmpty(studentPath(baseURL, studentId, "/photo"), "DELETE");
}

export interface ContactEdit {
  relationship: Relationship;
  fullName: string;
  phone: string;
  email: string;
  isEmergency: boolean;
}

export function updateContact(
  baseURL: string,
  studentId: string,
  contactId: string,
  edit: ContactEdit,
): Promise<Contact> {
  return sendJSON<Contact>(studentPath(baseURL, studentId, `/contacts/${encodeURIComponent(contactId)}`), "PUT", edit);
}

/** Refused with 409 while the contact can open the family app. */
export function removeContact(baseURL: string, studentId: string, contactId: string): Promise<void> {
  return sendEmpty(studentPath(baseURL, studentId, `/contacts/${encodeURIComponent(contactId)}`), "DELETE");
}
