import { describe, expect, it } from "vitest";

import { canEditTeachingLoad, canGrantTeacherAccess } from "./permissions";

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
  it("needs BOTH teachers:write and users:write", () => {
    expect(canGrantTeacherAccess(["teachers:write", "users:write"])).toBe(true);
    expect(canGrantTeacherAccess(["teachers:write"])).toBe(false);
    expect(canGrantTeacherAccess(["users:write"])).toBe(false);
    expect(canGrantTeacherAccess([])).toBe(false);
  });

  it("offers nothing while the permissions are still unknown: it creates logins", () => {
    expect(canGrantTeacherAccess(undefined)).toBe(false);
  });
});
