/**
 * Typed fetch helpers for the gateway's families endpoints (`/api/v1/families*`,
 * ADR 0041), behind `module_families` AND `module_students`. Same transport as
 * `app/students/api.ts`: cookie session (`credentials:"include"`), throw on any
 * non-2xx, unwrap the single-key envelopes. Shapes follow
 * `docs/api/openapi.yaml` (tag `families`) exactly.
 *
 * Reads need students:read (the adults and `accountCount`, guardians:read);
 * rename and dismiss need students:update; everything that hands out access —
 * create, add, remove, dissolve, adults, revoke — guardians:write. UX only: the
 * gateway is the security boundary.
 */

import { GENERIC_SERVER_FAILURE, spanishServerReason } from "../_lib/serverReason";
import { authFetch } from "../auth/authFetch";
import type { AdmissionStatus, Contact, Relationship } from "../students/api";

// --- entity types (match the gateway JSON contract) -----------------------

export interface Family {
  id: string;
  name: string;
  /** Ordered by surname, then first name. */
  studentIds: string[];
  /** Distinct app accounts (0–2). Present only for a caller holding guardians:read. */
  accountCount?: number;
  createdAt: string;
}

export interface FamilyMember {
  id: string;
  firstName: string;
  lastName: string;
  maternalLastName: string;
  level: string;
  gradeGroup: string;
  admissionStatus: AdmissionStatus;
  /** Imported without a CURP: never receives the family's accounts. */
  draft: boolean;
}

export interface FamilyDetail extends Family {
  /** In studentIds order; a pupil the service cannot return is omitted. */
  members: FamilyMember[];
}

export type SuggestionReason = "same_surnames" | "shared_contact_email" | "shared_contact_phone" | "shared_account";
export type SuggestionConfidence = "medium" | "high";

export interface FamilySuggestion {
  /** SHA-256 of the sorted pupil ids: the suggestion's identity. */
  signature: string;
  proposedName: string;
  studentIds: string[];
  reasons: SuggestionReason[];
  confidence: SuggestionConfidence;
}

/** One contact row of a person: the pair the invite route takes. */
export interface FamilyContactRef {
  studentId: string;
  contactId: string;
}

/** One adult of the family, however many pupils carry them as a contact. */
export interface FamilyPerson {
  fullName: string;
  relationship: Relationship;
  phone: string;
  email: string;
  /** The app account; absent when the person has none. */
  guardianUserId?: string;
  hasApp: boolean;
  contacts: FamilyContactRef[];
}

export interface CreateFamilyInput {
  /** Empty takes the first pupil's surnames (server side). */
  name: string;
  studentIds: readonly string[];
}

export interface FamilyContactInput {
  relationship: Relationship;
  fullName: string;
  phone: string;
  email: string;
  isEmergency: boolean;
}

// --- refusals ---------------------------------------------------------------

/** The machine codes a family write refuses with (`X-Error-Code`, 409). */
export const FAMILY_ACCOUNT_LIMIT = "FAMILY_ACCOUNT_LIMIT";
export const FAMILY_MANAGED_ACCESS = "FAMILY_MANAGED_ACCESS";
export const FAMILY_BUSY = "FAMILY_BUSY";

const FAMILY_COPY: Readonly<Record<string, string>> = {
  [FAMILY_ACCOUNT_LIMIT]: "Esta familia ya tiene 2 cuentas de la app",
  [FAMILY_MANAGED_ACCESS]: "Este acceso lo maneja la familia: quítalo desde la familia, no desde un solo hijo",
  [FAMILY_BUSY]: "Otra persona está cambiando esta familia; intenta de nuevo",
};

function fallbackFor(status: number): string {
  if (status === 403) return "Tu cuenta no tiene permiso para esta acción.";
  if (status === 404) return "No encontramos esa familia. Actualiza la página e intenta de nuevo.";
  if (status === 409) return "Los datos cambiaron mientras tanto. Actualiza la página e intenta de nuevo.";
  return GENERIC_SERVER_FAILURE;
}

/** A families failure: the status and code to branch on, `message` to show. */
export class FamiliesError extends Error {
  readonly status: number;
  /** The server's own words. */
  readonly detail: string;
  /** `X-Error-Code`, e.g. FAMILY_ACCOUNT_LIMIT. */
  readonly code: string | undefined;

  constructor(status: number, detail: string, code?: string) {
    super((code !== undefined ? FAMILY_COPY[code] : undefined) ?? spanishServerReason(detail) ?? fallbackFor(status));
    this.name = "FamiliesError";
    this.status = status;
    this.detail = detail;
    this.code = code;
  }
}

/**
 * What to tell the office about any failed family-related write, including the
 * contact invite (a StudentsError carrying FAMILY_ACCOUNT_LIMIT). Errors that
 * are not ours (a network failure) read `fallback`.
 */
