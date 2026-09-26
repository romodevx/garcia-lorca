import { describe, expect, it } from "vitest";

import { canEditTeachingLoad, canGrantTeacherAccess, canListUsers } from "./permissions";

describe("canEditTeachingLoad", () => {
  it("needs teachers:write", () => {
    expect(canEditTeachingLoad(["teachers:read", "teachers:write"])).toBe(true);
    expect(canEditTeachingLoad(["teachers:read"])).toBe(false);
  });

  it("does not hide anything while the permissions are still unknown (the backend decides)", () => {
    expect(canEditTeachingLoad(undefined)).toBe(true);
  });
});

describe("canGrantTeacherAccess", () => {
  const maestro = ["teaching:read", "teaching:write", "attendance_own:read", "attendance_own:write"];

  it("needs teachers:write, users:write AND every maestro permission (the backend's subset guard)", () => {
    expect(canGrantTeacherAccess(["teachers:write", "users:write", ...maestro])).toBe(true);
    expect(canGrantTeacherAccess(["teachers:write", "users:write"])).toBe(false);
    expect(canGrantTeacherAccess(["teachers:write", ...maestro])).toBe(false);
    expect(canGrantTeacherAccess(["users:write", ...maestro])).toBe(false);
    expect(canGrantTeacherAccess([])).toBe(false);
  });

  it.each(maestro)("without %s the 403 is certain, so nothing is offered", (missing) => {
    const held = ["teachers:write", "users:write", ...maestro.filter((p) => p !== missing)];
    expect(canGrantTeacherAccess(held)).toBe(false);
  });

  it("offers nothing while the permissions are still unknown: it creates logins", () => {
    expect(canGrantTeacherAccess(undefined)).toBe(false);
  });
});

describe("canListUsers", () => {
  it("needs users:read, and waits for the answer: it reads every account", () => {
    expect(canListUsers(["users:read"])).toBe(true);
    expect(canListUsers(["users:write"])).toBe(false);
    expect(canListUsers(undefined)).toBe(false);
  });
});
