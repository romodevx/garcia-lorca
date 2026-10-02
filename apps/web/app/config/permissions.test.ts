import { describe, expect, it } from "vitest";

import { canDeleteLevels, canWriteConfig } from "./permissions";

describe("canDeleteLevels", () => {
  it("only with levels:delete", () => {
    expect(canDeleteLevels(["config:read", "levels:delete"])).toBe(true);
    expect(canDeleteLevels(["config:read", "config:write"])).toBe(false);
  });

  it("offers nothing while the permissions are still unknown — it is destructive", () => {
    expect(canDeleteLevels(undefined)).toBe(false);
  });
});

describe("canWriteConfig", () => {
  it("only config:write edits the catalog", () => {
    expect(canWriteConfig(["config:read", "config:write"])).toBe(true);
    expect(canWriteConfig(["config:read"])).toBe(false);
    expect(canWriteConfig(undefined)).toBe(false);
  });
});