export function familyRefusal(cause: unknown, fallback: string): string {
  if (typeof cause !== "object" || cause === null || !("status" in cause)) return fallback;
  const code = "code" in cause && typeof cause.code === "string" ? cause.code : undefined;
  const copy = code !== undefined ? FAMILY_COPY[code] : undefined;
  if (copy !== undefined) return copy;
  return cause instanceof Error && cause.message !== "" ? cause.message : fallback;
}

// --- transport ----------------------------------------------------------------

const FAMILIES = "/api/v1/families";

function familiesURL(baseURL: string, rest = ""): string {
  return `${baseURL.replace(/\/+$/, "")}${FAMILIES}${rest}`;
}

const idPath = (id: string): string => `/${encodeURIComponent(id)}`;

async function failure(res: Response): Promise<FamiliesError> {
  const body = await res.text().catch(() => "");
  return new FamiliesError(res.status, body.trim(), res.headers.get("X-Error-Code") ?? undefined);
}

async function request(target: string, method: string, body?: unknown): Promise<Response> {
  const res = await authFetch(target, {
    method,
    credentials: "include",
    ...(body === undefined ? {} : { headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) }),
  });
  if (!res.ok) throw await failure(res);
  return res;
}

async function readJSON<T>(target: string, method = "GET", body?: unknown): Promise<T> {
  return (await (await request(target, method, body)).json()) as T;
}

// --- reads --------------------------------------------------------------------

export async function listFamilies(baseURL: string): Promise<Family[]> {
  return (await readJSON<{ families?: Family[] }>(familiesURL(baseURL))).families ?? [];
}

export async function listFamilySuggestions(baseURL: string): Promise<FamilySuggestion[]> {
  return (await readJSON<{ suggestions?: FamilySuggestion[] }>(familiesURL(baseURL, "/suggestions"))).suggestions ?? [];
}

export function getFamily(baseURL: string, familyId: string): Promise<FamilyDetail> {
  return readJSON<FamilyDetail>(familiesURL(baseURL, idPath(familyId)));
}

/** Needs guardians:read. */
export async function listFamilyContacts(baseURL: string, familyId: string): Promise<FamilyPerson[]> {
  return (await readJSON<{ people?: FamilyPerson[] }>(familiesURL(baseURL, `${idPath(familyId)}/contacts`))).people ?? [];
}

// --- writes -------------------------------------------------------------------

export function createFamily(baseURL: string, input: CreateFamilyInput): Promise<Family> {
  const name = input.name.trim();
  return readJSON<Family>(familiesURL(baseURL), "POST", { ...(name === "" ? {} : { name }), studentIds: input.studentIds });
}

export function renameFamily(baseURL: string, familyId: string, name: string): Promise<Family> {
  return readJSON<Family>(familiesURL(baseURL, idPath(familyId)), "PATCH", { name: name.trim() });
}

export async function dissolveFamily(baseURL: string, familyId: string): Promise<void> {
  await request(familiesURL(baseURL, idPath(familyId)), "DELETE");
}

export async function dismissFamilySuggestion(baseURL: string, studentIds: readonly string[]): Promise<void> {
  await request(familiesURL(baseURL, "/suggestions/dismiss"), "POST", { studentIds });
}

export function addStudentsToFamily(baseURL: string, familyId: string, studentIds: readonly string[]): Promise<Family> {
  return readJSON<Family>(familiesURL(baseURL, `${idPath(familyId)}/students`), "POST", { studentIds });
}

/** The family without the pupil, or null when it was the last one (the family is gone). */
export async function removeStudentFromFamily(baseURL: string, familyId: string, studentId: string): Promise<Family | null> {
  const res = await request(familiesURL(baseURL, `${idPath(familyId)}/students${idPath(studentId)}`), "DELETE");
  return res.status === 204 ? null : ((await res.json()) as Family);
}

/** Writes the adult on every pupil of the family: one contact per pupil comes back. */
export async function addFamilyContact(baseURL: string, familyId: string, input: FamilyContactInput): Promise<Contact[]> {
  const body = {
    relationship: input.relationship,
    fullName: input.fullName.trim(),
    ...(input.phone.trim() === "" ? {} : { phone: input.phone.trim() }),
    ...(input.email.trim() === "" ? {} : { email: input.email.trim() }),
    ...(input.isEmergency ? { isEmergency: true } : {}),
  };
  return (await readJSON<{ contacts?: Contact[] }>(familiesURL(baseURL, `${idPath(familyId)}/contacts`), "POST", body)).contacts ?? [];
}

/** Unlinks the account from every pupil of the family; answers how many contacts lost it. */
export async function revokeFamilyAccess(baseURL: string, familyId: string, guardianUserId: string): Promise<number> {
  return (await readJSON<{ contacts: number }>(familiesURL(baseURL, `${idPath(familyId)}/access/revoke`), "POST", { guardianUserId })).contacts;
}
