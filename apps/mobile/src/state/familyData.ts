import { useMemo } from "react";
import {
  getAccount,
  getCard,
  getGrades,
  type FamilyAccount,
  type FamilyCard,
  type FamilyGradeEntry,
} from "../api";
import { getAttendanceMonth, type AttendanceMonth } from "../api/attendance";
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

/** The selected child's finance statement. */
export function useChildAccount(childId: string | null): AsyncResource<FamilyAccount> {
  const load = useMemo(() => (childId ? () => getAccount(childId) : null), [childId]);
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

/** The selected child's numeric grade entries. */
export function useChildGrades(childId: string | null): AsyncResource<FamilyGradeEntry[]> {
  const load = useMemo(() => (childId ? () => getGrades(childId) : null), [childId]);
  return useAsyncResource(load);
}

/** The selected child's virtual card (task 13). */
export function useChildCard(childId: string | null): AsyncResource<FamilyCard> {
  const load = useMemo(() => (childId ? () => getCard(childId) : null), [childId]);
  return useAsyncResource(load);
}
