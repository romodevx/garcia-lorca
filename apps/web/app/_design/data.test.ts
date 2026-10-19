import { describe, expect, it } from "vitest";
import { MODULES, isModuleEnabled, isModuleVisible, type ModuleDef } from "./data";

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
describe("Inventario module", () => {
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
    const names = (typeof required === "string" ? [required] : required).flat();
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
    const perm = teachers().perm;
    const names = (typeof perm === "string" ? [perm] : perm).flat();
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
    // Task 9: the counter's own permissions (iam 0021).
    "collections:read", "payments:write",
    // T10: the Egresos module (iam 0027).
    "expenses:read", "expenses:write",
    // T16: Horarios.
    "schedule:read", "schedule:write",
    // T17: Actividades.
    "activities:read", "activities:write",
    // T18: Becas.
    "scholarships:read", "scholarships:write",
  ]);
  // These carry a permission nobody is granted ON PURPOSE, so the tile hides
  // itself until the module exists.
  const NOT_BUILT_YET = new Set(["documents", "billing", "reports"]);

  for (const m of MODULES) {
    if (NOT_BUILT_YET.has(m.id)) continue;
    it(`${m.id}`, () => {
      const names = (typeof m.perm === "string" ? [m.perm] : m.perm).flat();
      const reachable = names.filter((p) => GRANTABLE.has(p));
      expect(reachable.length).toBeGreaterThan(0);
    });
  }
});

describe("Registro de pagos is the counter (tarea 9)", () => {
  const byId = (id: string): ModuleDef => MODULES.find((m) => m.id === id) as ModuleDef;
  const cobranza = ["students:read", "students:list", "collections:read", "payments:write"];

  it("shows the counter to the Cobranza role, and not Finanzas", () => {
    expect(isModuleVisible(byId("payments"), cobranza)).toBe(true);
    expect(isModuleVisible(byId("finance"), cobranza)).toBe(false);
  });

  it("still shows the counter to whoever holds the finance pair", () => {
    expect(isModuleVisible(byId("payments"), ["finance:read", "finance:write"])).toBe(true);
  });

  it("hides the counter from a role that can key money but not read what is owed", () => {
    expect(isModuleVisible(byId("payments"), ["payments:write"])).toBe(false);
    expect(isModuleVisible(byId("payments"), ["collections:read"])).toBe(false);
    expect(isModuleVisible(byId("payments"), ["finance:write"])).toBe(false);
  });
});

describe("Egresos (T10)", () => {
  const egresos = MODULES.find((m) => m.id === "expenses") as ModuleDef;

  it("is gated by its own flag and permission", () => {
    expect(egresos.flag).toBe("module_expenses");
    expect(isModuleVisible(egresos, ["expenses:read"])).toBe(true);
  });

  // finance:read is the owner's whole ledger; it does not open Egresos, and
  // expenses:read does not open Finanzas.
  it("does not ride on finance:read", () => {
    expect(isModuleVisible(egresos, ["finance:read", "finance:write"])).toBe(false);
    expect(isModuleVisible(MODULES.find((m) => m.id === "finance") as ModuleDef, ["expenses:read"])).toBe(false);
  });
});

describe("Horarios (T16)", () => {
  const horarios = MODULES.find((m) => m.id === "schedule") as ModuleDef;

  it("is gated by its own flag", () => {
    expect(horarios.label).toBe("Horarios");
    expect(horarios.flag).toBe("module_schedule");
  });

  // The director reads the whole timetable; a teacher reads their own week
  // through teaching:read. Either one reveals the tile.
  it("shows to whoever reads the timetable, and to a teacher for their own", () => {
    expect(isModuleVisible(horarios, ["schedule:read"])).toBe(true);
    expect(isModuleVisible(horarios, ["teaching:read"])).toBe(true);
    expect(isModuleVisible(horarios, ["students:read"])).toBe(false);
  });
});

describe("Actividades (T17)", () => {
  const actividades = MODULES.find((m) => m.id === "activities") as ModuleDef;

  it("is gated by its own flag and permission", () => {
    expect(actividades.label).toBe("Actividades");
    expect(actividades.flag).toBe("module_activities");
    expect(isModuleVisible(actividades, ["activities:read"])).toBe(true);
    expect(isModuleVisible(actividades, ["finance:read", "students:read"])).toBe(false);
  });
});

describe("Becas (T18)", () => {
  const becas = MODULES.find((m) => m.id === "scholarships") as ModuleDef;

  it("is gated by its own flag and permission", () => {
    expect(becas.label).toBe("Becas");
    expect(becas.flag).toBe("module_scholarships");
    expect(isModuleVisible(becas, ["scholarships:read"])).toBe(true);
    expect(isModuleVisible(becas, ["finance:read", "students:read"])).toBe(false);
  });
});

// C2-013: a module whose flag is OFF is not on the panel at all. A module whose
// data lives in another module also needs that module's flag, as AppShell gates it.
describe("isModuleEnabled — flags", () => {
  const byId = (id: string): ModuleDef => {
    const m = MODULES.find((x) => x.id === id);
    if (m === undefined) throw new Error(`${id} module missing`);
    return m;
  };

  it("is on only when the module's own flag is on", () => {
    expect(isModuleEnabled(byId("students"), { module_students: true })).toBe(true);
    expect(isModuleEnabled(byId("students"), {})).toBe(false);
    expect(isModuleEnabled(byId("finance"), { module_students: true })).toBe(false);
  });

  it("keeps Egresos, Actividades and Becas off when finance is off", () => {
    const withoutFinance = { module_expenses: true, module_activities: true, module_scholarships: true };
    for (const id of ["expenses", "activities", "scholarships"]) {
      expect(isModuleEnabled(byId(id), withoutFinance)).toBe(false);
      expect(isModuleEnabled(byId(id), { ...withoutFinance, module_finance: true })).toBe(true);
    }
  });

  it("keeps Horarios off when config is off", () => {
    expect(isModuleEnabled(byId("schedule"), { module_schedule: true })).toBe(false);
    expect(isModuleEnabled(byId("schedule"), { module_schedule: true, module_config: true })).toBe(true);
  });
});
