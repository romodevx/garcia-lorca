import type { FamilyStudent } from "../api";
import { isQualitativeLevel, toChildView } from "./childView";

const student: FamilyStudent = {
  id: "s1",
  firstName: "Mateo",
  lastName: "Herrera Díaz",
  level: "Primaria",
  levelId: "lvl-pri",
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
    const view = toChildView({ ...student, level: "Kínder" });
    expect(view.levelLabel).toBe("Kínder");
    expect(view.qualitative).toBe(true);
  });

  // The level is the school's own name for it now (task 2): whatever the
  // school called it in Configuración is what the family reads.
  it("shows any level the school defined, by the school's own name", () => {
    const view = toChildView({ ...student, level: "Maternal" });
    expect(view.levelLabel).toBe("Maternal");
  });

  it("returns a safe, absent placeholder for a null child", () => {
    const view = toChildView(null);
    expect(view.present).toBe(false);
    expect(view.fullName).toBe("");
    expect(view.initials).toBe("");
  });
});

describe("isQualitativeLevel", () => {
  it("is true for kínder however the school spells it, and for preescolar", () => {
    // Preescolar is the official name of kínder: a school that calls its level
    // "Preescolar" must keep the qualitative grades it had as "Kínder".
    for (const name of ["kinder", "Kínder", "KINDER", "Preescolar", " preescolar "]) {
      expect(isQualitativeLevel(name)).toBe(true);
    }
    expect(isQualitativeLevel("Primaria")).toBe(false);
    expect(isQualitativeLevel("Maternal")).toBe(false);
    expect(isQualitativeLevel("")).toBe(false);
  });
});
