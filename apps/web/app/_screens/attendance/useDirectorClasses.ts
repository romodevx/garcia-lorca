import { useEffect, useMemo, useState } from "react";

import { API_URL } from "../../api-url";
import type { AttendanceMode } from "../../attendance/sheet";
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
import { sortBySchoolOrder } from "../../config/schoolOrder";
import { listTeachingAssignments } from "../../teachers/api";

export interface DirectorClasses {
  cycles: readonly Cycle[];
  /** In school order: level → grade → letter. */
  groups: readonly LabelledGroup[];
  cycleId: string;
  setCycleId: (id: string) => void;
  groupId: string;
  setGroupId: (id: string) => void;
  group: LabelledGroup | undefined;
  /** The chosen group's level mode; "daily" when the level does not say. */
  mode: AttendanceMode;
  /** Subjects taught in the chosen group (per-class levels only), by name. */
  subjects: readonly Subject[];
  subjectsLoading: boolean;
  subjectId: string;
  setSubjectId: (id: string) => void;
  loadError?: string;
}

/**
 * What the director picks before a list: cycle → group → (per-class level)
 * subject. The group's mode comes from its level, reached through its grade;
 * the subjects offered are the ones with a teaching assignment in that group.
 */
export function useDirectorClasses(): DirectorClasses {
  const [cycles, setCycles] = useState<Cycle[]>([]);
  const [levels, setLevels] = useState<Level[]>([]);
  const [grades, setGrades] = useState<GradeLevel[]>([]);
  const [groups, setGroups] = useState<LabelledGroup[]>([]);
  const [subjects, setSubjects] = useState<Subject[]>([]);
  const [subjectsLoading, setSubjectsLoading] = useState(false);
  const [cycleId, setCycleIdState] = useState("");
  const [groupId, setGroupIdState] = useState("");
  const [subjectId, setSubjectId] = useState("");
  const [loadError, setLoadError] = useState<string | undefined>(undefined);

  useEffect(() => {
    let active = true;
    Promise.all([listCycles(API_URL), listLevels(API_URL), listGradeLevels(API_URL)])
      .then(([cs, ls, gs]) => {
        if (!active) return;
        setCycles(cs);
        setLevels(ls);
        setGrades(gs);
        setCycleIdState((cs.find((c) => c.status === "active") ?? cs[0])?.id ?? "");
      })
      .catch(() => active && setLoadError("No se pudo cargar la configuración escolar."));
    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    if (cycleId === "") return;
    let active = true;
    listLabelledGroups(API_URL, cycleId)
      .then((gs) => active && setGroups(gs))
      .catch(() => active && setGroups([]));
    return () => {
      active = false;
    };
  }, [cycleId]);

  const orderedGroups = useMemo(() => sortBySchoolOrder(groups, { levels, grades }), [groups, levels, grades]);
  const group = groups.find((g) => g.id === groupId);
  const mode = useMemo<AttendanceMode>(() => {
    const levelId = grades.find((g) => g.id === group?.gradeLevelId)?.levelId;
    return levels.find((l) => l.id === levelId)?.attendanceMode ?? "daily";
  }, [grades, group, levels]);

  useEffect(() => {
    setSubjects([]);
    if (group === undefined || mode !== "per_class") return;
    let active = true;
    setSubjectsLoading(true);
    Promise.all([listTeachingAssignments(API_URL, { groupId: group.id }), listSubjects(API_URL, group.gradeLevelId)])
      .then(([assignments, all]) => {
        if (!active) return;
        const taught = new Set(assignments.map((a) => a.subjectId));
        const offered = all.filter((s) => taught.has(s.id)).sort((a, b) => a.name.localeCompare(b.name, "es"));
        setSubjects(offered);
        if (offered.length === 1) setSubjectId(offered[0]?.id ?? "");
      })
      .catch(() => active && setSubjects([]))
      .finally(() => active && setSubjectsLoading(false));
    return () => {
      active = false;
    };
  }, [group, mode]);

  const setCycleId = (id: string): void => {
    setCycleIdState(id);
    setGroupIdState("");
    setSubjectId("");
  };
  const setGroupId = (id: string): void => {
    setGroupIdState(id);
    setSubjectId("");
  };

  return { cycles, groups: orderedGroups, cycleId, setCycleId, groupId, setGroupId, group, mode, subjects, subjectsLoading, subjectId, setSubjectId, loadError };
}
