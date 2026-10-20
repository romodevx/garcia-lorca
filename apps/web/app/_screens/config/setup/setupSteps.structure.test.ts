import { describe, expect, it } from "vitest";

import type { Cycle, GradeLevel, Group, Level, Term } from "../../../config/api";
import { cycleMonths } from "../../../config/termMonths";
import { STEPS, STRUCTURE_STEPS, deriveSteps, stepsFor, type SetupInput, type StatusRead } from "./setupSteps";

/*
 * «Estructura de la escuela» (PLAN-redesign-estructura, module_school_structure):
 * the step list without «Grupos», with levels, grades and groups measured in
 * one step. The 8-step list stays as it was (setupSteps.test).
 */

const ready = <T,>(value: T): StatusRead<T> => ({ kind: "ready", value });
const ACTIVE: Cycle = { id: "c26", name: "2026–2027", status: "active", startsOn: "2026-08-24", endsOn: "2027-07-09" };
const level = (id: string, name: string): Level => ({ id, name, sepCode: "", enrollmentGranularity: "cycle" });
const PRIM = level("prim", "Primaria");
const SEC = level("sec", "Secundaria");
const grade = (id: string, levelId: string, name: string, ordinal: number): GradeLevel => ({ id, levelId, name, ordinal });
const P1 = grade("p1", "prim", "1º", 1);
const S1 = grade("s1", "sec", "1º", 1);
const term = (levelId: string): Term => ({ id: `t-${levelId}`, cycleId: "c26", levelId, name: "T", ordinal: 1, startsOn: "", endsOn: "", months: cycleMonths(ACTIVE.startsOn, ACTIVE.endsOn) });
const group = (gradeLevelId: string): Group => ({ id: `g-${gradeLevelId}`, cycleId: "c26", gradeLevelId, name: "A", capacity: 30 });

function complete(over: Partial<SetupInput> = {}): SetupInput {
  return {
    profile: ready({ name: "Colegio" }), cycles: [ACTIVE], selectedCycleId: "c26", levels: [PRIM, SEC], selectedLevelId: undefined,
    grades: ready([P1, S1]), terms: ready([term("prim"), term("sec")]), groups: ready([group("p1"), group("s1")]),
    plans: ready([{ levelId: "prim" }, { levelId: "sec" }]), closedDays: ready(1), subjects: ready([{ gradeLevelId: "p1" }, { gradeLevelId: "s1" }]),
    ...over,
  };
}

const structure = (input: SetupInput) => {
  const found = deriveSteps(input, null, STRUCTURE_STEPS).steps.find((s) => s.id === "levels");
  if (found === undefined) throw new Error("no structure step");
  return found;
};

describe("stepsFor", () => {
  it("keeps the eight steps without the flag", () => {
    expect(stepsFor(false)).toBe(STEPS);
  });

  it("merges levels, grades and groups into step 3 of 7 with the flag", () => {
    const steps = stepsFor(true);
    expect(steps.map((s) => s.title)).toEqual(["Datos del colegio", "Ciclo escolar", "Estructura de la escuela", "Periodos", "Materias", "Colegiaturas", "Calendario"]);
    expect(steps.map((s) => s.number)).toEqual([1, 2, 3, 4, 5, 6, 7]);
    expect(steps.some((s) => s.id === "groups")).toBe(false);
  });
});

describe("the merged step's status", () => {
  it("is done with every grade grouped, and says how many of each", () => {
    expect(structure(complete())).toMatchObject({ status: "done", line: "2 niveles · 2 grados · 2 grupos" });
  });

  it("says what is missing, in order: levels, then grades, then the cycle, then groups", () => {
    expect(structure(complete({ levels: [] })).line).toBe("Sin niveles");
    expect(structure(complete({ grades: ready([P1]) })).line).toBe("Faltan grados en Secundaria");
    expect(structure(complete({ selectedCycleId: undefined })).line).toBe("Primero crea el ciclo escolar");
    expect(structure(complete({ groups: ready([group("p1")]) })).line).toBe("Sin grupos en 1º de Secundaria");
  });

  it("is checking while either read is in flight", () => {
    expect(structure(complete({ groups: { kind: "loading" } })).status).toBe("loading");
    expect(structure(complete({ grades: { kind: "loading" } })).status).toBe("loading");
  });

  it("is never ready without a group in every grade", () => {
    const view = deriveSteps(complete({ groups: ready([group("p1")]) }), null, STRUCTURE_STEPS);
    expect(view.ready).toBe(false);
    expect(view.subtitle).toContain("falta Estructura de la escuela");
    expect(deriveSteps(complete(), null, STRUCTURE_STEPS).ready).toBe(true);
  });

  it("ends at Calendario: no Siguiente after step 7", () => {
    const steps = deriveSteps(complete(), null, STRUCTURE_STEPS).steps;
    expect(steps.at(-1)).toMatchObject({ id: "calendar", next: "none" });
  });
});

describe("the step that opens", () => {
  // Plan B2: a session that last had «Grupos» open finds it inside the merged step.
  it("opens the merged step for a stored «groups»", () => {
    expect(deriveSteps(complete(), "groups", STRUCTURE_STEPS).initialStepId).toBe("levels");
    expect(deriveSteps(complete(), "groups", STEPS).initialStepId).toBe("groups");
  });
});
