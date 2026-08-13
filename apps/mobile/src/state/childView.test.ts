import type { FamilyStudent } from "../api";
import { isQualitativeLevel, toChildView } from "./childView";

const student: FamilyStudent = {
  id: "s1",
  firstName: "Mateo",
  lastName: "Herrera Díaz",
  level: "primaria",
  gradeGroup: "3° A",
};

describe("toChildView", () => {
  it("derives display fields from a real family student", () => {
    const view = toChildView(student);
    expect(view).toMatchObject({
      present: true,
      fullName: "Mateo Herrera Díaz",
      short: "Mateo",
      initials: "MH",
      group: "3° A",
      levelLabel: "Primaria",
      qualitative: false,
    });
  });

  it("marks a kínder child as qualitative", () => {
    const view = toChildView({ ...student, level: "kinder" });
    expect(view.levelLabel).toBe("Kínder");
    expect(view.qualitative).toBe(true);
  });

  it("returns a safe, absent placeholder for a null child", () => {
    const view = toChildView(null);
    expect(view.present).toBe(false);
    expect(view.fullName).toBe("");
    expect(view.initials).toBe("");
  });
});

describe("isQualitativeLevel", () => {
  it("is true only for kínder", () => {
    expect(isQualitativeLevel("kinder")).toBe(true);
    expect(isQualitativeLevel("primaria")).toBe(false);
    expect(isQualitativeLevel("")).toBe(false);
  });
});
