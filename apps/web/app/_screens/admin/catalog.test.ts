import { describe, expect, it } from "vitest";
import { ALL_PERMISSIONS, GRANTABLE_PERMISSIONS, PERMISSION_CATALOG } from "./catalog";

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

describe("PERMISSION_CATALOG — cerrar periodos (tarea 7)", () => {
  // Closing a trimester publishes boletas to every family: the owner's call
  // alone. It is listed so the director sees it exists, marked owner-only so
  // the editor never lets a role be granted it.
  it("lists grades:close under Calificaciones, marked owner-only", () => {
    const group = PERMISSION_CATALOG.find((g) => g.resource === "Calificaciones");
    const close = group?.permissions.find((p) => p.value === "grades:close");
    expect(close?.label).toBe("Cerrar periodos y emitir boletas");
    expect(close?.ownerOnly).toBe(true);
  });

  it("keeps it out of the grantable set", () => {
    expect(GRANTABLE_PERMISSIONS).not.toContain("grades:close");
    expect(GRANTABLE_PERMISSIONS).toContain("grades:write");
  });
});

describe("PERMISSION_CATALOG — eliminar usuarios (T2)", () => {
  // Deleting an account is the director's alone (owner answer 4): listed so
  // it can be seen, marked owner-only so no role is ever offered it.
  it("lists users:delete under Usuarios, marked owner-only and not grantable", () => {
    const group = PERMISSION_CATALOG.find((g) => g.resource === "Usuarios");
    const del = group?.permissions.find((p) => p.value === "users:delete");
    expect(del?.label).toBe("Eliminar usuarios");
    expect(del?.ownerOnly).toBe(true);
    expect(GRANTABLE_PERMISSIONS).not.toContain("users:delete");
    expect(GRANTABLE_PERMISSIONS).toContain("users:write");
  });
});

describe("PERMISSION_CATALOG — Horarios (T16)", () => {
  it("groups reading and editing the timetable under Horarios", () => {
    const group = PERMISSION_CATALOG.find((g) => g.resource === "Horarios");
    expect(group?.permissions.map((p) => [p.value, p.label])).toEqual([
      ["schedule:read", "Ver horarios"],
      ["schedule:write", "Editar horarios y registrar cambios del día"],
    ]);
    expect(GRANTABLE_PERMISSIONS).toContain("schedule:write");
  });
});
