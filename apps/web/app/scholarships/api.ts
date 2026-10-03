/**
 * Typed fetch helpers for Becas (T18): `/api/v1/scholarships*`. Behind
 * `module_scholarships` (and `module_finance`) + cookie auth; reads need
 * `scholarships:read`, writes `scholarships:write`. This client is UX only —
 * the gateway is the boundary.
 *
 * `value` is integer on the wire: basis points for a percent (5000 = 50 %),
 * cents for the two fixed kinds. Errors are the gateway's text/plain
 * sentence, shown as-is.
 */
import { authFetch } from "../auth/authFetch";

/** percent: a share off · fixed_off: an amount off · fixed_price: what the family pays. */
export type ScholarshipKind = "percent" | "fixed_off" | "fixed_price";

export const SCHOLARSHIP_KINDS: readonly ScholarshipKind[] = ["percent", "fixed_off", "fixed_price"];

export type ScholarshipTargetGroup = "enrollment" | "tuition" | "concept" | "activity";

export interface ScholarshipInput {
  name: string;
  kind: ScholarshipKind;
  /** Basis points (1–10000) for percent; cents for fixed_off (≥ 1) and fixed_price (≥ 0). */
  value: number;
  /** `enrollment`, `tuition`, `concept:<id>`, `activity:<id>` — 1 to 50. */
  targets: string[];
  active: boolean;
}

export interface Scholarship extends ScholarshipInput {
  id: string;
  /** Pupils holding it today. */
  assignedCount: number;
}

export interface ScholarshipTarget {
  key: string;
  label: string;
  group: ScholarshipTargetGroup;
}

export interface ScholarshipOverlap {
  scholarshipId: string;
  name: string;
}

export interface ScholarshipHolder {
  studentId: string;
  startsOn: string;
  /** Other scholarships of the pupil sharing a target: they do not stack. */
  overlaps: ScholarshipOverlap[];
}

// --- transport -------------------------------------------------------------

const ID = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;
const SCHOLARSHIPS = "/api/v1/scholarships";

function url(baseURL: string, path: string): string {
  return `${baseURL.replace(/\/+$/, "")}${path}`;
}

/** An id that is not one never reaches the network (it would be a path). */
function idSegment(id: string): string {
  if (!ID.test(id)) {
    throw new Error("Ese registro no existe.");
  }
  return encodeURIComponent(id);
}

function scholarshipPath(id: string, suffix = ""): string {
  return `${SCHOLARSHIPS}/${idSegment(id)}${suffix}`;
}

/** The gateway's sentence, so the screen can say WHY it was refused. */
async function failure(res: Response): Promise<Error> {
  const body = (await res.text().catch(() => "")).trim();
  return new Error(body !== "" ? body : `HTTP ${res.status}`);
}

async function request<T>(baseURL: string, path: string, init: RequestInit = {}): Promise<T> {
  const res = await authFetch(url(baseURL, path), { credentials: "include", ...init });
  if (!res.ok) {
    throw await failure(res);
  }
  return (await res.json()) as T;
}

function send<T>(baseURL: string, path: string, method: "POST" | "PUT", body: unknown): Promise<T> {
  return request<T>(baseURL, path, { method, headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
}

type WireScholarship = Omit<Scholarship, "kind" | "targets" | "assignedCount"> & {
  kind: string;
  targets?: string[];
  assignedCount?: number;
};

type WireHolder = Omit<ScholarshipHolder, "overlaps"> & { overlaps?: ScholarshipOverlap[] };

function toScholarship(raw: WireScholarship): Scholarship {
  return {
    ...raw,
    kind: (SCHOLARSHIP_KINDS as readonly string[]).includes(raw.kind) ? (raw.kind as ScholarshipKind) : "percent",
    targets: raw.targets ?? [],
    assignedCount: raw.assignedCount ?? 0,
  };
}

function toHolder(raw: WireHolder): ScholarshipHolder {
  return { ...raw, overlaps: raw.overlaps ?? [] };
}

// --- scholarships ----------------------------------------------------------

export async function listScholarships(baseURL: string, includeInactive = false): Promise<Scholarship[]> {
  const res = await request<{ scholarships?: WireScholarship[] }>(baseURL, `${SCHOLARSHIPS}${includeInactive ? "?includeInactive=true" : ""}`);
  return (res.scholarships ?? []).map(toScholarship);
}

export async function getScholarship(baseURL: string, id: string): Promise<Scholarship> {
  return toScholarship(await request<WireScholarship>(baseURL, scholarshipPath(id)));
}

export async function createScholarship(baseURL: string, input: ScholarshipInput): Promise<Scholarship> {
  return toScholarship(await send<WireScholarship>(baseURL, SCHOLARSHIPS, "POST", input));
}

/** Pending charges take the new terms; paid ones keep what they were settled with. */
export async function updateScholarship(baseURL: string, id: string, input: ScholarshipInput): Promise<Scholarship> {
  return toScholarship(await send<WireScholarship>(baseURL, scholarshipPath(id), "PUT", input));
}

/** The inscription, the tuition, the active concepts and activities, as of today. */
export async function listScholarshipTargets(baseURL: string): Promise<ScholarshipTarget[]> {
  const res = await request<{ targets?: ScholarshipTarget[] }>(baseURL, `${SCHOLARSHIPS}/targets`);
  return res.targets ?? [];
}

// --- holders ---------------------------------------------------------------

export async function listScholarshipHolders(baseURL: string, id: string): Promise<ScholarshipHolder[]> {
  const res = await request<{ students?: WireHolder[] }>(baseURL, scholarshipPath(id, "/students"));
  return (res.students ?? []).map(toHolder);
}

/** Applies at once to their pending charges it targets. Returns the new holders. */
export async function assignScholarship(baseURL: string, id: string, studentIds: readonly string[]): Promise<ScholarshipHolder[]> {
  const res = await send<{ students?: WireHolder[] }>(baseURL, scholarshipPath(id, "/students"), "POST", { studentIds });
  return (res.students ?? []).map(toHolder);
}

/** Pending charges go back to full price; paid ones keep it. Returns how many lost it. */
export async function unassignScholarship(baseURL: string, id: string, studentIds: readonly string[]): Promise<number> {
  const res = await send<{ removed?: number }>(baseURL, scholarshipPath(id, "/students/remove"), "POST", { studentIds });
  return res.removed ?? 0;
}
