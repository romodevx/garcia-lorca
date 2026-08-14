import { describe, expect, it } from "vitest";
import { MODULES, isModuleVisible, type ModuleDef } from "./data";

const gradesModule = (): ModuleDef => {
  const m = MODULES.find((x) => x.id === "grades");
  if (m === undefined) throw new Error("grades module missing");
  return m;
};

describe("isModuleVisible — any-of perms", () => {
  it("shows a module when the principal holds ANY of its perms", () => {
    const m: ModuleDef = { ...gradesModule(), perm: ["grades:read", "teaching:read"] };
    expect(isModuleVisible(m, ["teaching:read"])).toBe(true);
    expect(isModuleVisible(m, ["grades:read"])).toBe(true);
    expect(isModuleVisible(m, ["grades:read", "attendance:read"])).toBe(true);
  });

  it("hides a module when the principal holds NONE of its perms", () => {
    const m: ModuleDef = { ...gradesModule(), perm: ["grades:read", "teaching:read"] };
    expect(isModuleVisible(m, ["students:read"])).toBe(false);
    expect(isModuleVisible(m, [])).toBe(false);
  });

  it("still supports a single-string perm", () => {
    const m: ModuleDef = { ...gradesModule(), perm: "students:read" };
    expect(isModuleVisible(m, ["students:read"])).toBe(true);
    expect(isModuleVisible(m, ["teaching:read"])).toBe(false);
  });

  it("never shows a deactivated (hidden) module even if the perm matches", () => {
    const m: ModuleDef = { ...gradesModule(), perm: "grades:read", hidden: true };
    expect(isModuleVisible(m, ["grades:read"])).toBe(false);
  });
});

describe("Calificaciones module", () => {
  it("is visible to a teaching:read-only principal (no grades:read)", () => {
    expect(isModuleVisible(gradesModule(), ["teaching:read"])).toBe(true);
  });

  it("is still visible to a grades:read principal", () => {
    expect(isModuleVisible(gradesModule(), ["grades:read"])).toBe(true);
  });
});
