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
}

export interface ImportSummary {
  total: number;
  new: number;
  /** A subset of `new`. */
  warnings: number;
  already: number;
  skipped: number;
  pending: number;
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
}

/** The refusal codes the gateway puts in X-Error-Code for this flow. */
export type StudentImportCode =
  | "FILE_NOT_XLSX"
  | "FILE_EMPTY"
  | "FILE_MISSING_COLUMN"
  | "FILE_TOO_LARGE"
  | "NO_ACTIVE_CYCLE"
  | "PREVIEW_STALE"
  | "LEVELS_UNRESOLVED";

/** A refusal the screen can act on: the status, the machine code and the body's words. */
export class StudentImportError extends Error {
  readonly status: number;
  readonly code: string | undefined;
  /** The body: for FILE_MISSING_COLUMN, the missing column's name. */
  readonly detail: string;

  constructor(status: number, detail: string, code: string | undefined) {
    super(detail !== "" ? detail : `HTTP ${status}`);
    this.name = "StudentImportError";
    this.status = status;
    this.detail = detail;
    this.code = code;
  }
}

async function post(baseURL: string, action: "preview" | "commit", body: StudentImportRequest): Promise<StudentImportResult> {
  const res = await authFetch(`${baseURL.replace(/\/+$/, "")}${BASE_PATH}/${action}`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const detail = (await res.text().catch(() => "")).trim();
    throw new StudentImportError(res.status, detail, res.headers.get("X-Error-Code") ?? undefined);
  }
  return (await res.json()) as StudentImportResult;
}

/** What the commit would do, row by row. Writes nothing. */
export function previewStudentImport(baseURL: string, body: StudentImportRequest): Promise<StudentImportResult> {
  return post(baseURL, "preview", body);
}

/** Writes what the preview showed; 409 PREVIEW_STALE when the plan changed since. */
export function commitStudentImport(baseURL: string, body: StudentImportRequest): Promise<StudentImportResult> {
  return post(baseURL, "commit", body);
}
