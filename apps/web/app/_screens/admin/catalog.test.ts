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

describe("PERMISSION_CATALOG — notas del alumno (tarea 6)", () => {
  // Writing notes is delegable (a teacher writes them), so the role editor
  // must offer it. Voiding anyone's note is the owner's alone: iam refuses to
  // grant it, so offering it would only produce an error.
  it("offers writing notes, and not voiding anyone's", () => {
    expect(ALL_PERMISSIONS).toContain("students:notes_write");
    expect(ALL_PERMISSIONS).not.toContain("students:notes_void");
  });
});

describe("PERMISSION_CATALOG — ventanilla (tarea 9)", () => {
  // The counter's two permissions are delegable (that is the Cobranza role);
  // finance:read/write stay owner-only in iam, so the editor never offers them.
  it("offers the counter's permissions, and not the whole of finance", () => {
    expect(ALL_PERMISSIONS).toContain("collections:read");
    expect(ALL_PERMISSIONS).toContain("payments:write");
    expect(ALL_PERMISSIONS).not.toContain("finance:read");
    expect(ALL_PERMISSIONS).not.toContain("finance:write");
  });
});
