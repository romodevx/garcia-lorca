import { describe, expect, it } from "vitest";

import type { Cycle, GradeLevel, Group, Level, Term } from "../../../config/api";
import { cycleMonths } from "../../../config/termMonths";
import { STEPS, deriveSteps, type SetupInput, type StatusRead, type StepId, type StepStatus } from "./setupSteps";

const LOADING = { kind: "loading" } as const;
const FAILED = { kind: "failed" } as const;
const ready = <T,>(value: T): StatusRead<T> => ({ kind: "ready", value });

const ACTIVE: Cycle = { id: "c26", name: "2026–2027", status: "active", startsOn: "2026-08-24", endsOn: "2027-07-09" };
const DRAFT: Cycle = { id: "c27", name: "2027–2028", status: "draft", startsOn: "2027-08-23", endsOn: "2028-07-07" };
const CLOSED: Cycle = { id: "c25", name: "2025–2026", status: "closed", startsOn: "2025-08-25", endsOn: "2026-07-10" };

const level = (id: string, name: string): Level => ({ id, name, sepCode: "", enrollmentGranularity: "cycle" });
const PRIM = level("prim", "Primaria");
const SEC = level("sec", "Secundaria");
const PREE = level("pree", "Preescolar");

const grade = (id: string, levelId: string, name: string, ordinal: number): GradeLevel => ({ id, levelId, name, ordinal });
const P1 = grade("p1", "prim", "1º", 1);
const S1 = grade("s1", "sec", "1º", 1);
const S2 = grade("s2", "sec", "2º", 2);

/** Every month of the active cycle: Ago 2026 … Jul 2027. */
const ALL_MONTHS = cycleMonths(ACTIVE.startsOn, ACTIVE.endsOn);
/** A level's only term, grading every month of the cycle unless told otherwise. */
const term = (levelId: string, months: string[] = ALL_MONTHS): Term => ({ id: `t-${levelId}`, cycleId: "c26", levelId, name: "1er trimestre", ordinal: 1, startsOn: "", endsOn: "", months });
const group = (gradeLevelId: string): Group => ({ id: `g-${gradeLevelId}`, cycleId: "c26", gradeLevelId, name: "A", capacity: 30 });
const plan = (levelId: string) => ({ levelId });

/** The draft cycle's terms, covering its own months. */
const draftTerms = (): Pick<SetupInput, "terms"> => {
  const months = cycleMonths(DRAFT.startsOn, DRAFT.endsOn);
  return { terms: ready([term("prim", months), term("sec", months)]) };
};

/** A school with everything measurable done, for the active cycle. */
function complete(): SetupInput {
  return {
    profile: ready({ name: "Colegio García Lorca" }),
    cycles: [ACTIVE, DRAFT],
    selectedCycleId: "c26",
    levels: [PRIM, SEC],
    selectedLevelId: undefined,
    grades: ready([P1, S1, S2]),
    terms: ready([term("prim"), term("sec")]),
    groups: ready([group("p1"), group("s1"), group("s2")]),
    plans: ready([plan("prim"), plan("sec")]),
    closedDays: ready(3),
  };
}

const step = (input: SetupInput, id: StepId) => {
  const found = deriveSteps(input, null).steps.find((s) => s.id === id);
  if (found === undefined) throw new Error(`no step ${id}`);
  return found;
};

describe("STEPS registry", () => {
  it("lists the eight steps in order with their titles and purposes (§1.1, §1.2)", () => {
    expect(STEPS.map((s) => [s.number, s.id, s.title, s.purpose])).toEqual([
      [1, "profile", "Datos del colegio", "Lo que imprime cada comprobante."],
      [2, "cycle", "Ciclo escolar", "El ciclo decide fechas, periodos y cobros."],
      [3, "levels", "Niveles y grados", "Los niveles que ofrece el colegio y los grados de cada uno."],
      [4, "terms", "Periodos", "Cómo se parte el año para calificar."],
      [5, "subjects", "Materias", "Las materias de cada grado."],
      [6, "groups", "Grupos", "Cada grado necesita al menos un grupo para inscribir alumnos."],
      [7, "tuition", "Colegiaturas", "Lo que cuesta cada nivel en este ciclo."],
      [8, "calendar", "Calendario", "Días sin clases y actividades. Es opcional."],
    ]);
  });

  it("measures six steps: Materias never counts and Calendario never blocks", () => {
    expect(STEPS.filter((s) => s.measurable).map((s) => s.id)).toEqual(["profile", "cycle", "levels", "terms", "groups", "tuition"]);
  });
});

