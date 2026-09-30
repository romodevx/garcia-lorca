import { describe, expect, it } from "vitest";

import { placeStudents } from "./placement";

const grades = [
  { id: "pre-1", levelId: "pre" },
  { id: "pri-1", levelId: "pri" },
];
const groups = [
  { id: "pre-1-b", gradeLevelId: "pre-1" },
  { id: "pri-1-a", gradeLevelId: "pri-1" },
];

describe("placeStudents", () => {
  it("places a pupil in the grade and group of their active enrolment", () => {
    const placed = placeStudents(
      [{ id: "s1", levelId: "pre" }],
      { enrollments: [{ studentId: "s1", groupId: "pre-1-b", status: "active" }], groups, grades },
    );
    expect(placed.get("s1")).toEqual({ levelId: "pre", gradeLevelId: "pre-1", groupId: "pre-1-b" });
  });

  it("ignores enrolments that are no longer active", () => {
    const placed = placeStudents(
      [{ id: "s1", levelId: "pre" }],
      { enrollments: [{ studentId: "s1", groupId: "pre-1-b", status: "withdrawn" }], groups, grades },
    );
    expect(placed.get("s1")).toEqual({ levelId: "pre", gradeLevelId: "", groupId: "" });
  });

  it("keeps the file's level for a pupil with no group this cycle", () => {
    const placed = placeStudents([{ id: "s1", levelId: "pri" }], { enrollments: [], groups, grades });
    expect(placed.get("s1")).toEqual({ levelId: "pri", gradeLevelId: "", groupId: "" });
  });

  it("takes the level from the group when the file has none or disagrees", () => {
    const placed = placeStudents(
      [{ id: "s1", levelId: "" }, { id: "s2", levelId: "pre" }],
      {
        enrollments: [
          { studentId: "s1", groupId: "pre-1-b", status: "active" },
          { studentId: "s2", groupId: "pri-1-a", status: "active" },
        ],
        groups,
        grades,
      },
    );
    expect(placed.get("s1")?.levelId).toBe("pre");
    expect(placed.get("s2")).toEqual({ levelId: "pri", gradeLevelId: "pri-1", groupId: "pri-1-a" });
  });

  it("treats a group this cycle does not know as no group", () => {
    const placed = placeStudents(
      [{ id: "s1", levelId: "pre" }],
      { enrollments: [{ studentId: "s1", groupId: "gone", status: "active" }], groups, grades },
    );
    expect(placed.get("s1")).toEqual({ levelId: "pre", gradeLevelId: "", groupId: "" });
  });
});
