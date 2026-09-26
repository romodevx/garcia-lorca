"use client";

import { useCallback, useEffect, useMemo, useState } from "react";

import { API_URL } from "../../api-url";
import {
  labelGroups,
  listCycles,
  listGradeLevels,
  listGroups,
  listLevels,
  listSubjects,
  type Cycle,
  type GradeLevel,
  type LabelledGroup,
  type Level,
  type Subject,
} from "../../config/api";
import { listTeachers, listTeachingAssignments, type Teacher, type TeachingAssignment } from "../../teachers/api";
import type { LoadCatalogue } from "../../teachers/load";

export type LoadStatus = "loading" | "error" | "ready";

export interface TeacherLoadData {
  status: LoadStatus;
  teacher: Teacher | undefined;
  cycles: readonly Cycle[];
  cycleId: string;
  selectCycle: (cycleId: string) => void;
  /** Reads again whatever failed: the roster and catalogue, or the cycle. */
  retry: () => void;
  catalogue: LoadCatalogue;
  /** Re-reads the cycle's assignments after a write. */
  reloadAssignments: () => Promise<void>;
  /** Takes a freed slot off the screen without waiting for a re-read. */
  dropAssignment: (assignmentId: string) => void;
}

interface Shared {
  teachers: Teacher[];
  cycles: Cycle[];
  levels: Level[];
  grades: GradeLevel[];
}

interface OfCycle {
  /** The cycle these rows belong to, so a late answer for another is dropped. */
  cycleId: string;
  groups: LabelledGroup[];
  subjects: Subject[];
  assignments: TeachingAssignment[];
}

const NO_CYCLE: Omit<OfCycle, "cycleId"> = { groups: [], subjects: [], assignments: [] };

/**
 * Subjects live per grade level: ONE read of the whole catalogue, kept to the
 * grades that have a group in the cycle.
 */
async function subjectsOf(groups: readonly LabelledGroup[]): Promise<Subject[]> {
  const gradeIds = new Set(groups.map((g) => g.gradeLevelId));
  const all = await listSubjects(API_URL, "");
  return all.filter((s) => gradeIds.has(s.gradeLevelId));
}

/**
 * Everything the teaching-load page reads: the roster and the catalogue once,
 * then the chosen cycle's groups, subjects and ALL its assignments — who else
 * holds a slot is what makes "La imparte …" possible.
 */
export function useTeacherLoadData(teacherId: string): TeacherLoadData {
  const [shared, setShared] = useState<Shared | undefined>(undefined);
  const [cycleId, setCycleId] = useState("");
  const [ofCycle, setOfCycle] = useState<OfCycle | undefined>(undefined);
  const [sharedFailed, setSharedFailed] = useState(false);
  const [cycleFailed, setCycleFailed] = useState(false);
  const [sharedAttempt, setSharedAttempt] = useState(0);
  const [cycleAttempt, setCycleAttempt] = useState(0);

  useEffect(() => {
    let active = true;
    setSharedFailed(false);
    Promise.all([listTeachers(API_URL), listCycles(API_URL), listLevels(API_URL), listGradeLevels(API_URL)])
      .then(([teachers, cycles, levels, grades]) => {
        if (!active) return;
        setShared({ teachers, cycles, levels, grades });
        setCycleId((cycles.find((c) => c.status === "active") ?? cycles[0])?.id ?? "");
      })
      .catch(() => active && setSharedFailed(true));
    return () => {
      active = false;
    };
  }, [sharedAttempt]);

  const grades = shared?.grades;
  useEffect(() => {
    if (cycleId === "" || grades === undefined) return;
    let active = true;
    setOfCycle(undefined);
    setCycleFailed(false);
    Promise.all([listGroups(API_URL, cycleId), listTeachingAssignments(API_URL, { cycleId })])
      .then(async ([rawGroups, assignments]) => {
        const groups = labelGroups(rawGroups, grades);
        const subjects = await subjectsOf(groups);
        if (active) setOfCycle({ cycleId, groups, subjects, assignments });
      })
      .catch(() => active && setCycleFailed(true));
    return () => {
      active = false;
    };
  }, [cycleId, grades, cycleAttempt]);

  const retry = useCallback((): void => {
    if (shared === undefined) setSharedAttempt((n) => n + 1);
    else setCycleAttempt((n) => n + 1);
  }, [shared]);

  const reloadAssignments = useCallback(async (): Promise<void> => {
    const requested = cycleId;
    const assignments = await listTeachingAssignments(API_URL, { cycleId: requested });
    setOfCycle((prev) => (prev === undefined || prev.cycleId !== requested ? prev : { ...prev, assignments }));
  }, [cycleId]);

  const dropAssignment = useCallback((assignmentId: string): void => {
    setOfCycle((prev) =>
      prev === undefined ? prev : { ...prev, assignments: prev.assignments.filter((a) => a.id !== assignmentId) },
    );
  }, []);

  const cycleLoaded = shared !== undefined && (cycleId === "" || ofCycle !== undefined);
  const status: LoadStatus = sharedFailed || cycleFailed ? "error" : cycleLoaded ? "ready" : "loading";
  const catalogue = useMemo<LoadCatalogue>(
    () => {
      const rows = ofCycle ?? NO_CYCLE;
      return {
        levels: shared?.levels ?? [],
        grades: shared?.grades ?? [],
        teachers: shared?.teachers ?? [],
        groups: rows.groups,
        subjects: rows.subjects,
        assignments: rows.assignments,
      };
    },
    [shared, ofCycle],
  );

  return {
    status,
    teacher: shared?.teachers.find((t) => t.id === teacherId),
    cycles: shared?.cycles ?? [],
    cycleId,
    selectCycle: setCycleId,
    retry,
    catalogue,
    reloadAssignments,
    dropAssignment,
  };
}
