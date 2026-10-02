/**
 * Typed fetch helpers for Actividades (T17): `/api/v1/activities*`. Behind
 * `module_activities` (and `module_finance`) + cookie auth; reads need
 * `activities:read`, writes `activities:write`. This client is UX only — the
 * gateway is the boundary.
 *
 * MONEY IS ALWAYS INTEGER CENTS on the wire. Errors are the gateway's
 * text/plain sentence, shown as-is.
 */
import { authFetch } from "../auth/authFetch";

export type Periodicity = "monthly" | "weekly" | "per_cycle";

export const PERIODICITIES: readonly Periodicity[] = ["monthly", "weekly", "per_cycle"];

/** 1 = Monday … 7 = Sunday. */
export type Weekday = 1 | 2 | 3 | 4 | 5 | 6 | 7;

export interface ActivitySlot {
  weekday: number;
  /** HH:MM */
  start: string;
  /** HH:MM, after start. */
  end: string;
}

export interface ActivityPriceOption {
  /** Present on what the server returns; send it to edit an option in place. */
  id?: string;
  /** Days a week attended for this price; null is the flat fee. */
  daysPerWeek: number | null;
  amountCents: number;
}

export interface ActivityInput {
  name: string;
  schedule: ActivitySlot[];
  periodicity: Periodicity;
  billingStartsOn: string;
  firstDueOn: string;
  lateFeeCents: number;
  active: boolean;
  options: ActivityPriceOption[];
}

export interface Activity extends ActivityInput {
  id: string;
  enrolledCount: number;
}

export interface ActivityEnrollment {
  id: string;
  activityId: string;
  studentId: string;
  optionId: string;
  startsOn: string;
  endsOn?: string | null;
}

export interface EnrollmentRequest {
  studentId: string;
  optionId: string;
}

// --- transport -------------------------------------------------------------

const ID = /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/;
const ACTIVITIES = "/api/v1/activities";

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

function activityPath(id: string, suffix = ""): string {
  return `${ACTIVITIES}/${idSegment(id)}${suffix}`;
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

type WireOption = Omit<ActivityPriceOption, "daysPerWeek"> & { daysPerWeek?: number | null };
type WireActivity = Omit<Activity, "periodicity" | "options" | "schedule" | "lateFeeCents" | "enrolledCount"> & {
  periodicity: string;
  options?: WireOption[];
  schedule?: ActivitySlot[];
  lateFeeCents?: number;
  enrolledCount?: number;
};

function toActivity(raw: WireActivity): Activity {
  return {
    ...raw,
    periodicity: (PERIODICITIES as readonly string[]).includes(raw.periodicity) ? (raw.periodicity as Periodicity) : "monthly",
    schedule: raw.schedule ?? [],
    options: (raw.options ?? []).map((o) => ({ ...o, daysPerWeek: o.daysPerWeek ?? null })),
    lateFeeCents: raw.lateFeeCents ?? 0,
    enrolledCount: raw.enrolledCount ?? 0,
  };
}

// --- activities ------------------------------------------------------------

export async function listActivities(baseURL: string, includeInactive = false): Promise<Activity[]> {
  const res = await request<{ activities?: WireActivity[] }>(baseURL, `${ACTIVITIES}${includeInactive ? "?includeInactive=true" : ""}`);
  return (res.activities ?? []).map(toActivity);
}

export async function getActivity(baseURL: string, id: string): Promise<Activity> {
  return toActivity(await request<WireActivity>(baseURL, activityPath(id)));
}

export async function createActivity(baseURL: string, input: ActivityInput): Promise<Activity> {
  return toActivity(await send<WireActivity>(baseURL, ACTIVITIES, "POST", input));
}

/** Options sent with their id are edited in place; one left out is removed. */
export async function updateActivity(baseURL: string, id: string, input: ActivityInput): Promise<Activity> {
  return toActivity(await send<WireActivity>(baseURL, activityPath(id), "PUT", input));
}

// --- enrolments ------------------------------------------------------------

export async function listEnrollments(baseURL: string, activityId: string): Promise<ActivityEnrollment[]> {
  const res = await request<{ enrollments?: ActivityEnrollment[] }>(baseURL, activityPath(activityId, "/enrollments"));
  return res.enrollments ?? [];
}

/** Enrolls from today; the periods already billable are charged at once. */
export async function enrollStudents(
  baseURL: string,
  activityId: string,
  enrollments: readonly EnrollmentRequest[],
): Promise<{ enrollments: ActivityEnrollment[]; chargesCreated: number }> {
  const res = await send<{ enrollments?: ActivityEnrollment[]; chargesCreated?: number }>(
    baseURL,
    activityPath(activityId, "/enrollments"),
    "POST",
    { enrollments },
  );
  return { enrollments: res.enrollments ?? [], chargesCreated: res.chargesCreated ?? 0 };
}

/** Ends today; charges already issued are never touched. Returns how many ended. */
export async function endEnrollments(baseURL: string, activityId: string, studentIds: readonly string[]): Promise<number> {
  const res = await send<{ ended?: number }>(baseURL, activityPath(activityId, "/enrollments/end"), "POST", { studentIds });
  return res.ended ?? 0;
}

/** Applies to periods billed from now on. */
export function changeActivityOption(baseURL: string, activityId: string, studentId: string, optionId: string): Promise<ActivityEnrollment> {
  return send<ActivityEnrollment>(baseURL, activityPath(activityId, `/enrollments/${idSegment(studentId)}`), "PUT", { optionId });
}
