import { describe, expect, it } from "vitest";

import type { LevelDeletionCounts } from "../../../config/api";
import { daysLeft, deletedLines, keptLines, longDay, namesMatch, restoredSummary, skippedLine, trashSummary } from "./copy";

const ZERO: LevelDeletionCounts = {
  gradeLevels: 0, groups: 0, subjects: 0, terms: 0, teachingAssignments: 0, students: 0, enrollments: 0,
  attendanceMarks: 0, monthlyGrades: 0, reportCards: 0, tuitionPlans: 0, uniformItems: 0, schoolEvents: 0,
};

describe("deletedLines", () => {
  it("names each academic thing with its count, singular or plural, and hides zeros", () => {
    const lines = deletedLines({ ...ZERO, gradeLevels: 6, groups: 1, reportCards: 120, enrollments: 2 });
    expect(lines).toEqual(["6 grados", "1 grupo", "2 inscripciones a grupos", "120 boletas"]);
  });

  it("never lists what is kept: pupils and uniform items", () => {
    expect(deletedLines({ ...ZERO, students: 40, uniformItems: 3 })).toEqual([]);
  });
});

describe("keptLines", () => {
  it("says the money stays and the pupils wait to be reassigned", () => {
    expect(keptLines({ ...ZERO, students: 120, uniformItems: 4 }, { charges: 10, payments: 1 })).toEqual([
      "10 cargos y 1 pago en Finanzas",
      "120 alumnos quedarán sin nivel, grado ni grupo para reasignarlos",
      "4 prendas de uniforme quedarán sin nivel",
    ]);
  });

  it("says nothing when nothing is kept", () => {
    expect(keptLines(ZERO, { charges: 0, payments: 0 })).toEqual([]);
  });

  it("one pupil reads in the singular", () => {
    expect(keptLines({ ...ZERO, students: 1 }, { charges: 0, payments: 0 })).toEqual([
      "1 alumno quedará sin nivel, grado ni grupo para reasignarlo",
    ]);
  });
});

describe("namesMatch", () => {
  it("ignores case and the spaces around it", () => {
    expect(namesMatch("  primaria ", "PRIMARIA")).toBe(true);
    expect(namesMatch("kínder", "KÍNDER")).toBe(true);
  });

  it("refuses anything else", () => {
    expect(namesMatch("PRIMARIA ALTA", "PRIMARIA")).toBe(false);
    expect(namesMatch("", "PRIMARIA")).toBe(false);
  });
});

describe("dates", () => {
  it("longDay reads on the school's calendar", () => {
    expect(longDay("2026-10-05T17:00:00Z")).toBe("5 de octubre de 2026");
  });

  it("daysLeft rounds up and never goes below zero", () => {
    const now = new Date("2026-09-28T12:00:00Z");
    expect(daysLeft("2026-10-05T11:00:00Z", now)).toBe(7);
    expect(daysLeft("2026-09-28T13:00:00Z", now)).toBe(1);
    expect(daysLeft("2026-09-27T13:00:00Z", now)).toBe(0);
  });
});

describe("restore copy", () => {
  it("summarises what came back", () => {
    expect(restoredSummary("PRIMARIA", { gradeLevels: 6, groups: 0, students: 120 })).toBe("PRIMARIA restaurado: 6 grados, 120 alumnos.");
    expect(restoredSummary("KÍNDER", {})).toBe("KÍNDER restaurado.");
  });

  it("says plainly what did not come back and why", () => {
    expect(skippedLine({ kind: "enrollments", count: 3, reason: "los alumnos ya están en otro grupo" })).toBe(
      "3 inscripciones a grupos no se restauraron: los alumnos ya están en otro grupo",
    );
    expect(skippedLine({ kind: "enrollments", count: 1, reason: "el alumno ya está en otro grupo" })).toBe(
      "1 inscripción a grupo no se restauró: el alumno ya está en otro grupo",
    );
    expect(skippedLine({ kind: "mystery", count: 2, reason: "x" })).toBe("2 registros no se restauraron: x");
  });

  it("trashSummary keeps the row short", () => {
    expect(trashSummary({ ...ZERO, gradeLevels: 6, groups: 12, subjects: 40, terms: 3, students: 120 })).toBe("6 grados · 12 grupos · 40 asignaturas · 120 alumnos");
    expect(trashSummary(ZERO)).toBe("Nivel vacío");
  });
});
