import { describe, expect, it } from "vitest";
import { ALL_PERMISSIONS, PERMISSION_CATALOG } from "./catalog";

describe("PERMISSION_CATALOG — teaching (Maestro role)", () => {
  it("offers teaching:read and teaching:write so an admin can build a Maestro role", () => {
    expect(ALL_PERMISSIONS).toContain("teaching:read");
    expect(ALL_PERMISSIONS).toContain("teaching:write");
  });

  it("groups the teaching permissions under a Docencia resource with clear labels", () => {
    const group = PERMISSION_CATALOG.find((g) => g.permissions.some((p) => p.value === "teaching:read"));
    expect(group?.resource).toBe("Docencia");
    const read = group?.permissions.find((p) => p.value === "teaching:read");
    const write = group?.permissions.find((p) => p.value === "teaching:write");
    expect(read?.label).toMatch(/mis grupos/i);
    expect(write?.label).toMatch(/mis grupos/i);
  });
});
