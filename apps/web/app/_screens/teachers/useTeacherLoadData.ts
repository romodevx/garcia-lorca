"use client";

import { useCallback, useEffect, useMemo, useState } from "react";

import { API_URL } from "../../api-url";
import {
  listCycles,
  listGradeLevels,
  listLabelledGroups,
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
  groups: LabelledGroup[];
  subjects: Subject[];
  assignments: TeachingAssignment[];
}

const NO_CYCLE: OfCycle = { groups: [], subjects: [], assignments: [] };

/** Subjects live per grade level: read those of every grade that has a group. */
async function subjectsOf(groups: readonly LabelledGroup[]): Promise<Subject[]> {
  const gradeIds = [...new Set(groups.map((g) => g.gradeLevelId))];
  const perGrade = await Promise.all(gradeIds.map((id) => listSubjects(API_URL, id)));
  return perGrade.flat();
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
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let active = true;
    Promise.all([listTeachers(API_URL), listCycles(API_URL), listLevels(API_URL), listGradeLevels(API_URL)])
      .then(([teachers, cycles, levels, grades]) => {
        if (!active) return;
        setShared({ teachers, cycles, levels, grades });
        setCycleId((cycles.find((c) => c.status === "active") ?? cycles[0])?.id ?? "");
      })
      .catch(() => active && setFailed(true));
    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    if (cycleId === "") return;
    let active = true;
    setOfCycle(undefined);
    Promise.all([listLabelledGroups(API_URL, cycleId), listTeachingAssignments(API_URL, { cycleId })])
      .then(async ([groups, assignments]) => {
        const subjects = await subjectsOf(groups);
        if (active) setOfCycle({ groups, subjects, assignments });
      })
      .catch(() => active && setFailed(true));
    return () => {
      active = false;
    };
  }, [cycleId]);

  const reloadAssignments = useCallback(async (): Promise<void> => {
    const assignments = await listTeachingAssignments(API_URL, { cycleId });
    setOfCycle((prev) => (prev === undefined ? prev : { ...prev, assignments }));
  }, [cycleId]);

  const dropAssignment = useCallback((assignmentId: string): void => {
    setOfCycle((prev) =>
      prev === undefined ? prev : { ...prev, assignments: prev.assignments.filter((a) => a.id !== assignmentId) },
    );
  }, []);

  const cycleLoaded = shared !== undefined && (cycleId === "" || ofCycle !== undefined);
  const status: LoadStatus = failed ? "error" : cycleLoaded ? "ready" : "loading";
  const catalogue = useMemo<LoadCatalogue>(
    () => ({
      levels: shared?.levels ?? [],
      grades: shared?.grades ?? [],
      teachers: shared?.teachers ?? [],
      ...(ofCycle ?? NO_CYCLE),
    }),
    [shared, ofCycle],
  );

  return {
    status,
    teacher: shared?.teachers.find((t) => t.id === teacherId),
    cycles: shared?.cycles ?? [],
    cycleId,
    selectCycle: setCycleId,
    catalogue,
    reloadAssignments,
    dropAssignment,
  };
}
