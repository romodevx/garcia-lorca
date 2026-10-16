import { describe, expect, it } from "vitest";

import { canReachView, moduleIdOfView } from "./moduleAccess";
import type { View } from "./routes";

const MAESTRO = ["teaching:read", "teaching:write", "attendance_own:read", "attendance_own:write"];

describe("moduleIdOfView", () => {
  it.each<[View, string | undefined]>([
    [{ screen: "students" }, "students"],
    [{ screen: "studentEdit", studentId: "s1" }, "students"],
    [{ screen: "studentNew" }, "students"],
    [{ screen: "finance" }, "finance"],
    [{ screen: "payments" }, "payments"],
    [{ screen: "announcements" }, "announcements"],
    [{ screen: "config" }, "settings"],
    [{ screen: "grades" }, "grades"],
    [{ screen: "attendance" }, "attendance"],
    [{ screen: "teachers" }, "teachers"],
    [{ screen: "teacherLoad", teacherId: "t1" }, "teachers"],
    [{ screen: "levelDelete", levelId: "l1" }, "settings"],
    [{ screen: "levelPeriods", levelId: "l1" }, "settings"],
    [{ screen: "studentImport" }, "settings"],
    [{ screen: "uniforms" }, "uniforms"],
    [{ screen: "schedule" }, "schedule"],
    [{ screen: "activities" }, "activities"],
    [{ screen: "scholarships" }, "scholarships"],
    [{ screen: "admin" }, "admin"],
    [{ screen: "module", moduleId: "reports" }, "reports"],
    [{ screen: "dashboard" }, undefined],
    [{ screen: "login" }, undefined],
    [{ screen: "changePassword" }, undefined],
  ])("%j belongs to %s", (view, id) => {
    expect(moduleIdOfView(view)).toBe(id);
  });
});

describe("canReachView", () => {
  it.each<View>([
    { screen: "students" },
    { screen: "finance" },
    { screen: "teachers" },
    { screen: "config" },
    { screen: "admin" },
    { screen: "studentEdit", studentId: "s1" },
  ])("a teacher cannot reach %j", (view) => {
    expect(canReachView(view, MAESTRO)).toBe(false);
  });

  it.each<View>([{ screen: "grades" }, { screen: "attendance" }, { screen: "schedule" }, { screen: "dashboard" }])(
    "a teacher reaches %j",
    (view) => {
      expect(canReachView(view, MAESTRO)).toBe(true);
    },
  );

  it("the owner reaches the admin screens", () => {
    const owner = ["students:read", "finance:read", "teachers:read", "config:read", "users:read"];
    for (const screen of ["students", "finance", "teachers", "config", "admin"] as const) {
      expect(canReachView({ screen }, owner)).toBe(true);
    }
  });

  it("unresolved permissions redirect nobody", () => {
    expect(canReachView({ screen: "finance" }, undefined)).toBe(true);
  });

  it("an unknown module id is left to the placeholder", () => {
    expect(canReachView({ screen: "module", moduleId: "nope" }, MAESTRO)).toBe(true);
  });
});
