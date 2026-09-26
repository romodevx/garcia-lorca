import { describe, expect, it } from "vitest";

import { canEditTeachingLoad } from "./permissions";

describe("canEditTeachingLoad", () => {
  it("needs teachers:write", () => {
    expect(canEditTeachingLoad(["teachers:read", "teachers:write"])).toBe(true);
    expect(canEditTeachingLoad(["teachers:read"])).toBe(false);
  });

  it("does not hide anything while the permissions are still unknown (the backend decides)", () => {
    expect(canEditTeachingLoad(undefined)).toBe(true);
  });
});
