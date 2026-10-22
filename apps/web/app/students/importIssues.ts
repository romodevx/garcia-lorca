/**
 * A pupil's import issues (PLAN-import-avisos, module_import_review): every
 * value the legacy import changed or dropped, kept with the pupil until the
 * office corrects it (saving the file) or accepts it («Está bien así»). The
 * routes need module_students AND module_import_review; listing is
 * students:read, accepting students:update. The gateway is the boundary.
 */
import { authFetch } from "../auth/authFetch";
import type { IssueField } from "../studentImport/api";
import { StudentsError, type Student } from "./api";

export interface StudentImportIssue {
  id: string;
  studentId: string;
  field: IssueField;
  code: string;
  /** What the Excel said. */
  originalValue: string;
  /** What was stored; "" when the value was dropped. */
  storedValue: string;
  createdAt: string;
  resolvedAt?: string;
  /** "" while open; "corrected" or "accepted" once closed. */
  resolution: string;
  /** On a draft's curp issue: the group the import meant the pupil for. */
  intendedGroupId?: string;
}

/** The refusal code of an action a draft cannot take (enroll, move, link, invite). */
export const STUDENT_DRAFT = "STUDENT_DRAFT";

function issuesPath(baseURL: string, studentId: string, rest = ""): string {
  return `${baseURL.replace(/\/+$/, "")}/api/v1/students/${encodeURIComponent(studentId)}/import-issues${rest}`;
}

async function refusal(res: Response): Promise<StudentsError> {
  const body = await res.text().catch(() => "");
  return new StudentsError(res.status, body.trim(), res.headers.get("X-Error-Code") ?? undefined);
}

/** The pupil's open import issues. */
export async function listImportIssues(baseURL: string, studentId: string): Promise<StudentImportIssue[]> {
  const res = await authFetch(issuesPath(baseURL, studentId), { credentials: "include" });
  if (!res.ok) throw await refusal(res);
  const data = (await res.json()) as { issues?: StudentImportIssue[] };
  return data.issues ?? [];
}

/** «Está bien así»: closes the issue without changing the value. */
export async function acceptImportIssue(baseURL: string, studentId: string, issueId: string): Promise<StudentImportIssue> {
  const res = await authFetch(issuesPath(baseURL, studentId, `/${encodeURIComponent(issueId)}/accept`), {
    method: "POST",
    credentials: "include",
  });
  if (!res.ok) throw await refusal(res);
  return (await res.json()) as StudentImportIssue;
}

/** A pupil the import stored without a valid CURP. */
export function isDraft(s: Pick<Student, "draft" | "admissionStatus">): boolean {
  return s.draft === true || s.admissionStatus === "draft";
}

/** The ⚠ badge's accessible name. */
export function issuesLabel(count: number): string {
  return `${count} ${count === 1 ? "aviso" : "avisos"} de importación`;
}

/** Why a draft is not offered what it cannot do. */
export const DRAFT_NOTE = "Es un borrador: falta la CURP. Captúrala en «Editar ficha» para poder inscribirlo, cambiarlo de grupo o darle acceso a la familia.";

/** The same, when the server refused it (409 STUDENT_DRAFT). */
export const DRAFT_PLACEMENT_REFUSAL = "Es un borrador: falta la CURP. Captúrala en «Editar ficha» para poder inscribirlo.";
