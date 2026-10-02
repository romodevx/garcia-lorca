"use client";

import { useEffect, useMemo, useState } from "react";

import { API_URL } from "../../api-url";
import { listCycles, listGradeLevels, listLabelledGroups, listLevels, type GradeLevel, type LabelledGroup, type Level } from "../../config/api";
import { listEnrollments, listStudents, type Enrollment, type Student } from "../../students/api";
import { placeStudents } from "./placement";
import type { FilterCatalog, FilterableStudent } from "./studentFilter";

/** A pupil placed in the active cycle, ready for the T4 filters. */
export interface RosterStudent extends FilterableStudent {
  /** "1° PRIMARIA · A", or "" without a group this cycle. */
  room: string;
}

export interface StudentRoster {
  status: "loading" | "ok" | "failed";
  students: RosterStudent[];
  catalog: FilterCatalog;
}

/** Who can still be picked: a withdrawn or rejected pupil cannot. */
const OUT = new Set(["withdrawn", "rejected"]);

interface Sources {
  students: Student[];
  levels: Level[];
  grades: GradeLevel[];
  groups: LabelledGroup[];
  enrollments: Enrollment[];
}

async function readSources(): Promise<Sources> {
  const [students, levels, grades, cycles] = await Promise.all([
    listStudents(API_URL),
    listLevels(API_URL).catch((): Level[] => []),
    listGradeLevels(API_URL).catch((): GradeLevel[] => []),
    listCycles(API_URL).catch(() => []),
  ]);
  const cycle = cycles.find((c) => c.status === "active") ?? cycles[0];
  if (cycle === undefined) return { students, levels, grades, groups: [], enrollments: [] };
  // Without the cycle's groups or enrolments the pupils keep their level only.
  const [groups, enrollments] = await Promise.all([
    listLabelledGroups(API_URL, cycle.id).catch((): LabelledGroup[] => []),
    listEnrollments(API_URL, { cycleId: cycle.id }).catch((): Enrollment[] => []),
  ]);
  return { students, levels, grades, groups, enrollments };
}

/**
 * The pupils a picker offers (activities T17, scholarships T18), each placed
 * in the active cycle's level → grade → group the same way the students list
 * places them, plus the catalog the chips are drawn from.
 */
export function useStudentRoster(): StudentRoster {
  const [sources, setSources] = useState<Sources | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let live = true;
    readSources()
      .then((s) => live && setSources(s))
      .catch(() => live && setFailed(true));
    return () => {
      live = false;
    };
  }, []);

  return useMemo((): StudentRoster => {
    if (failed) return { status: "failed", students: [], catalog: { levels: [], grades: [], groups: [] } };
    if (sources === null) return { status: "loading", students: [], catalog: { levels: [], grades: [], groups: [] } };
    const pickable = sources.students.filter((s) => !OUT.has(s.admissionStatus));
    const placed = placeStudents(pickable, { enrollments: sources.enrollments, groups: sources.groups, grades: sources.grades });
    const roomOf = new Map(sources.groups.map((g) => [g.id, g.label]));
    const students = pickable.map((s): RosterStudent => {
      const p = placed.get(s.id) ?? { levelId: s.levelId, gradeLevelId: "", groupId: "" };
      return { id: s.id, firstName: s.firstName, lastName: s.lastName, maternalLastName: s.maternalLastName, ...p, room: roomOf.get(p.groupId) ?? "", searchText: [s.curp] };
    });
    return { status: "ok", students, catalog: { levels: sources.levels, grades: sources.grades, groups: sources.groups } };
  }, [sources, failed]);
}
