import { useMemo } from "react";
import { getCard, getStatement, type FamilyCard, type FamilyStatement } from "../api";
import { getAttendanceMonth, type AttendanceMonth } from "../api/attendance";
import { getPeriodGrades, type PeriodGrades } from "../api/grades";
import { getReportCard, listReportCards, type ReportCard, type ReportCardSummary } from "../api/reportCards";
import { useFreshDataSignal } from "./freshness";
import { useAsyncResource, type AsyncResource } from "./useAsyncResource";

/**
 * Per-child family reads, each an {@link AsyncResource} keyed on the selected
 * child id. A null id (signed out / no linked child) is a first-class no-op: the
 * resource stays empty and fires no request. Each hook loads independently so a
 * screen only pays for the data it shows.
 */

/**
 * In-flight requests for one (child, month), shared: Inicio and Asistencia read
 * the same month, and the freshness signal reloads both at once — one request
 * serves them. Only IN-FLIGHT calls are shared; a settled month is never served
 * from memory, so a reload always reaches the server.
 */
const attendanceInFlight = new Map<string, Promise<AttendanceMonth>>();

function sharedAttendanceMonth(childId: string, month: string): Promise<AttendanceMonth> {
  const key = `${childId}|${month}`;
  const pending = attendanceInFlight.get(key);
  if (pending) {
    return pending;
  }
  const request = getAttendanceMonth(childId, month).finally(() => {
    attendanceInFlight.delete(key);
  });
  attendanceInFlight.set(key, request);
  return request;
}

/** The selected child's account statement: finance's ledger (T7). */
export function useChildStatement(childId: string | null): AsyncResource<FamilyStatement> {
  const load = useMemo(() => (childId ? () => getStatement(childId) : null), [childId]);
  return useAsyncResource(load);
}

/**
 * One month (YYYY-MM) of the selected child's attendance. A null month is a
 * no-op, like a null child. It reloads on the freshness signal (app back in the
 * foreground, an attendance push) — a teacher can change it at any moment.
 */
export function useChildAttendanceMonth(
  childId: string | null,
  month: string | null,
): AsyncResource<AttendanceMonth> {
  const load = useMemo(
    () => (childId && month ? () => sharedAttendanceMonth(childId, month) : null),
    [childId, month],
  );
  const resource = useAsyncResource(load);
  useFreshDataSignal(resource.reload, load !== null);
  return resource;
}

/**
 * The selected child's grades per evaluation period for the cycle. Reloads on
 * the freshness signal: a teacher's capture should show up when the parent
 * reopens the app.
 */
export function useChildPeriodGrades(childId: string | null): AsyncResource<PeriodGrades> {
  const load = useMemo(() => (childId ? () => getPeriodGrades(childId) : null), [childId]);
  const resource = useAsyncResource(load);
  useFreshDataSignal(resource.reload, load !== null);
  return resource;
}

/**
 * The selected child's issued boletas. Reloads on the freshness signal: a new
 * boleta (or a corrected version) arrives with a push or on reopening the app.
 */
export function useChildReportCards(childId: string | null): AsyncResource<ReportCardSummary[]> {
  const load = useMemo(() => (childId ? () => listReportCards(childId) : null), [childId]);
  const resource = useAsyncResource(load);
  useFreshDataSignal(resource.reload, load !== null);
  return resource;
}

/** One boleta of the selected child. A null id is a no-op. */
export function useChildReportCard(childId: string | null, reportCardId: string | null): AsyncResource<ReportCard> {
  const load = useMemo(
    () => (childId && reportCardId ? () => getReportCard(childId, reportCardId) : null),
    [childId, reportCardId],
  );
  return useAsyncResource(load);
}

/** The selected child's virtual card (task 13). */
export function useChildCard(childId: string | null): AsyncResource<FamilyCard> {
  const load = useMemo(() => (childId ? () => getCard(childId) : null), [childId]);
  return useAsyncResource(load);
}
