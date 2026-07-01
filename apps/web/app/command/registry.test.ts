import { messages } from "@garcia-lorca/i18n";
import type { FlagSet } from "@garcia-lorca/ui/flags";
import { describe, expect, it } from "vitest";

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

  it("localizes titles and splits aliases from i18n", () => {
    const students = buildCommandRegistry({ module_students: true }, t);
    const create = students.find((c) => c.id === "students.create");
    expect(create?.title).toBe("Crear alumno");
    expect(create?.aliases).toContain("nuevo estudiante");
    expect(create?.aliases).toContain("agregar alumno");
  });

  it("keys each command's route to its module for navigation", () => {
    const [nav] = buildCommandRegistry({ module_students: true }, t);
    expect(nav?.route).toBe("module:students");
    expect(nav?.moduleId).toBe("students");
  });
});
