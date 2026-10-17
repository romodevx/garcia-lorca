"use client";

import { useEffect, useState } from "react";

import { API_URL } from "../../api-url";
import { listCycles, listLabelledGroups, type LabelledGroup } from "../../config/api";
import { listEnrollments, type Enrollment } from "../../students/api";

/**
 * Each pupil's classroom as every screen names it — "3° PRIMARIA · A", grade
 * included (ISSUE-T14-018) — from the active enrollment of the cycle. A pupil
 * with no active enrollment, or in a group the list does not have, is absent:
 * the caller then shows what the pupil's file stores.
 */
export function roomLabelsOf(enrollments: readonly Enrollment[], groups: readonly LabelledGroup[]): ReadonlyMap<string, string> {
  const labelOf = new Map(groups.map((g) => [g.id, g.label]));
  const out = new Map<string, string>();
  for (const e of enrollments) {
    if (e.status !== "active" || out.has(e.studentId)) continue;
    const label = labelOf.get(e.groupId);
    if (label !== undefined) out.set(e.studentId, label);
  }
  return out;
}

const NONE: ReadonlyMap<string, string> = new Map();

/** The active cycle's id, or undefined when the school has none. */
async function activeCycleId(): Promise<string | undefined> {
  const cycles = await listCycles(API_URL);
  return (cycles.find((c) => c.status === "active") ?? cycles[0])?.id;
}

async function loadRoomLabels(): Promise<ReadonlyMap<string, string>> {
  const cycleId = await activeCycleId();
  if (cycleId === undefined) return NONE;
  const [groups, enrollments] = await Promise.all([listLabelledGroups(API_URL, cycleId), listEnrollments(API_URL, { cycleId })]);
  return roomLabelsOf(enrollments, groups);
}

/**
 * The active cycle's classroom labels, by pupil id. Empty until read, and empty
 * for good when a read fails: a courtesy for the eye, never a gate.
 */
export function useRoomLabels(): ReadonlyMap<string, string> {
  const [labels, setLabels] = useState<ReadonlyMap<string, string>>(NONE);
  useEffect(() => {
    let live = true;
    loadRoomLabels()
      .then((got) => { if (live) setLabels(got); })
      .catch(() => {});
    return () => { live = false; };
  }, []);
  return labels;
}
