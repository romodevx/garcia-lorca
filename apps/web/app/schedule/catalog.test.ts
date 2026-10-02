import { describe, expect, it } from "vitest";

import { ANA, G1S, G1SA, G3, G3A, G3B, MATE, PRIMARIA, SECUNDARIA, sampleBoard } from "./__fixtures__/board";
import { assignmentsOfGroup, blockLabel, filterBlocks, indexBoard } from "./catalog";

describe("indexBoard", () => {
  const index = indexBoard(sampleBoard());

  it("names a group by its grade and letter", () => {
    expect(index.groupLabel(G3A)).toBe("3.º A");
  });

  it("names subjects and teachers, and says so when one is gone", () => {
    expect(index.subjectName(MATE)).toBe("Matemáticas");
    expect(index.teacherName(ANA)).toBe("Ana Ruiz");
    expect(index.teacherName("nope")).toBe("Docente sin registro");
  });

  it("knows the level and grade a group sits in", () => {
    expect(index.levelOfGroup(G3B)).toBe(PRIMARIA);
    expect(index.gradeOfGroup(G1SA)).toBe(G1S);
  });
});

describe("blockLabel", () => {
  it("reads a class aloud: subject, group, teacher, day and time", () => {
    const board = sampleBoard();
    expect(blockLabel(indexBoard(board), board.blocks[0]!)).toBe("Matemáticas, 3.º A, Ana Ruiz, lunes 08:00 a 08:50");
  });
});

describe("filterBlocks", () => {
  const board = sampleBoard();
  const ids = (f: Parameters<typeof filterBlocks>[1]): string[] => filterBlocks(board, f).map((b) => b.id.slice(-1));

  it.each([
    ["nothing chosen: every class", { levelId: "", gradeId: "", groupId: "", teacherId: "" }, ["1", "2", "3"]],
    ["a group: its week", { levelId: PRIMARIA, gradeId: G3, groupId: G3A, teacherId: "" }, ["1", "2"]],
    ["a grade: all its groups", { levelId: PRIMARIA, gradeId: G3, groupId: "", teacherId: "" }, ["1", "2", "3"]],
    ["another level: none", { levelId: SECUNDARIA, gradeId: "", groupId: "", teacherId: "" }, []],
    ["a teacher: their week", { levelId: "", gradeId: "", groupId: "", teacherId: ANA }, ["1", "3"]],
    ["a teacher in a group", { levelId: PRIMARIA, gradeId: G3, groupId: G3A, teacherId: ANA }, ["1"]],
  ])("%s", (_name, filter, want) => {
    expect(ids(filter)).toEqual(want);
  });
});

describe("assignmentsOfGroup", () => {
  it("offers only the group's own teaching assignments, labelled subject — teacher", () => {
    const board = sampleBoard();
    const got = assignmentsOfGroup(board, indexBoard(board), G3A);
    expect(got.map((o) => o.label)).toEqual(["Español — Luis Pérez", "Matemáticas — Ana Ruiz"]);
  });
});
