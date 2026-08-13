import { apiFetch } from "./client";
import { ApiError } from "./errors";

/**
 * Typed helpers over the gateway's guardian self-scoped family surface
 * (`/api/v1/family/*`, Bearer). Every route is scoped to the authenticated
 * guardian's own children by construction — no argument here can widen it; the
 * studentId a caller passes is verified against the caller's links server-side,
 * and an id outside that set answers 404. See services/gateway family.go.
 *
 * All helpers throw an {@link ApiError} on any non-2xx, mirroring
 * ./notifications.ts.
 */

/** One of the guardian's linked children (the entry point for every by-child read). */
export interface FamilyStudent {
  id: string;
  firstName: string;
  lastName: string;
  /** Canonical level name: "kinder" | "primaria" | "secundaria" | "preparatoria" | "". */
  level: string;
  gradeGroup: string;
}

/** A child's placement in a cycle. Grades and attendance are keyed by enrollment. */
export interface FamilyEnrollment {
  id: string;
  studentId: string;
  cycleId: string;
  groupId: string;
  termId: string;
  status: string;
  enrolledAt: string;
}

/** A child's core record plus their current active enrollment, if any. */
export interface FamilyProfile extends FamilyStudent {
  currentEnrollment: FamilyEnrollment | null;
}

/** One finance charge on a child's account. Amounts are integer cents. */
export interface FamilyCharge {
  id: string;
  description: string;
  amountCents: number;
  discountCents: number;
  paidCents: number;
  remainingCents: number;
  dueDate: string;
  status: string;
}

/** One payment applied to a child's charge. Amounts are integer cents. */
export interface FamilyPayment {
  id: string;
  chargeId: string;
  amountCents: number;
  method: string;
  reference: string;
  paidAt: string;
}

/** A child's full finance statement. */
export interface FamilyAccount {
  charges: FamilyCharge[];
  payments: FamilyPayment[];
  totalBalanceCents: number;
}

/** One numeric grade entry, enriched with its subject and term display names. */
export interface FamilyGradeEntry {
  id: string;
  subjectId: string;
  termId: string;
  teacherId: string;
  value: number;
  subjectName: string;
  termName: string;
}

/** The attendance statuses the backend stores as exceptions. */
export type FamilyAttendanceStatus = "present" | "absent" | "late" | "excused";

/** One stored attendance record for a child on a given day (ISO date). */
export interface FamilyAttendanceRecord {
  id: string;
  date: string;
  status: FamilyAttendanceStatus;
  note: string;
}

interface StudentsEnvelope {
  students: FamilyStudent[] | null;
}
interface GradesEnvelope {
  gradeEntries: FamilyGradeEntry[] | null;
}
interface AttendanceEnvelope {
  records: FamilyAttendanceRecord[] | null;
}

/** Reads one non-2xx into a typed error so callers branch on `.status`. */
async function unwrap<T>(res: Response, what: string): Promise<T> {
  if (!res.ok) {
    throw new ApiError(res.status, `failed to load ${what}`);
  }
  return (await res.json()) as T;
}

/**
 * Lists the guardian's linked children — the entry point the app fetches first to
 * learn the studentId(s) it feeds to every other family read.
 */
export async function listChildren(): Promise<FamilyStudent[]> {
  const data = await unwrap<StudentsEnvelope>(await apiFetch("/api/v1/family/students"), "children");
  return data.students ?? [];
}

/** Reads one child's core record and current enrollment. */
export async function getProfile(studentId: string): Promise<FamilyProfile> {
  return unwrap<FamilyProfile>(await apiFetch(`/api/v1/family/students/${studentId}/profile`), "profile");
}

/** Reads one child's finance statement (charges, payments, outstanding balance). */
export async function getAccount(studentId: string): Promise<FamilyAccount> {
  const data = await unwrap<Partial<FamilyAccount>>(
    await apiFetch(`/api/v1/family/students/${studentId}/account`),
    "account",
  );
  return {
    charges: data.charges ?? [],
    payments: data.payments ?? [],
    totalBalanceCents: data.totalBalanceCents ?? 0,
  };
}

/** Reads one child's grade entries, optionally narrowed to a single term. */
export async function getGrades(studentId: string, termId?: string): Promise<FamilyGradeEntry[]> {
  const query = termId ? `?termId=${encodeURIComponent(termId)}` : "";
  const data = await unwrap<GradesEnvelope>(
    await apiFetch(`/api/v1/family/students/${studentId}/grades${query}`),
    "grades",
  );
  return data.gradeEntries ?? [];
}

/** Reads one child's stored attendance records over an optional inclusive range. */
export async function getAttendance(
  studentId: string,
  from?: string,
  to?: string,
): Promise<FamilyAttendanceRecord[]> {
  const params = new URLSearchParams();
  if (from) params.set("from", from);
  if (to) params.set("to", to);
  const query = params.toString() ? `?${params.toString()}` : "";
  const data = await unwrap<AttendanceEnvelope>(
    await apiFetch(`/api/v1/family/students/${studentId}/attendance${query}`),
    "attendance",
  );
  return data.records ?? [];
}

/**
 * Acknowledges one of the guardian's own children's notifications. The gateway
 * marks it on the guardian's behalf after verifying ownership, so this persists
 * server-side (the read state survives a relaunch), unlike the former
 * session-local hack.
 */
export async function markNotificationRead(notificationId: string): Promise<void> {
  const res = await apiFetch(`/api/v1/family/notifications/${notificationId}/read`, { method: "POST" });
  if (!res.ok) {
    throw new ApiError(res.status, "failed to mark notification read");
  }
}
