import { apiFetch } from "./client";
import { ApiError } from "./errors";

/**
 * A guardian's view of one child's attendance for one calendar month
 * (`GET /api/v1/family/students/{id}/attendance?month=YYYY-MM`). The gateway
 * scopes the read to the caller's own children: an unlinked id answers 404.
 */

/** How a day (or one class of it) was recorded. `not_taken` = no list yet. */
export type AttendanceStatus = "present" | "absent" | "late" | "excused" | "partial" | "not_taken";

/** Whether the school takes one list a day or one per class. */
export type AttendanceMode = "daily" | "per_class";

/** One class's record on a per-class day. */
export interface AttendanceClass {
  subjectName: string;
  status: AttendanceStatus;
  note?: string;
}

/** One school day. `classes` is empty on a daily-list day. */
export interface AttendanceDay {
  date: string;
  status: AttendanceStatus;
  mode: AttendanceMode;
  note?: string;
  classes: AttendanceClass[];
}

/** Month totals, counted by the server over the month's days. */
export interface AttendanceSummary {
  present: number;
  absent: number;
  late: number;
  excused: number;
  partial: number;
}

/** One month of a child's attendance, days newest first. */
export interface AttendanceMonth {
  month: string;
  /** The school's today (YYYY-MM-DD), so the app never trusts the phone's clock for it. */
  today: string;
  days: AttendanceDay[];
  summary: AttendanceSummary;
}

type WireDay = Omit<AttendanceDay, "classes"> & { classes?: AttendanceClass[] | null };
interface WireMonth {
  month?: string;
  today?: string;
  days?: WireDay[] | null;
  summary?: Partial<AttendanceSummary> | null;
}

function normalizeSummary(summary: Partial<AttendanceSummary> | null | undefined): AttendanceSummary {
  return {
    present: summary?.present ?? 0,
    absent: summary?.absent ?? 0,
    late: summary?.late ?? 0,
    excused: summary?.excused ?? 0,
    partial: summary?.partial ?? 0,
  };
}

/** Reads one child's attendance for `month` (YYYY-MM). Throws {@link ApiError} on non-2xx. */
export async function getAttendanceMonth(studentId: string, month: string): Promise<AttendanceMonth> {
  const res = await apiFetch(
    `/api/v1/family/students/${encodeURIComponent(studentId)}/attendance?month=${encodeURIComponent(month)}`,
  );
  if (!res.ok) {
    throw new ApiError(res.status, "failed to load attendance");
  }
  const data = (await res.json()) as WireMonth;
  return {
    month: data.month ?? month,
    today: data.today ?? "",
    days: (data.days ?? []).map((d) => ({ ...d, classes: d.classes ?? [] })),
    summary: normalizeSummary(data.summary),
  };
}