describe("deriveSteps: done lines (§1.2)", () => {
  it.each<[StepId, string]>([
    ["profile", "Colegio García Lorca"],
    ["cycle", "2026–2027 · activo"],
    ["levels", "2 niveles · 3 grados"],
    ["terms", "2 niveles con periodos"],
    ["groups", "3 grupos"],
    ["tuition", "2 niveles con precio"],
    ["calendar", "3 días cerrados"],
  ])("%s is done with %j", (id, line) => {
    expect(step(complete(), id)).toMatchObject({ status: "done", line });
  });

  it("uses the singular for one of each", () => {
    const input: SetupInput = {
      ...complete(),
      levels: [PRIM],
      grades: ready([P1]),
      terms: ready([term("prim")]),
      groups: ready([group("p1")]),
      plans: ready([plan("prim")]),
      closedDays: ready(1),
    };
    expect(step(input, "levels").line).toBe("1 nivel · 1 grado");
    expect(step(input, "terms").line).toBe("1 nivel con periodos");
    expect(step(input, "groups").line).toBe("1 grupo");
    expect(step(input, "tuition").line).toBe("1 nivel con precio");
    expect(step(input, "calendar").line).toBe("1 día cerrado");
  });

  it("Materias always shows Revisar and is never done", () => {
    expect(step(complete(), "subjects")).toMatchObject({ status: "review", line: "Revisar" });
  });
});

describe("deriveSteps: pending lines (§1.2)", () => {
  const cases: [string, Partial<SetupInput>, StepId, string][] = [
    ["profile without a name", { profile: ready({ name: "  " }) }, "profile", "Los comprobantes salen sin nombre"],
    ["no cycle at all", { cycles: [], selectedCycleId: undefined }, "cycle", "Sin ciclo activo"],
    ["only a draft cycle", { cycles: [DRAFT], selectedCycleId: "c27" }, "cycle", "Sin ciclo activo"],
    ["the selected cycle is a draft", { selectedCycleId: "c27" }, "cycle", "Borrador · en curso: 2026–2027"],
    ["the selected cycle is closed", { cycles: [CLOSED, ACTIVE], selectedCycleId: "c25" }, "cycle", "Cerrado · en curso: 2026–2027"],
    ["one level without grades", { grades: ready([P1]) }, "levels", "Faltan grados en Secundaria"],
    ["two levels without grades", { levels: [PRIM, SEC, PREE], grades: ready([P1]) }, "levels", "Faltan grados en Secundaria y 1 más"],
    ["one level without terms", { terms: ready([term("prim")]) }, "terms", "Sin periodos en Secundaria"],
    ["two levels without terms", { terms: ready([]) }, "terms", "Sin periodos en Primaria y 1 más"],
    ["a level whose terms leave a month out", { terms: ready([term("prim"), term("sec", ALL_MONTHS.slice(1))]) }, "terms", "Meses sin periodo en Secundaria"],
    ["levels whose terms have no months", { terms: ready([term("prim", []), term("sec", [])]) }, "terms", "Meses sin periodo en Primaria y 1 más"],
    ["terms from a gateway that sends no months", { terms: ready([{ ...term("prim"), months: undefined }, term("sec")]) }, "terms", "Meses sin periodo en Primaria"],
    ["one grade without groups", { groups: ready([group("p1"), group("s1")]) }, "groups", "Sin grupos en 2º de Secundaria"],
    ["three grades without groups", { groups: ready([]) }, "groups", "Sin grupos en 1º de Primaria y 2 más"],
    ["no level priced", { plans: ready([]) }, "tuition", "Ningún nivel tiene precio"],
    ["one level unpriced", { plans: ready([plan("prim")]) }, "tuition", "Sin precio en Secundaria"],
    ["two levels unpriced", { levels: [PRIM, SEC, PREE], plans: ready([plan("prim")]) }, "tuition", "Sin precio en Secundaria y 1 más"],
    ["no closed days", { closedDays: ready(0) }, "calendar", "Opcional"],
  ];

  it.each(cases)("%s", (_name, patch, id, line) => {
    const got = step({ ...complete(), ...patch }, id);
    expect(got.line).toBe(line);
    expect(got.status).toBe(id === "calendar" ? "optional" : "pending");
  });
});

