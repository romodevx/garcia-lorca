import { describe, expect, it } from "vitest";

import { MODULES, isModuleVisible, type ModuleDef } from "../_design/data";
import { HOME_AREAS, groupHomeModules } from "./areas";

const OWNER = ["students:read", "finance:read", "finance:write", "attendance:read", "grades:read", "comms:read", "config:read", "users:read", "teachers:read", "expenses:read", "schedule:read", "activities:read", "scholarships:read", "uniforms:stock"];
const visibleFor = (perms: readonly string[]): ModuleDef[] => MODULES.filter((m) => isModuleVisible(m, perms));
const ids = (mods: readonly ModuleDef[]): string[] => mods.map((m) => m.id);

describe("home areas (PLAN-redesign-home §3.1)", () => {
  it("are, in order: people, academic, money, system", () => {
    expect(HOME_AREAS.map((a) => a.id)).toEqual(["people", "academic", "money", "system"]);
    expect(HOME_AREAS.map((a) => a.color)).toEqual(["forest", "purple", "orange", "neutral"]);
  });

  it("orders each area's modules by daily use", () => {
    const grouped = groupHomeModules(visibleFor(OWNER));
    expect(grouped.map((g) => [g.area.id, ids(g.modules)])).toEqual([
      ["people", ["students", "announcements", "activities"]],
      ["academic", ["attendance", "grades", "schedule", "teachers"]],
      ["money", ["payments", "finance", "scholarships", "uniforms", "expenses"]],
      ["system", ["settings", "admin"]],
    ]);
  });

  it("leaves out an area with no visible module", () => {
    const grouped = groupHomeModules(visibleFor(["attendance:read", "grades:read", "config:read"]));
    expect(grouped.map((g) => g.area.id)).toEqual(["academic", "system"]);
  });

  // The order list and the module's own `group` must never disagree: a module
  // in the wrong list would be drawn under an area its card does not belong to.
  it.each(HOME_AREAS.map((a) => [a.id, a] as const))("the %s order lists exactly the modules of that group", (_, area) => {
    const ofGroup = MODULES.filter((m) => m.group === area.id).map((m) => m.id);
    expect([...area.order].sort()).toEqual([...ofGroup].sort());
  });

  it("files the modules nobody sees yet where the reviews put them", () => {
    const groupOf = (id: string): string | undefined => MODULES.find((m) => m.id === id)?.group;
    expect(groupOf("billing")).toBe("money");
    expect(groupOf("reports")).toBe("system");
    expect(groupOf("documents")).toBe("people");
  });

  it("makes `group` required: a module without one does not compile", () => {
    // @ts-expect-error — `group` is missing.
    const orphan: ModuleDef = { id: "x", label: "X", icon: "star", color: "lime", desc: "", badge: null, flag: "module_students", perm: "x:read" };
    expect(orphan.id).toBe("x");
  });
});
