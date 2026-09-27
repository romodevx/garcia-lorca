import { useEffect, useState } from "react";

import { API_URL } from "../../api-url";
import { listGradeLevels, listLabelledGroups, listLevels, listSubjects, type LabelledGroup, type Subject } from "../../config/api";
import { schoolOrder, type SchoolCatalogue } from "../../config/schoolOrder";
import { listTeachers, listTeachingAssignments, type Teacher, type TeachingAssignment } from "../../teachers/api";

export interface AssignmentChoice {
  id: string;
  /** "1° PRIMARIA · A · Matemáticas · Ada Lovelace". */
  label: string;
}

export interface DirectorAssignments {
  choices: AssignmentChoice[];
  loading: boolean;
  failed: boolean;
}

const LOCALE = "es-MX";
const distinct = (xs: readonly string[]): string[] => Array.from(new Set(xs));

interface Catalogue extends SchoolCatalogue {
  groups: readonly LabelledGroup[];
  subjects: readonly Subject[];
  teachers: readonly Teacher[];
}

/** Every assignment, readable and in the order a school reads its groups: level → grade → letter → subject. */
export function assignmentChoices(assignments: readonly TeachingAssignment[], cat: Catalogue): AssignmentChoice[] {
  const groupById = new Map(cat.groups.map((g) => [g.id, g]));
  const subjectName = (id: string): string => cat.subjects.find((s) => s.id === id)?.name ?? id;
  const teacherName = (id: string): string => {
    const t = cat.teachers.find((x) => x.id === id);
    return t !== undefined ? `${t.firstName} ${t.lastName}` : id;
  };
  const byGroup = schoolOrder(cat);
  const missing = (id: string): LabelledGroup => ({ id, cycleId: "", gradeLevelId: "", name: id, capacity: 0, label: id });
  const rows = assignments.map((a) => ({ a, group: groupById.get(a.groupId) ?? missing(a.groupId), subject: subjectName(a.subjectId) }));
  rows.sort((x, y) => byGroup(x.group, y.group) || x.subject.localeCompare(y.subject, LOCALE));
  return rows.map(({ a, group, subject }) => ({ id: a.id, label: `${group.label} · ${subject} · ${teacherName(a.teacherId)}` }));
}

async function loadChoices(): Promise<AssignmentChoice[]> {
  const [assignments, teachers, levels, grades] = await Promise.all([
    listTeachingAssignments(API_URL),
    listTeachers(API_URL),
    listLevels(API_URL),
    listGradeLevels(API_URL),
  ]);
  const groups = (await Promise.all(distinct(assignments.map((a) => a.cycleId)).map((id) => listLabelledGroups(API_URL, id)))).flat();
  const subjects = (await Promise.all(distinct(groups.map((g) => g.gradeLevelId)).map((id) => listSubjects(API_URL, id)))).flat();
  return assignmentChoices(assignments, { groups, subjects, teachers, levels, grades });
}

/** The director's pick list: any group's assignments, school-wide. */
export function useDirectorAssignments(): DirectorAssignments {
  const [state, setState] = useState<DirectorAssignments>({ choices: [], loading: true, failed: false });

  useEffect(() => {
    let active = true;
    loadChoices()
      .then((choices) => active && setState({ choices, loading: false, failed: false }))
      .catch(() => active && setState({ choices: [], loading: false, failed: true }));
    return () => {
      active = false;
    };
  }, []);

  return state;
}
