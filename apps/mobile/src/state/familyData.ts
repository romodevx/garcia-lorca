import { useMemo } from "react";
import {
  getAccount,
  getAttendance,
  getGrades,
  type FamilyAccount,
  type FamilyAttendanceRecord,
  type FamilyGradeEntry,
} from "../api";
import { useAsyncResource, type AsyncResource } from "./useAsyncResource";

/**
 * Per-child family reads, each an {@link AsyncResource} keyed on the selected
 * child id. A null id (signed out / no linked child) is a first-class no-op: the
 * resource stays empty and fires no request. Each hook loads independently so a
 * screen only pays for the data it shows.
 */

/** The selected child's finance statement. */
export function useChildAccount(childId: string | null): AsyncResource<FamilyAccount> {
  const load = useMemo(() => (childId ? () => getAccount(childId) : null), [childId]);
  return useAsyncResource(load);
}

/** The selected child's stored attendance records. */
export function useChildAttendance(childId: string | null): AsyncResource<FamilyAttendanceRecord[]> {
  const load = useMemo(() => (childId ? () => getAttendance(childId) : null), [childId]);
  return useAsyncResource(load);
}

/** The selected child's numeric grade entries. */
export function useChildGrades(childId: string | null): AsyncResource<FamilyGradeEntry[]> {
  const load = useMemo(() => (childId ? () => getGrades(childId) : null), [childId]);
  return useAsyncResource(load);
}
