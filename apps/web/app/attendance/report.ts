/**
 * The attendance report (T12): a group's lists over a date range and, per
 * pupil, how many found them present, absent, late (retardo) or excused —
 * `GET /api/v1/attendance/reports` and its PDF and Excel exports. Requires
 * `attendance:read`; the session travels in the HttpOnly cookie.
 */

import { authFetch } from "../auth/authFetch";
import { attendanceFailure } from "./sheet";

export interface AttendanceCounts {
  present: number;
  absent: number;
  late: number;
  excused: number;
}

export interface AttendanceReportRow extends AttendanceCounts {
  enrollmentId: string;
  studentId: string;
  /** Surnames first; "" when the directory could not name the pupil. */
  name: string;
}

export interface AttendanceReport {
  groupId: string;
  groupLabel: string;
  from: string;
  to: string;
  /** The group's lists taken in the range, daily and per-class alike. */
  listsTaken: number;
  rows: AttendanceReportRow[];
  totals: AttendanceCounts;
}

export interface AttendanceReportQuery {
  groupId: string;
  /** Inclusive `YYYY-MM-DD` days, at most 366 apart. */
  from: string;
  to: string;
}

export type ReportFormat = "pdf" | "xlsx";

const REPORTS = "/api/v1/attendance/reports";
const FILENAME = /filename="?([^";]+)"?/;

function reportURL(baseURL: string, suffix: string, q: AttendanceReportQuery): string {
  const search = new URLSearchParams({ groupId: q.groupId, from: q.from, to: q.to });
  return `${baseURL.replace(/\/+$/, "")}${REPORTS}${suffix}?${search.toString()}`;
}

export async function getAttendanceReport(baseURL: string, q: AttendanceReportQuery): Promise<AttendanceReport> {
  const res = await authFetch(reportURL(baseURL, "", q), { credentials: "include" });
  if (!res.ok) {
    throw await attendanceFailure(res);
  }
  const raw = (await res.json()) as Omit<AttendanceReport, "rows"> & { rows?: AttendanceReportRow[] | null };
  return { ...raw, rows: raw.rows ?? [] };
}

/** An export (PDF or Excel), fetched with the session, ready to save. */
export async function downloadAttendanceReport(baseURL: string, q: AttendanceReportQuery, format: ReportFormat): Promise<{ blob: Blob; filename: string }> {
  const res = await authFetch(reportURL(baseURL, `/${format}`, q), { credentials: "include" });
  if (!res.ok) {
    throw await attendanceFailure(res);
  }
  const named = FILENAME.exec(res.headers.get("Content-Disposition") ?? "");
  return { blob: await res.blob(), filename: named?.[1] ?? `reporte-asistencia.${format}` };
}
