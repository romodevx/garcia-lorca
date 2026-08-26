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

const attendanceModule = (): ModuleDef => {
  const m = MODULES.find((x) => x.id === "attendance");
  if (m === undefined) throw new Error("attendance module missing");
  return m;
};

describe("Asistencia module", () => {
  it("is visible to an attendance_own:read-only principal (a teacher, no attendance:read)", () => {
    expect(isModuleVisible(attendanceModule(), ["attendance_own:read"])).toBe(true);
  });

  it("is still visible to an attendance:read principal (a director)", () => {
    expect(isModuleVisible(attendanceModule(), ["attendance:read"])).toBe(true);
  });

  it("is hidden from a principal holding neither attendance perm", () => {
    expect(isModuleVisible(attendanceModule(), ["students:read"])).toBe(false);
  });
});

const uniformsModule = (): ModuleDef => {
  const m = MODULES.find((x) => x.id === "uniforms");
  if (m === undefined) throw new Error("uniforms module missing");
  return m;
};

/**
 * These exist because the tile was invisible to everybody for a whole slice.
 *
 * The read permission was split into uniforms:stock and uniforms:orders — the
 * storeroom needs no student names, the order list carries every one of them —
 * and the migration, the iam domain and the gateway were all updated. The tile
 * kept asking for `uniforms:read`, which by then existed nowhere, so nobody held
 * it and the module never appeared on the dashboard.
 *
 * Every permission this registry names must be one somebody can actually hold.
 */
describe("Uniformes module", () => {
  it("is visible to a storeroom principal, who holds no student-data permission", () => {
    expect(isModuleVisible(uniformsModule(), ["uniforms:stock"])).toBe(true);
  });

  it("is visible to a director, who may also read the order list", () => {
    expect(isModuleVisible(uniformsModule(), ["uniforms:stock", "uniforms:orders"])).toBe(true);
  });

  it("is hidden from a principal holding no uniforms permission", () => {
    expect(isModuleVisible(uniformsModule(), ["students:read"])).toBe(false);
  });

  it("does not ask for a permission that no longer exists", () => {
    // uniforms:read was retired by the split. A tile gated on a retired
    // permission is a tile nobody ever sees.
    const required = uniformsModule().perm;
    const names = typeof required === "string" ? [required] : required;
    expect(names).not.toContain("uniforms:read");
  });
});

describe("Docentes is a module, not a panel inside Configuración", () => {
  const teachers = (): ModuleDef => {
    const m = MODULES.find((x) => x.id === "teachers");
    if (m === undefined) throw new Error("the teachers module is missing from MODULES");
    return m;
  };

  it("gates on its own flag and its own permission", () => {
    // Not module_config and not config:read: the whole point of the move is that
    // a school can hand somebody the staff roster without handing them the
    // cycles, levels, grades and groups as well.
    expect(teachers().flag).toBe("module_teachers");
    const names = typeof teachers().perm === "string" ? [teachers().perm] : teachers().perm;
    expect(names).toContain("teachers:read");
    expect(names).not.toContain("config:read");
  });

  it("is visible to a teachers:read holder who cannot read the catalogue", () => {
    expect(isModuleVisible(teachers(), ["teachers:read"])).toBe(true);
  });

  it("stays hidden from a config-only holder", () => {
    expect(isModuleVisible(teachers(), ["config:read", "config:write"])).toBe(false);
  });
});

/**
 * The wider version of the same bug: any tile gated on a permission the system
 * never grants is a module nobody can reach. The deactivated ones are excluded
 * on purpose — they hold an ungrantable permission precisely so they disappear.
 */
describe("every live module's permission is one somebody can hold", () => {
  // The catalogue the iam migrations actually grant.
  const GRANTABLE = new Set([
    "config:read", "config:write",
    "students:read", "students:list", "students:create", "students:update", "students:enroll",
    "grades:read", "grades:write",
    "users:read", "users:write", "roles:read", "roles:write",
    "finance:read", "finance:write",
    "attendance:read", "attendance:write", "attendance_own:read", "attendance_own:write",
    "comms:read", "comms:write",
    "guardians:read", "guardians:write",
    "teaching:read", "teaching:write",
    "family:read",
    "uniforms:stock", "uniforms:orders", "uniforms:write",
    "uniforms_own:read", "uniforms_own:write",
    "teachers:read", "teachers:write",
  ]);
  // These carry a permission nobody is granted ON PURPOSE, so the tile hides
  // itself until the module exists.
  const NOT_BUILT_YET = new Set(["scholarships", "documents", "billing", "reports"]);

  for (const m of MODULES) {
    if (NOT_BUILT_YET.has(m.id)) continue;
    it(`${m.id}`, () => {
      const names = typeof m.perm === "string" ? [m.perm] : m.perm;
      const reachable = names.filter((p) => GRANTABLE.has(p));
      expect(reachable.length).toBeGreaterThan(0);
    });
  }
});
