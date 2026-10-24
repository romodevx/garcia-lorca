/**
 * The families surface's pure rules (ADR 0041): who may do what, how a
 * suggestion is worded, and which pupils still lack a family. No I/O.
 */

import type { AdmissionStatus } from "../students/api";
import type { Family, FamilyMember, SuggestionConfidence, SuggestionReason } from "./api";

/** The server's cap (ADR 0041 D3): at most two app accounts per family. */
export const FAMILY_ACCOUNT_CAP = 2;

/** What the caller may do here. UX only: the gateway refuses the rest. */
export interface FamiliesAccess {
  /** The adults and the families' account counts. */
  canReadGuardians: boolean;
  /** Build, change and dissolve families; the adults and their access (owner). */
  canWriteGuardians: boolean;
  /** Rename a family; dismiss a suggestion. */
  canUpdateStudents: boolean;
}

export function familiesAccess(permissions: readonly string[] | undefined): FamiliesAccess {
  const has = (p: string): boolean => permissions?.includes(p) === true;
  return { canReadGuardians: has("guardians:read"), canWriteGuardians: has("guardians:write"), canUpdateStudents: has("students:update") };
}

const REASONS: Readonly<Record<SuggestionReason, string>> = {
  same_surnames: "Mismos apellidos",
  shared_contact_email: "Mismo correo",
  shared_contact_phone: "Mismo teléfono",
  shared_account: "Ya comparten cuenta",
};

export const reasonLabel = (reason: SuggestionReason): string => REASONS[reason] ?? reason;

export const confidenceLabel = (confidence: SuggestionConfidence): string => (confidence === "high" ? "Muy probable" : "Posible");

/** Every pupil's family, by pupil id. */
export function membershipOf(families: readonly Family[]): ReadonlyMap<string, Family> {
  const map = new Map<string, Family>();
  for (const family of families) for (const id of family.studentIds) map.set(id, family);
  return map;
}

/** A pupil the school no longer has is not somebody to place in a family. */
const GONE: ReadonlySet<AdmissionStatus> = new Set<AdmissionStatus>(["withdrawn", "rejected"]);

/** The current pupils that belong to no family. */
export function withoutFamily<T extends { id: string; admissionStatus: AdmissionStatus }>(pupils: readonly T[], membership: ReadonlyMap<string, Family>): T[] {
  return pupils.filter((p) => !membership.has(p.id) && !GONE.has(p.admissionStatus));
}

/**
 * The children a family account reaches: the server propagates an account to
 * every member except drafts (no CURP) and pupils the school no longer has.
 */
export function childrenAnAccountSees(members: readonly FamilyMember[]): FamilyMember[] {
  return members.filter((m) => !m.draft && !GONE.has(m.admissionStatus) && m.admissionStatus !== "draft");
}

/** Spanish writes «e», not «y», before the sound i (i-, hi-), never before a diphthong (hie-, io-, iu-). */
function conjunctionBefore(word: string): string {
  return /^h?[ií](?![aeou])/iu.test(word) ? "e" : "y";
}

/** "ANA", "ANA y LUIS", "ANA, LUIS e IVÁN". */
export function spanishList(words: readonly string[]): string {
  if (words.length <= 1) return words[0] ?? "";
  const last = words[words.length - 1] as string;
  return `${words.slice(0, -1).join(", ")} ${conjunctionBefore(last)} ${last}`;
}

/** What the screens read of a family to say who an account sees. */
export interface FamilyReach {
  studentIds: readonly string[];
  members: readonly FamilyMember[];
}

/** The family could not be read: the account may reach any of its children. */
export const REACH_UNKNOWN = "Verá a todos los alumnos de su familia";

/**
 * Security gate M2: before handing out an account, the owner reads which
 * children it will see. Nothing when it reaches one child or none. Fails
 * closed: an unread family, or one the service returned short, is described
 * as reaching every child it lists rather than only the ones it named.
 */
export function accessReachLine(family: FamilyReach | undefined): string | undefined {
  if (family === undefined) return REACH_UNKNOWN;
  if (family.members.length < family.studentIds.length) {
    return family.studentIds.length < 2 ? undefined : `Verá a ${family.studentIds.length} alumnos de su familia`;
  }
  const seen = childrenAnAccountSees(family.members);
  if (seen.length < 2) return undefined;
  return `Verá a ${spanishList(seen.map((m) => m.firstName.trim()))}`;
}

export const withApp = (families: readonly Family[]): number => families.filter((f) => (f.accountCount ?? 0) > 0).length;

export const accountsLabel = (count: number): string => `${count} de ${FAMILY_ACCOUNT_CAP}`;

/** Unknown (no guardians:read) is never "full": the server decides then. */
export const familyIsFull = (count: number | undefined): boolean => count !== undefined && count >= FAMILY_ACCOUNT_CAP;

/** The name a new family takes: its first pupil's surnames, as the server would. */
export function proposedFamilyName(first: { lastName: string; maternalLastName?: string } | undefined): string {
  if (first === undefined) return "";
  return [first.lastName.trim(), (first.maternalLastName ?? "").trim()].filter((s) => s !== "").join(" ");
}