describe("deriveSteps: an empty collection is never done", () => {
  const noLevels: SetupInput = { ...complete(), levels: [], grades: ready([]), terms: ready([]), groups: ready([]), plans: ready([]) };

  it.each<[StepId, string]>([
    ["levels", "Sin niveles"],
    ["terms", "Primero agrega un nivel"],
    ["groups", "Primero agrega un nivel"],
    ["tuition", "Primero agrega un nivel"],
  ])("no levels ⇒ %s pending (%j)", (id, line) => {
    expect(step(noLevels, id)).toMatchObject({ status: "pending", line });
  });

  it("levels but no grades at all ⇒ Grupos pending", () => {
    const input: SetupInput = { ...complete(), grades: ready([]), groups: ready([]) };
    expect(step(input, "groups")).toMatchObject({ status: "pending", line: "Primero agrega un grado" });
  });

  it.each<StepId>(["terms", "groups", "tuition"])("no selected cycle ⇒ %s pending, whatever its read says", (id) => {
    const input: SetupInput = { ...complete(), cycles: [], selectedCycleId: undefined, terms: LOADING, groups: FAILED, plans: LOADING };
    expect(step(input, id)).toMatchObject({ status: "pending", line: "Primero crea el ciclo escolar" });
  });

  it("no selected cycle ⇒ Calendario stays optional", () => {
    const input: SetupInput = { ...complete(), cycles: [], selectedCycleId: undefined, closedDays: LOADING };
    expect(step(input, "calendar")).toMatchObject({ status: "optional", line: "Opcional" });
  });
});

describe("deriveSteps: reads in flight or failed (§4.3)", () => {
  const cases: [StepId, Partial<SetupInput>][] = [
    ["profile", { profile: FAILED }],
    ["levels", { grades: FAILED }],
    ["terms", { terms: FAILED }],
    ["groups", { groups: FAILED }],
    ["groups", { grades: FAILED }],
    ["tuition", { plans: FAILED }],
    ["calendar", { closedDays: FAILED }],
  ];

  it.each(cases)("a failed read leaves %s unmarked, without a line", (id, patch) => {
    const got = step({ ...complete(), ...patch }, id);
    expect(got.status).toBe<StepStatus>("unknown");
    expect(got.line).toBeUndefined();
  });

  it.each<[StepId, Partial<SetupInput>]>([
    ["profile", { profile: LOADING }],
    ["levels", { grades: LOADING }],
    ["terms", { terms: LOADING }],
    ["groups", { groups: LOADING }],
    ["tuition", { plans: LOADING }],
    ["calendar", { closedDays: LOADING }],
  ])("a read in flight shows %s as Revisando…", (id, patch) => {
    expect(step({ ...complete(), ...patch }, id)).toMatchObject({ status: "loading", line: "Revisando…" });
  });

  it("is settled only once no read is in flight", () => {
    expect(deriveSteps(complete(), null).settled).toBe(true);
    expect(deriveSteps({ ...complete(), plans: FAILED }, null).settled).toBe(true);
    expect(deriveSteps({ ...complete(), plans: LOADING }, null).settled).toBe(false);
  });
});

describe("deriveSteps: the selected cycle decides", () => {
  it("measures the draft cycle, not the active one", () => {
    const input: SetupInput = { ...complete(), selectedCycleId: "c27", terms: ready([]), groups: ready([]), plans: ready([]) };
    const steps = deriveSteps(input, null).steps;
    expect(steps.filter((s) => s.status === "pending").map((s) => s.id)).toEqual(["cycle", "terms", "groups", "tuition"]);
  });
});

describe("deriveSteps: header", () => {
  const pendingAll: SetupInput = { ...complete(), profile: ready({ name: "" }), terms: ready([]), groups: ready([]), plans: ready([]) };

  it.each<[string, SetupInput, string]>([
    ["0 pending", complete(), "Ciclo 2026–2027 · listo para inscribir"],
    ["1 pending", { ...complete(), plans: ready([]) }, "Ciclo 2026–2027 · falta Colegiaturas"],
    ["2 pending", { ...complete(), groups: ready([]), plans: ready([]) }, "Ciclo 2026–2027 · faltan Grupos y Colegiaturas"],
    ["3+ pending", pendingAll, "Ciclo 2026–2027 · faltan Datos del colegio, Periodos y 2 más"],
    ["a draft cycle", { ...complete(), ...draftTerms(), selectedCycleId: "c27" }, "Ciclo 2027–2028 · borrador"],
    ["a draft cycle with gaps", { ...complete(), ...draftTerms(), selectedCycleId: "c27", plans: ready([]) }, "Ciclo 2027–2028 · borrador · falta Colegiaturas"],
    ["no cycle", { ...complete(), cycles: [], selectedCycleId: undefined }, "Sin ciclo escolar · faltan Periodos, Grupos y 1 más"],
  ])("%s", (_name, input, subtitle) => {
    expect(deriveSteps(input, null).subtitle).toBe(subtitle);
  });

  it("titles the card Configuración once the six measured steps are done, Puesta en marcha before", () => {
    expect(deriveSteps(complete(), null)).toMatchObject({ ready: true, cardTitle: "Configuración" });
    expect(deriveSteps({ ...complete(), plans: ready([]) }, null)).toMatchObject({ ready: false, cardTitle: "Puesta en marcha" });
  });

  it("an unknown step is neither missing nor ready", () => {
    const got = deriveSteps({ ...complete(), plans: FAILED }, null);
    expect(got.subtitle).toBe("Ciclo 2026–2027");
    expect(got.ready).toBe(false);
  });

  it("is not ready without levels, even though nothing per-level is missing", () => {
    const input: SetupInput = { ...complete(), levels: [], grades: ready([]), terms: ready([]), groups: ready([]), plans: ready([]) };
    expect(deriveSteps(input, null).ready).toBe(false);
  });
});

