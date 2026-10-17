import { describe, expect, it } from "vitest";

import { studentRoomLabel } from "./studentRoom";

describe("studentRoomLabel (CC-211, CC-313)", () => {
  it("is the classroom the gateway named, grade included", () => {
    expect(studentRoomLabel({ level: "PRIMARIA", gradeGroup: "B", groupLabel: "3° PRIMARIA · B" })).toBe("3° PRIMARIA · B");
  });

  it("falls back to the level and what the file stores when the list has no label", () => {
    expect(studentRoomLabel({ level: "PRIMARIA", gradeGroup: "B" })).toBe("PRIMARIA · B");
    expect(studentRoomLabel({ level: "PRIMARIA", gradeGroup: "B", groupLabel: "" })).toBe("PRIMARIA · B");
  });

  it("drops an empty part instead of leaving a dangling separator", () => {
    expect(studentRoomLabel({ level: "PRIMARIA", gradeGroup: "" })).toBe("PRIMARIA");
    expect(studentRoomLabel({ level: "", gradeGroup: "" })).toBe("");
  });
});
