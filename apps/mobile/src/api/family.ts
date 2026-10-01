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
  /** The PATERNAL surname (a child captured before T3 may carry both here). */
  lastName: string;
  /** The maternal surname; empty or absent when not captured (T3). */
  maternalLastName?: string;
  /** The level's name as the school wrote it in Configuración ("Preescolar"). */
  level: string;
  /** The config level id. */
  levelId: string;
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

/**
 * What a ledger line records. scholarship (T18) and credit_note (T8) are
 * reserved: no line carries them yet, the screen already renders them.
 */
export type MovementKind = "charge" | "surcharge" | "discount" | "scholarship" | "payment" | "credit_note";

/**
 * One line of the child's ledger (T7), as finance worked it out. Amounts are
 * integer cents; `amountCents` is always positive and `kind` says which way it
 * moves the balance. The family's view never carries a voided deposit nor a
 * charge that has not opened.
 */
export interface FamilyMovement {
  kind: MovementKind;
  /** YYYY-MM-DD. */
  date: string;
  description: string;
  amountCents: number;
  /** The running balance after this line. Negative is the family's credit. */
  balanceCents: number;
  method?: string;
  cardType?: string;
  reference?: string;
  folio?: string;
  /** A surcharge earned up to today and not yet written down. */
  accrued: boolean;
  notYetDue: boolean;
  voided: boolean;
}

export interface FamilyStatementTotals {
  chargedCents: number;
  discountCents: number;
  paidCents: number;
  /** charged − discounts − paid. Negative is credit in the family's favour. */
  balanceCents: number;
  notYetDueCents: number;
}

/**
 * The child's account statement: the SAME ledger the school reads
 * (`finance.GetStatement`), in the family's view. The app renders it and
 * derives nothing from it.
 */
export interface FamilyStatement {
  movements: FamilyMovement[];
  totals: FamilyStatementTotals;
  /**
   * What the school is asking for TODAY: the months that have come due, already
   * carrying the surcharge that has accrued and net of the early-payment reward
   * — the counter's figure.
   */
  dueTodayCents: number;
  /** Its parts, so the screen can explain its own figure. */
  surchargeCents: number;
  earlyDiscountCents: number;
  /** The day those figures were worked out for, YYYY-MM-DD; empty when unknown. */
  asOf: string;
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

interface StudentsEnvelope {
  students: FamilyStudent[] | null;
}
interface GradesEnvelope {
  gradeEntries: FamilyGradeEntry[] | null;
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

const EMPTY_TOTALS: FamilyStatementTotals = { chargedCents: 0, discountCents: 0, paidCents: 0, balanceCents: 0, notYetDueCents: 0 };

/** Reads one child's account statement (T7): the ledger, its totals and today's figure. */
export async function getStatement(studentId: string): Promise<FamilyStatement> {
  const data = await unwrap<Partial<FamilyStatement> & { movements?: FamilyMovement[] | null }>(
    await apiFetch(`/api/v1/family/students/${studentId}/statement`),
    "statement",
  );
  return {
    movements: data.movements ?? [],
    totals: { ...EMPTY_TOTALS, ...data.totals },
    dueTodayCents: data.dueTodayCents ?? 0,
    surchargeCents: data.surchargeCents ?? 0,
    earlyDiscountCents: data.earlyDiscountCents ?? 0,
    asOf: data.asOf ?? "",
  };
}

/** The authenticated path of the statement's PDF. */
export function statementPdfPath(studentId: string): string {
  return `/api/v1/family/students/${studentId}/statement/pdf`;
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

/** The school's details as the card prints them (task 12's profile). */
export interface CardSchool {
  name: string;
  legalName: string;
  rfc: string;
  cct: string;
  address: string;
  phone: string;
  legend: string;
}

/**
 * A child's virtual card (task 13). `qr` is an opaque code (GLCARD:<token>):
 * it names nobody, and only the school's staff can read it back.
 */
export interface FamilyCard {
  studentId: string;
  name: string;
  level: string;
  gradeGroup: string;
  cycle: string;
  enrollmentNumber: string;
  school: CardSchool;
  /** Empty while the school has the card suspended. */
  qr: string;
  /** The school revoked the card and has not issued a new one yet. */
  suspended?: boolean;
}

/** Reads one child's virtual card. */
export async function getCard(studentId: string): Promise<FamilyCard> {
  return unwrap<FamilyCard>(await apiFetch(`/api/v1/family/students/${encodeURIComponent(studentId)}/card`), "card");
}

/** Where the child's photo is served; it needs the Bearer token like any read. */
export function cardPhotoPath(studentId: string): string {
  return `/api/v1/family/students/${encodeURIComponent(studentId)}/photo`;
}
