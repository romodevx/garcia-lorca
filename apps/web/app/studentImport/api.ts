/**
 * The gateway's legacy pupil import (ADR 0033, tag `student-import` in
 * docs/api/openapi.yaml). The preview writes nothing; the commit re-sends the
 * same file with the preview's planHash and writes only what the preview
 * showed. Behind module_students + module_config + module_student_import and
 * the owner-only `students:import`; the gateway is the boundary, this is UX.
 *
 * Types mirror the contract's StudentImportRequest / StudentImportResult.
 */
import { authFetch } from "../auth/authFetch";

const BASE_PATH = "/api/v1/imports/students";

/** Where one level of the file goes: an existing level, or a new one of the file's name. */
export type LevelChoice = { levelId: string } | { create: true };

export interface StudentImportRequest {
  fileName: string;
  /** The .xlsx, base64 (≤ 2 MB decoded). */
  fileBase64: string;
  /** Keyed by the file's own name for the level ("Preparatoria"). */
  levels?: Record<string, LevelChoice>;
  /** Capacity of each group the import creates, keyed exactly by placement.key. */
  capacities?: Record<string, number>;
  /** The preview's planHash; required by the commit. */
  planHash?: string;
  /**
   * module_import_review: the office's decision per row (only rows that differ
   * from a plain import). The commit must carry the same as its preview.
   */
  rows?: RowDecision[];
}

/** What the office does with one row of the file. */
export type RowAction = "import" | "omit" | "draft";

/** Corrections to a row's values, validated on the server like the file's own. */
export interface RowOverrides {
  curp?: string;
  /** dd/mm/yyyy or yyyy-mm-dd. */
  birthDate?: string;
  homePhone?: string;
  mobilePhone?: string;
  email?: string;
  postalCode?: string;
  state?: string;
  bankReference?: string;
  municipality?: string;
  street?: string;
  /** "APELLIDOS, NOMBRES", as the previous system writes it. */
  fullName?: string;
}

export type OverrideName = keyof RowOverrides;

export interface RowDecision {
  row: number;
  action?: RowAction;
  overrides?: RowOverrides;
}

/** The part of a pupil's file an import issue is about. */
export type IssueField =
  | "birth_date"
  | "phone_home"
  | "phone_mobile"
  | "email"
  | "postal_code"
  | "state"
  | "bank_reference"
  | "surnames"
  | "first_name"
  | "address"
  | "curp";

/** One value of a preview row the import changes or drops. */
export interface ImportRowIssue {
  field: IssueField;
  code: string;
  /** What the file said. */
  original: string;
  /** What would be stored; "" when dropped. */
  stored: string;
}

/** One refused decision (422 IMPORT_ROWS_INVALID): the row, the override (or "action"/"row") and why. */
export interface RowFieldError {
  row: number;
  field: string;
  code: string;
}

export type LevelStatus = "matched" | "chosen" | "create" | "unresolved";

export interface ImportLevel {
  fileName: string;
  status: LevelStatus;
  levelId?: string;
  levelName?: string;
  students: number;
  /** No active tuition table in the cycle: no colegiatura will be billed. */
  noTuition: boolean;
}

export interface ImportPlacement {
  /** `level|ordinal(2 digits)|group` — the capacities key. */
  key: string;
  level: string;
  grade: string;
  group: string;
  students: number;
  capacity: number;
  levelCreated: boolean;
  gradeCreated: boolean;
  groupCreated: boolean;
  error?: "level_not_found" | "level_being_deleted" | "invalid_placement";
}

export type RowStatus = "new" | "warning" | "already" | "skipped" | "pending";

export interface ImportRow {
  /** The spreadsheet row (the header is 1). */
  row: number;
  name: string;
  curp: string;
  destination: string;
  status: RowStatus;
  reason?: string;
  warnings?: string[];
  enrollmentNumber?: string;
  /** module_import_review: per field, what the import changes or drops. */
  issues?: ImportRowIssue[];
  /** module_import_review: the CURP is missing or invalid, so it may go in as a draft. */
  canDraft?: boolean;
  /** module_import_review: the row is (or would be) stored as a draft. */
  draft?: boolean;
}

export interface ImportSummary {
  total: number;
  new: number;
  /** A subset of `new`. */
  warnings: number;
  already: number;
  skipped: number;
  pending: number;
  /** A subset of `already`: earlier imports moved (preview: to move) to the cycle's first day. */
  movedToCycleStart: number;
}

