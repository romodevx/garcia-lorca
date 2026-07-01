import { describe, expect, it } from "vitest";

import { matchCommands } from "./match";
import type { Command } from "./types";

/**
 * The matching core: fold accents, fuzzy/substring match over title + aliases,
 * rank by (kind priority, then textual score). Pure and platform-agnostic.
 *
 *   kind priority: module(0) > create(1) > recent(2) > record(3) > quick(4)
 */
const cmd = (over: Partial<Command> & Pick<Command, "id" | "kind" | "title">): Command => ({
  icon: "dot",
  route: "/x",
  ...over,
});

const FIXTURE: Command[] = [
  cmd({ id: "students.nav", kind: "module", title: "Estudiantes", aliases: ["alumnos"] }),
  cmd({ id: "students.create", kind: "create", title: "Crear alumno", aliases: ["nuevo estudiante", "agregar alumno"] }),
  cmd({ id: "grades.nav", kind: "module", title: "Calificaciones", aliases: ["notas"] }),
  cmd({ id: "attendance.quick", kind: "quick", title: "Tomar asistencia" }),
  cmd({ id: "record:GL-1", kind: "record", title: "María Peña", subtitle: "4° A" }),
];

const ids = (q: string, limit?: number): string[] =>
  matchCommands(FIXTURE, q, limit === undefined ? undefined : { limit }).map((s) => s.command.id);

describe("matchCommands", () => {
  it("returns everything (ranked by kind) for an empty query", () => {
    // kind order: module(0), module(0), create(1), record(3), quick(4).
    expect(ids("")).toEqual([
      "students.nav",
      "grades.nav",
      "students.create",
      "record:GL-1",
      "attendance.quick",
    ]);
  });

  it("matches a prefix of the title", () => {
    expect(ids("cal")).toContain("grades.nav");
  });

  it("matches an alias, not just the title", () => {
    expect(ids("notas")).toContain("grades.nav");
    expect(ids("nuevo estudiante")).toContain("students.create");
  });

  it("folds accents in BOTH query and haystack (maria → María, nino → niño)", () => {
    expect(ids("maria")).toContain("record:GL-1");
    expect(ids("pena")).toContain("record:GL-1");
  });

  it("ranks module above create for the same term ('alumno')", () => {
    const got = ids("alumno");
    // Estudiantes (module, via alias 'alumnos') before Crear alumno (create).
    expect(got.indexOf("students.nav")).toBeLessThan(got.indexOf("students.create"));
  });

  it("excludes non-matches", () => {
    expect(ids("zzz")).toEqual([]);
  });

  it("does a fuzzy (subsequence) match, not only substring", () => {
    // "crlaln" is a subsequence of "crear alumno" (folded).
    expect(ids("craln")).toContain("students.create");
  });

  it("respects the limit (default 8, overridable)", () => {
    expect(ids("", 2)).toHaveLength(2);
  });
});