describe("deriveSteps: the step that opens (§1.4)", () => {
  it("reopens the step stored in this session", () => {
    expect(deriveSteps({ ...complete(), plans: ready([]) }, "subjects").initialStepId).toBe("subjects");
  });

  it("ignores a stored value that is not a step", () => {
    expect(deriveSteps({ ...complete(), plans: ready([]) }, "papelera").initialStepId).toBe("tuition");
  });

  it("opens the first pending step otherwise", () => {
    expect(deriveSteps({ ...complete(), groups: ready([]), plans: ready([]) }, null).initialStepId).toBe("groups");
  });

  it("opens Niveles y grados when nothing is pending", () => {
    expect(deriveSteps(complete(), null).initialStepId).toBe("levels");
    expect(deriveSteps({ ...complete(), plans: FAILED, closedDays: ready(0) }, null).initialStepId).toBe("levels");
  });
});

describe("deriveSteps: the Siguiente button (§1.3)", () => {
  const nextOf = (input: SetupInput) => Object.fromEntries(deriveSteps(input, null).steps.map((s) => [s.id, s.next]));

  it("is lime only where the step has no lime of its own, and absent on the last step", () => {
    expect(nextOf(complete())).toEqual({
      profile: "primary",
      cycle: "primary",
      levels: "text",
      terms: "primary",
      subjects: "text",
      groups: "primary",
      tuition: "primary",
      calendar: "none",
    });
  });

  it("is text while the step's own lime shows: empty profile, no cycles, selected level without terms", () => {
    const input: SetupInput = { ...complete(), profile: ready({ name: "" }), cycles: [], selectedCycleId: undefined, selectedLevelId: "sec", terms: ready([term("prim")]) };
    expect(nextOf(input)).toMatchObject({ profile: "text", cycle: "text", terms: "text" });
  });
});

describe("ISSUE-024 · what Migrar alumnos will create, from the catalog itself", () => {
  // The import CREATES the catalog it does not find (config placements, ADR
  // 0033): a gap is something it will fill, never something it needs first.
  const hint = (input: SetupInput) => deriveSteps(input, null).importHint;

  it("an empty catalog: it creates levels, grades and groups", () => {
    expect(hint({ ...complete(), levels: [], grades: ready([]), groups: ready([]) })).toBe("Crea los niveles, grados y grupos que falten.");
  });

  it("a level without grades: it creates grades and groups", () => {
    expect(hint({ ...complete(), grades: ready([P1]), groups: ready([group("p1")]) })).toBe("Crea los grados y grupos que falten.");
  });

  it("a grade without a group in the cycle: it creates groups", () => {
    expect(hint({ ...complete(), groups: ready([group("p1")]) })).toBe("Crea los grupos que falten.");
  });

  it("no cycle chosen: groups are what it would create", () => {
    expect(hint({ ...complete(), selectedCycleId: undefined, groups: ready([]) })).toBe("Crea los grupos que falten.");
  });

  it("a complete catalog: no hint", () => {
    expect(hint(complete())).toBeUndefined();
  });

  it("never asks for anything first", () => {
    const inputs = [{ ...complete(), levels: [], grades: ready([]), groups: ready([]) }, { ...complete(), groups: ready([]) }];
    for (const input of inputs) expect(hint(input)).not.toMatch(/Necesita/);
  });

  it("says nothing while a read is in flight or failed", () => {
    expect(hint({ ...complete(), grades: LOADING })).toBeUndefined();
    expect(hint({ ...complete(), groups: FAILED })).toBeUndefined();
  });
});
