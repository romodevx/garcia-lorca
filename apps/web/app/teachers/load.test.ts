import { describe, expect, it } from "vitest";

import type { GradeLevel, LabelledGroup, Level, Subject } from "../config/api";
import type { Teacher, TeachingAssignment } from "./api";
import { currentLoad, groupChoices, isSelectable, subjectNames, type LoadCatalogue } from "./load";

const levels: Level[] = [
  { id: "prim", name: "Primaria", sepCode: "", enrollmentGranularity: "cycle" },
  { id: "sec", name: "Secundaria", sepCode: "", enrollmentGranularity: "cycle" },
];
const grades: GradeLevel[] = [
  { id: "p1", levelId: "prim", name: "1° Primaria", ordinal: 1 },
  { id: "s2", levelId: "sec", name: "2° Secundaria", ordinal: 2 },
  { id: "s1", levelId: "sec", name: "1° Secundaria", ordinal: 1 },
];
const group = (id: string, gradeLevelId: string, name: string, gradeName: string): LabelledGroup => ({
  id,
  cycleId: "c1",
  gradeLevelId,
  name,
  capacity: 30,
  label: `${gradeName} · ${name}`,
});
const groups: LabelledGroup[] = [
  group("s1B", "s1", "B", "1° Secundaria"),
  group("s1A", "s1", "A", "1° Secundaria"),
  group("s2A", "s2", "A", "2° Secundaria"),
  group("p1A", "p1", "A", "1° Primaria"),
];
const subjects: Subject[] = [
  { id: "mat-s1", gradeLevelId: "s1", name: "Matemáticas" },
  { id: "mat-s2", gradeLevelId: "s2", name: "Matemáticas" },
  { id: "fis-s1", gradeLevelId: "s1", name: "Física" },
  { id: "mat-p1", gradeLevelId: "p1", name: "Matemáticas" },
  { id: "esp-p1", gradeLevelId: "p1", name: "Español" },
];
const teachers: Teacher[] = [
  { id: "ada", firstName: "Ada", lastName: "Lovelace", email: "", userId: "" },
  { id: "ana", firstName: "Ana", lastName: "Ruiz", email: "", userId: "" },
];
const ta = (id: string, groupId: string, subjectId: string, teacherId: string): TeachingAssignment => ({
  id,
  cycleId: "c1",
  groupId,
  subjectId,
  teacherId,
});

function catalogue(assignments: TeachingAssignment[]): LoadCatalogue {
  return { levels, grades, groups, subjects, assignments, teachers };
}

describe("currentLoad", () => {
  it("groups one teacher's slots by subject NAME across grades, sorted es-MX, groups in school order", () => {
    const load = currentLoad(
      "ada",
      catalogue([
        ta("a1", "s2A", "mat-s2", "ada"),
        ta("a2", "s1B", "mat-s1", "ada"),
        ta("a3", "s1A", "fis-s1", "ada"),
        ta("a4", "p1A", "mat-p1", "ada"),
        ta("x1", "s1A", "mat-s1", "ana"),
      ]),
    );

    expect(load.map((s) => s.subjectName)).toEqual(["Física", "Matemáticas"]);
    expect(load[1]?.slots.map((s) => s.groupLabel)).toEqual(["1° Primaria · A", "1° Secundaria · B", "2° Secundaria · A"]);
    expect(load[1]?.slots[0]).toEqual({
      assignmentId: "a4",
      groupId: "p1A",
      subjectId: "mat-p1",
      groupLabel: "1° Primaria · A",
      subjectName: "Matemáticas",
    });
  });

  it("is empty for a teacher with no load", () => {
    expect(currentLoad("ada", catalogue([ta("x1", "s1A", "mat-s1", "ana")]))).toEqual([]);
  });
});

describe("subjectNames", () => {
  it("lists each name once, sorted es-MX (accents do not sink to the end)", () => {
    expect(subjectNames(subjects)).toEqual(["Español", "Física", "Matemáticas"]);
  });
});

describe("groupChoices", () => {
  it("resolves, per group, the subject row of THAT group's grade with the chosen name", () => {
    const tree = groupChoices("ada", "Matemáticas", catalogue([]));

    expect(tree.map((l) => l.level.name)).toEqual(["Primaria", "Secundaria"]);
    const sec = tree[1];
    expect(sec?.grades.map((g) => g.grade.name)).toEqual(["1° Secundaria", "2° Secundaria"]);
    expect(sec?.grades[0]?.groups.map((g) => [g.group.id, g.availability])).toEqual([
      ["s1A", { kind: "free", subjectId: "mat-s1" }],
      ["s1B", { kind: "free", subjectId: "mat-s1" }],
    ]);
    expect(sec?.grades[1]?.groups[0]?.availability).toEqual({ kind: "free", subjectId: "mat-s2" });
  });

  it("offers only the grades that carry the subject — a group of another grade is never a choice (T13)", () => {
    const tree = groupChoices("ada", "Física", catalogue([]));

    expect(tree.map((l) => l.level.name)).toEqual(["Secundaria"]);
    expect(tree[0]?.grades.map((g) => g.grade.name)).toEqual(["1° Secundaria"]);
    expect(tree[0]?.grades[0]?.groups.map((g) => [g.group.id, g.availability])).toEqual([
      ["s1A", { kind: "free", subjectId: "fis-s1" }],
      ["s1B", { kind: "free", subjectId: "fis-s1" }],
    ]);
  });

  it("every offered group resolves to the subject row of its OWN grade", () => {
    const tree = groupChoices("ada", "Matemáticas", catalogue([]));
    const bySubject = new Map(subjects.map((s) => [s.id, s.gradeLevelId]));
    for (const level of tree) {
      for (const grade of level.grades) {
        for (const { group, availability } of grade.groups) {
          if (isSelectable(availability)) expect(bySubject.get(availability.subjectId)).toBe(group.gradeLevelId);
        }
      }
    }
  });

  it("tells mine from held by another teacher", () => {
    const tree = groupChoices(
      "ada",
      "Matemáticas",
      catalogue([ta("a1", "s1A", "mat-s1", "ada"), ta("x1", "s1B", "mat-s1", "ana")]),
    );
    const [s1A, s1B] = tree[1]?.grades[0]?.groups ?? [];

    expect(s1A?.availability).toEqual({ kind: "mine", subjectId: "mat-s1" });
    expect(s1B?.availability).toEqual({ kind: "held", subjectId: "mat-s1", teacherName: "Ana Ruiz" });
  });

  it("only free and held groups are selectable", () => {
    expect(isSelectable({ kind: "free", subjectId: "x" })).toBe(true);
    expect(isSelectable({ kind: "held", subjectId: "x", teacherName: "Ana" })).toBe(true);
    expect(isSelectable({ kind: "mine", subjectId: "x" })).toBe(false);
  });
});
