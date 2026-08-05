/**
 * Typed fetch helpers for the gateway's academic-catalog endpoints
 * (`/api/v1/config/*`), gated behind the `module_config` flag + cookie auth. The
 * session lives ONLY in the HttpOnly cookie the gateway set, so every call uses
 * `credentials:"include"`; nothing secret ever travels in the body or response.
 *
 * Reads require `config:read`, writes `config:write` (the owner role holds both).
 * These helpers mirror `app/auth/api.ts`: a `url()` trailing-slash guard, throw on
 * any non-2xx, and parse the gateway's `{cycles:[…]}`/`{levels:[…]}`/… envelopes.
 * The shapes match the gateway REST contract exactly; dates are `YYYY-MM-DD`.
 */

// --- entity types (match the gateway JSON contract) -----------------------

export type CycleStatus = "draft" | "active" | "closed";

export interface Cycle {
  id: string;
  name: string;
  status: CycleStatus;
  startsOn: string;
  endsOn: string;
}

export interface CreateCycleInput {
  name: string;
  /** Omit to let the server default to "draft". */
  status?: CycleStatus;
  startsOn: string;
  endsOn: string;
}

export type EnrollmentGranularity = "cycle" | "term";

export interface Level {
  id: string;
  name: string;
  sepCode: string;
  enrollmentGranularity: EnrollmentGranularity;
}

export interface CreateLevelInput {
  name: string;
  sepCode: string;
  enrollmentGranularity: EnrollmentGranularity;
}

export interface GradeLevel {
  id: string;
  levelId: string;
  name: string;
  ordinal: number;
}

export interface CreateGradeLevelInput {
  levelId: string;
  name: string;
  ordinal: number;
}

export interface Subject {
  id: string;
  gradeLevelId: string;
  name: string;
}

export interface CreateSubjectInput {
  gradeLevelId: string;
  name: string;
}

export interface Term {
  id: string;
  cycleId: string;
  levelId: string;
  name: string;
  ordinal: number;
  startsOn: string;
  endsOn: string;
}

export interface CreateTermInput {
  cycleId: string;
  levelId: string;
  name: string;
  ordinal: number;
  startsOn: string;
  endsOn: string;
}

export interface Group {
  id: string;
  cycleId: string;
  gradeLevelId: string;
  name: string;
  capacity: number;
}

export interface CreateGroupInput {
  cycleId: string;
  gradeLevelId: string;
  name: string;
  capacity: number;
}

// --- transport helpers ----------------------------------------------------

const BASE_PATH = "/api/v1/config";

function url(baseURL: string, path: string): string {
  return `${baseURL.replace(/\/+$/, "")}${path}`;
}

/** POST a JSON body and return the created entity. Throws on any non-2xx. */
async function postJSON<T>(baseURL: string, path: string, body: unknown): Promise<T> {
  const res = await fetch(url(baseURL, `${BASE_PATH}${path}`), {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    throw new Error(`config ${path}: HTTP ${res.status}`);
  }
  return (await res.json()) as T;
}

/** GET a list endpoint and unwrap the single-key envelope (`{ <key>: [...] }`). */
async function getList<T>(baseURL: string, path: string, key: string): Promise<T[]> {
  const res = await fetch(url(baseURL, `${BASE_PATH}${path}`), { credentials: "include" });
  if (!res.ok) {
    throw new Error(`config ${path}: HTTP ${res.status}`);
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

// --- cycles ---------------------------------------------------------------

export function createCycle(baseURL: string, input: CreateCycleInput): Promise<Cycle> {
  return postJSON<Cycle>(baseURL, "/cycles", input);
}

export function listCycles(baseURL: string): Promise<Cycle[]> {
  return getList<Cycle>(baseURL, "/cycles", "cycles");
}

// --- levels ---------------------------------------------------------------

export function createLevel(baseURL: string, input: CreateLevelInput): Promise<Level> {
  return postJSON<Level>(baseURL, "/levels", input);
}

export function listLevels(baseURL: string): Promise<Level[]> {
  return getList<Level>(baseURL, "/levels", "levels");
}

// --- grade levels ---------------------------------------------------------

export function createGradeLevel(baseURL: string, input: CreateGradeLevelInput): Promise<GradeLevel> {
  return postJSON<GradeLevel>(baseURL, "/grade-levels", input);
}

export function listGradeLevels(baseURL: string, levelId: string): Promise<GradeLevel[]> {
  return getList<GradeLevel>(baseURL, `/grade-levels${query({ levelId })}`, "gradeLevels");
}

// --- subjects -------------------------------------------------------------

export function createSubject(baseURL: string, input: CreateSubjectInput): Promise<Subject> {
  return postJSON<Subject>(baseURL, "/subjects", input);
}

export function listSubjects(baseURL: string, gradeLevelId: string): Promise<Subject[]> {
  return getList<Subject>(baseURL, `/subjects${query({ gradeLevelId })}`, "subjects");
}

// --- terms ----------------------------------------------------------------

export function createTerm(baseURL: string, input: CreateTermInput): Promise<Term> {
  return postJSON<Term>(baseURL, "/terms", input);
}

export function listTerms(baseURL: string, cycleId: string, levelId?: string): Promise<Term[]> {
  return getList<Term>(baseURL, `/terms${query({ cycleId, levelId })}`, "terms");
}

// --- groups ---------------------------------------------------------------

export function createGroup(baseURL: string, input: CreateGroupInput): Promise<Group> {
  return postJSON<Group>(baseURL, "/groups", input);
}

export function listGroups(baseURL: string, cycleId: string, gradeLevelId?: string): Promise<Group[]> {
  return getList<Group>(baseURL, `/groups${query({ cycleId, gradeLevelId })}`, "groups");
}
