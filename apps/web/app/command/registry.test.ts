import { messages } from "@garcia-lorca/i18n";
import type { FlagSet } from "@garcia-lorca/ui/flags";
import { describe, expect, it } from "vitest";

import { MODULES } from "../_design/data";
import { buildCommandRegistry } from "./registry";

/**
 * Layer A registry: derive nav + create + quick commands from the module list,
 * FILTERED by feature flags (a disabled module contributes ZERO commands — the
 * security-relevant behavior the palette depends on). Titles are localized.
 *
 * NOTE: in this demo SPA `flags` come from the client `DEMO_FLAGS`; when auth +
 * gateway land, this same function runs server-side (RSC) against real per-user
 * flags/role so disallowed commands never reach the bundle.
 */
const t = (key: string): string => (messages.es.palette as Record<string, string>)[key] ?? key;

const ids = (flags: FlagSet): string[] => buildCommandRegistry(flags, t).map((c) => c.id);

describe("buildCommandRegistry", () => {
  it("includes a module nav command for each enabled module", () => {
    const got = ids({ module_students: true, module_finance: true });
    expect(got).toContain("students.nav");
    expect(got).toContain("finance.nav");
  });

  it("excludes ALL commands of a disabled module (flag OFF → 0 commands)", () => {
    const got = ids({ module_students: true }); // finance OFF
    expect(got).toContain("students.nav");
    expect(got.some((id) => id.startsWith("finance."))).toBe(false);
  });

  it("emits create/quick commands only for enabled modules", () => {
    expect(ids({ module_students: true })).toContain("students.create");
    expect(ids({ module_attendance: true })).toContain("attendance.quick");
    expect(ids({ module_students: true })).not.toContain("attendance.quick");
  });

  it("returns nothing when no flags are enabled", () => {
    expect(buildCommandRegistry({}, t)).toEqual([]);
  });

  it("never surfaces a module deactivated from the panel, even with its flag ON", () => {
    // "documents" is hidden (school doesn't offer it) — kept future-ready, but the
    // palette must not expose its nav/create commands regardless of the flag value.
    const got = ids({ module_docs: true });
    expect(got.some((id) => id.startsWith("documents."))).toBe(false);
  });

  it("localizes titles and splits aliases from i18n", () => {
    const students = buildCommandRegistry({ module_students: true }, t);
    const create = students.find((c) => c.id === "students.create");
    expect(create?.title).toBe("Crear alumno");
    expect(create?.aliases).toContain("nuevo estudiante");
    expect(create?.aliases).toContain("agregar alumno");
  });

  it("registers the config academic-catalog commands under module_config", () => {
    const got = ids({ module_config: true });
    // Nav to the catalog plus the fewest-clicks create shortcuts.
    expect(got).toContain("settings.nav");
    expect(got).toContain("settings.create.cycle");
    expect(got).toContain("settings.create.level");
    expect(got).toContain("settings.create.group");
    expect(got).toContain("settings.create.teacher");
    expect(got).toContain("settings.create.assignment");
    // All route back to the config surface.
    const cmds = buildCommandRegistry({ module_config: true }, t);
    expect(cmds.find((c) => c.id === "settings.create.cycle")?.route).toBe("module:settings");
    expect(cmds.find((c) => c.id === "settings.create.teacher")?.route).toBe("module:settings");
    expect(cmds.find((c) => c.id === "settings.create.assignment")?.route).toBe("module:settings");
    // Teacher command is localized + alias-split from i18n.
    const teacher = cmds.find((c) => c.id === "settings.create.teacher");
    expect(teacher?.title).toBe("Crear profesor");
    expect(teacher?.aliases).toContain("nuevo profesor");
  });

  it("emits zero config commands when module_config is OFF", () => {
    expect(ids({ module_students: true }).some((id) => id.startsWith("settings."))).toBe(false);
  });

  it("registers the enroll + transfer commands under module_students", () => {
    const got = ids({ module_students: true });
    expect(got).toContain("students.create.enroll");
    expect(got).toContain("students.create.transfer");
    const cmds = buildCommandRegistry({ module_students: true }, t);
    // Both route to the students surface (the enrollment screen).
    expect(cmds.find((c) => c.id === "students.create.enroll")?.route).toBe("module:students");
    expect(cmds.find((c) => c.id === "students.create.transfer")?.route).toBe("module:students");
    // Localized + alias-split from i18n.
    const enroll = cmds.find((c) => c.id === "students.create.enroll");
    expect(enroll?.title).toBe("Inscribir alumno");
    expect(enroll?.aliases).toContain("matricular");
  });

  it("emits zero enroll/transfer commands when module_students is OFF", () => {
    const got = ids({ module_finance: true });
    expect(got.some((id) => id.startsWith("students."))).toBe(false);
  });

  it("registers the capture-grades command under module_grades", () => {
    const got = ids({ module_grades: true });
    expect(got).toContain("grades.create.capture");
    const cmds = buildCommandRegistry({ module_grades: true }, t);
    // Routes to the grades surface (the roster grid).
    expect(cmds.find((c) => c.id === "grades.create.capture")?.route).toBe("module:grades");
    // Localized + alias-split from i18n.
    const capture = cmds.find((c) => c.id === "grades.create.capture");
    expect(capture?.title).toBe("Capturar calificaciones");
    expect(capture?.aliases).toContain("cargar notas");
  });

  it("emits zero grades commands when module_grades is OFF", () => {
    const got = ids({ module_students: true });
    expect(got.some((id) => id.startsWith("grades."))).toBe(false);
  });

  it("registers the admin (iam) surface commands under module_iam", () => {
    const got = ids({ module_iam: true });
    // Nav to the admin surface plus the fewest-clicks create shortcuts.
    expect(got).toContain("admin.nav");
    expect(got).toContain("admin.create.user");
    expect(got).toContain("admin.create.role");
    const cmds = buildCommandRegistry({ module_iam: true }, t);
    // All route back to the admin surface.
    expect(cmds.find((c) => c.id === "admin.create.user")?.route).toBe("module:admin");
    expect(cmds.find((c) => c.id === "admin.create.role")?.route).toBe("module:admin");
    // Localized + alias-split from i18n.
    const createUser = cmds.find((c) => c.id === "admin.create.user");
    expect(createUser?.title).toBe("Crear usuario");
    expect(createUser?.aliases).toContain("nuevo usuario");
  });

  it("emits zero admin commands when module_iam is OFF", () => {
    expect(ids({ module_students: true }).some((id) => id.startsWith("admin."))).toBe(false);
  });

  it("routes the ventanilla shortcut straight to the payment flow", () => {
    const cmds = buildCommandRegistry({ module_finance: true }, t);
    // Nav opens the surface; the create shortcut opens it ON the payment form.
    expect(cmds.find((c) => c.id === "finance.nav")?.route).toBe("module:finance");
    expect(cmds.find((c) => c.id === "finance.create")?.route).toBe("module:finance:payment");
  });

  it("keys each command's route to its module for navigation", () => {
    const [nav] = buildCommandRegistry({ module_students: true }, t);
    expect(nav?.route).toBe("module:students");
    expect(nav?.moduleId).toBe("students");
  });

  // Permission filtering (role-aware visibility): when a permission set is
  // supplied, a module contributes commands only if the caller holds its READ
  // permission — layered ON TOP of the flag gate. Omitting the set (undefined)
  // preserves the flag-only behavior the demo/unit tests above rely on.
  describe("permission filtering", () => {
    const idsP = (flags: FlagSet, perms: readonly string[]): string[] =>
      buildCommandRegistry(flags, t, perms).map((c) => c.id);

    it("surfaces only modules the caller holds READ on, even when the flag is ON", () => {
      // grades + finance flags ON, but the caller only has grades:read.
      const got = idsP({ module_grades: true, module_finance: true }, ["grades:read"]);
      expect(got).toContain("grades.nav");
      expect(got).toContain("grades.create.capture");
      expect(got.some((id) => id.startsWith("finance."))).toBe(false);
    });

    it("drops a module whose flag is ON but whose READ permission is missing", () => {
      const got = idsP({ module_finance: true }, ["grades:read"]);
      expect(got.some((id) => id.startsWith("finance."))).toBe(false);
    });

    it("still requires the flag: a permitted module with its flag OFF stays absent", () => {
      // Caller CAN read finance, but the module flag is OFF → no commands.
      const got = idsP({ module_grades: true }, ["grades:read", "finance:read"]);
      expect(got).toContain("grades.nav");
      expect(got.some((id) => id.startsWith("finance."))).toBe(false);
    });

    it("returns nothing when the caller holds no matching permissions", () => {
      expect(buildCommandRegistry({ module_finance: true, module_grades: true }, t, [])).toEqual([]);
    });

    it("maps admin visibility to users:read and settings to config:read", () => {
      const admin = idsP({ module_iam: true, module_config: true }, ["users:read"]);
      expect(admin).toContain("admin.nav");
      expect(admin.some((id) => id.startsWith("settings."))).toBe(false);

      const config = idsP({ module_iam: true, module_config: true }, ["config:read"]);
      expect(config).toContain("settings.nav");
      expect(config.some((id) => id.startsWith("admin."))).toBe(false);
    });

    it("omitting the permission set keeps flag-only behavior (backward compatible)", () => {
      const got = buildCommandRegistry({ module_finance: true }, t).map((c) => c.id);
      expect(got).toContain("finance.nav");
    });
  });

  it("a teacher (maestro role) gets only Asistencia and Calificaciones commands, every flag ON", () => {
    const allOn = Object.fromEntries(MODULES.map((m) => [m.flag, true]));
    const maestro = ["teaching:read", "teaching:write", "attendance_own:read", "attendance_own:write"];

    const modules = new Set(buildCommandRegistry(allOn, t, maestro).map((c) => c.moduleId));

    expect([...modules].sort()).toEqual(["attendance", "grades"]);
  });
});