export interface StudentImportResult {
  cycle: { id: string; name: string };
  levels: ImportLevel[];
  placements: ImportPlacement[];
  rows: ImportRow[];
  summary: ImportSummary;
  unresolved: string[];
  planHash: string;
  /** Set by the commit. */
  batchId?: string;
  /** Set by the commit only with module_families on and guardians:write held. */
  families?: StudentImportFamilies;
}

/** A family the import built; `name` and `studentIds` only for students:read. */
export interface StudentImportFamily {
  id: string;
  name?: string;
  studentIds?: string[];
}

/** The very likely families the commit built without asking (best effort). */
export interface StudentImportFamilies {
  /** False when the build was cut partway: some families may exist anyway. */
  built: boolean;
  created: StudentImportFamily[];
  joined: StudentImportFamily[];
  /** Groups left to the office as suggestions. */
  skipped: number;
  /** Suggestions still to review; absent when they could not be read. */
  toReview?: number;
}

/** What undoing an import's families did. */
export interface StudentImportFamiliesUndo {
  removed: number;
  dissolved: number;
  /** Families left whole because a member has an app account. */
  keptWithAccess: number;
}

/** The refusal codes the gateway puts in X-Error-Code for this flow. */
export type StudentImportCode =
  | "FILE_NOT_XLSX"
  | "FILE_EMPTY"
  | "FILE_MISSING_COLUMN"
  | "FILE_TOO_LARGE"
  | "NO_ACTIVE_CYCLE"
  | "PREVIEW_STALE"
  | "LEVELS_UNRESOLVED"
  | "IMPORT_ROWS_INVALID";

const ROWS_INVALID = "IMPORT_ROWS_INVALID";

/** A refusal the screen can act on: the status, the machine code and the body's words. */
export class StudentImportError extends Error {
  readonly status: number;
  readonly code: string | undefined;
  /** The body: for FILE_MISSING_COLUMN, the missing column's name. */
  readonly detail: string;
  /** IMPORT_ROWS_INVALID: each refused decision; empty otherwise. */
  readonly rows: readonly RowFieldError[];

  constructor(status: number, detail: string, code: string | undefined, rows: readonly RowFieldError[] = []) {
    super(detail !== "" ? detail : `HTTP ${status}`);
    this.name = "StudentImportError";
    this.status = status;
    this.detail = detail;
    this.code = code;
    this.rows = rows;
  }
}

/** The refused decisions of a 422 IMPORT_ROWS_INVALID envelope; [] when unreadable. */
function refusedRows(body: string): RowFieldError[] {
  try {
    const parsed = JSON.parse(body) as { error?: { details?: { rows?: RowFieldError[] } } };
    return parsed.error?.details?.rows ?? [];
  } catch {
    return [];
  }
}

async function postJSON<T>(baseURL: string, path: string, body?: unknown): Promise<T> {
  const res = await authFetch(`${baseURL.replace(/\/+$/, "")}${BASE_PATH}/${path}`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
  if (!res.ok) {
    const detail = (await res.text().catch(() => "")).trim();
    const code = res.headers.get("X-Error-Code") ?? undefined;
    throw new StudentImportError(res.status, detail, code, code === ROWS_INVALID ? refusedRows(detail) : []);
  }
  return (await res.json()) as T;
}

function post(baseURL: string, action: "preview" | "commit", body: StudentImportRequest): Promise<StudentImportResult> {
  return postJSON<StudentImportResult>(baseURL, action, body);
}

/** Takes back the families the commit of `batchId` built. Idempotent. */
export function undoStudentImportFamilies(baseURL: string, batchId: string): Promise<StudentImportFamiliesUndo> {
  return postJSON<StudentImportFamiliesUndo>(baseURL, `${encodeURIComponent(batchId)}/families/undo`);
}

/** What the commit would do, row by row. Writes nothing. */
export function previewStudentImport(baseURL: string, body: StudentImportRequest): Promise<StudentImportResult> {
  return post(baseURL, "preview", body);
}

/** Writes what the preview showed; 409 PREVIEW_STALE when the plan changed since. */
export function commitStudentImport(baseURL: string, body: StudentImportRequest): Promise<StudentImportResult> {
  return post(baseURL, "commit", body);
}
