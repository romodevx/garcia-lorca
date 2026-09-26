/**
 * The card-scan vocabulary shared by the director client
 * (`POST /api/v1/attendance/scan`) and the teacher's self-scoped one
 * (`POST /api/v1/teaching/attendance/scan`): both take the same body and answer
 * the same way, so the shape and the response reading live here once.
 *
 * A rejected card is an EXPECTED outcome while scanning a queue of students, so
 * `readScanResponse` never throws on a 4xx: it returns a rejection the scanner
 * shows and moves on. Only the network itself throws.
 */

import { authFetch } from "../auth/authFetch";

export interface ScanCardInput {
  groupId: string;
  /** Only for a per-class list. */
  subjectId?: string;
  /** The QR's raw content (`GLCARD:<token>`). */
  code: string;
}

export interface ScannedStudent {
  enrollmentId: string;
  studentId: string;
  name: string;
}

export type ScanRejection = "invalid" | "foreign" | "suspended" | "denied" | "unknown";

export type CardScanResult =
  | { kind: "ok"; student: ScannedStudent }
  | { kind: "rejected"; reason: ScanRejection; message: string };

const HTTP_NOT_FOUND = 404;
const HTTP_CONFLICT = 409;
const HTTP_UNPROCESSABLE = 422;
const HTTP_UNAUTHORIZED = 401;
const HTTP_FORBIDDEN = 403;

const FALLBACK: Record<ScanRejection, string> = {
  invalid: "Código no válido",
  foreign: "Este alumno no pertenece a este grupo",
  suspended: "Credencial suspendida",
  denied: "No tienes permiso para registrar asistencia en esta lista.",
  unknown: "No se pudo leer la credencial. Intenta de nuevo.",
};

/** Which rejection a failed response is; anything unrecognised is "unknown". */
function reasonOf(res: Response): ScanRejection {
  const header = res.headers.get("X-Scan-Result");
  if (res.status === HTTP_UNPROCESSABLE && (header === "invalid" || header === "foreign")) return header;
  if (res.status === HTTP_CONFLICT) return "suspended";
  if (res.status === HTTP_NOT_FOUND || res.status === HTTP_FORBIDDEN || res.status === HTTP_UNAUTHORIZED) return "denied";
  return "unknown";
}

const isScannedStudent = (value: unknown): value is ScannedStudent => {
  if (typeof value !== "object" || value === null) return false;
  const v = value as Record<string, unknown>;
  return typeof v.enrollmentId === "string" && v.enrollmentId !== "" && typeof v.studentId === "string" && typeof v.name === "string";
};

/** Reads a scan response into its outcome; the server's text/plain reason is shown as is. */
export async function readScanResponse(res: Response): Promise<CardScanResult> {
  if (res.ok) {
    const body: unknown = await res.json().catch(() => undefined);
    if (isScannedStudent(body)) return { kind: "ok", student: { enrollmentId: body.enrollmentId, studentId: body.studentId, name: body.name } };
    return { kind: "rejected", reason: "unknown", message: FALLBACK.unknown };
  }
  const reason = reasonOf(res);
  const text = (await res.text().catch(() => "")).trim();
  return { kind: "rejected", reason, message: text !== "" ? text : FALLBACK[reason] };
}

/** The POST body: subjectId only when there is one. */
function scanBody(input: ScanCardInput): Record<string, string> {
  const subject = input.subjectId !== undefined && input.subjectId !== "" ? { subjectId: input.subjectId } : {};
  return { groupId: input.groupId, ...subject, code: input.code };
}

/** POSTs one scanned code to `url` and reads the outcome. */
export async function postCardScan(url: string, input: ScanCardInput): Promise<CardScanResult> {
  const res = await authFetch(url, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(scanBody(input)),
  });
  return readScanResponse(res);
}
