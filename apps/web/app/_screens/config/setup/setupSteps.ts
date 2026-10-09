import type { Cycle, GradeLevel, Group, Level, SchoolProfile, Term } from "../../../config/api";
import type { TuitionPlan } from "../../../finance/api";

/*
 * The "Puesta en marcha" step list of Configuración académica
 * (PLAN-redesign-configuracion §1.1–§1.4, §4.3): which steps exist, and the
 * pure derivation of each step's status, its line, the header and the step
 * that opens. Presentation only — nothing here decides what a user may do.
 */

export type StepId = "profile" | "cycle" | "levels" | "terms" | "subjects" | "groups" | "tuition" | "calendar";

/**
 * Where a step's own lime button stands (§1.3): always there, never there, or
 * only while the step is empty (then "Siguiente" steps back to text).
 */
export type OwnPrimary = "always" | "never" | "whenEmpty";

export interface StepDef {
  id: StepId;
  number: number;
  title: string;
  /** The line under "Paso N de 8", inside the step. */
  purpose: string;
  /** Counts towards "listo para inscribir". Materias and Calendario never do. */
  measurable: boolean;
  ownPrimary: OwnPrimary;
}

export const STEPS: readonly StepDef[] = [
  { id: "profile", number: 1, title: "Datos del colegio", purpose: "Lo que imprime cada comprobante.", measurable: true, ownPrimary: "whenEmpty" },
  { id: "cycle", number: 2, title: "Ciclo escolar", purpose: "El ciclo decide fechas, periodos y cobros.", measurable: true, ownPrimary: "whenEmpty" },
  { id: "levels", number: 3, title: "Niveles y grados", purpose: "Los niveles que ofrece el colegio y los grados de cada uno.", measurable: true, ownPrimary: "always" },
  { id: "terms", number: 4, title: "Periodos", purpose: "Cómo se parte el año para calificar.", measurable: true, ownPrimary: "whenEmpty" },
  { id: "subjects", number: 5, title: "Materias", purpose: "Las materias de cada grado.", measurable: false, ownPrimary: "always" },
  { id: "groups", number: 6, title: "Grupos", purpose: "Cada grado necesita al menos un grupo para inscribir alumnos.", measurable: true, ownPrimary: "never" },
  { id: "tuition", number: 7, title: "Colegiaturas", purpose: "Lo que cuesta cada nivel en este ciclo.", measurable: true, ownPrimary: "never" },
  { id: "calendar", number: 8, title: "Calendario", purpose: "Días sin clases y actividades. Es opcional.", measurable: false, ownPrimary: "never" },
];

/** The step that opens when nothing is pending: the one edited the most. */
const FALLBACK_STEP: StepId = "levels";

/** How many names the header lists before "y N más". */
const MAX_NAMED_GAPS = 2;

/** One status read: in flight, failed, or its value. */
export type StatusRead<T> = { kind: "loading" } | { kind: "failed" } | { kind: "ready"; value: T };

/**
 * "unknown" is a failed read: no mark and no line — the status helps, it is
 * not data worth an error. "review" is Materias; "optional" is Calendario.
 */
export type StepStatus = "loading" | "unknown" | "done" | "pending" | "review" | "optional";

/** What "Siguiente" looks like in a step: lime, text, or not there at all. */
export type NextKind = "primary" | "text" | "none";

/** The cycle-scoped reads arrive already keyed to the selected cycle (useSetupStatus). */
export interface SetupInput {
  profile: StatusRead<Pick<SchoolProfile, "name">>;
  cycles: Cycle[];
  selectedCycleId: string | undefined;
  levels: Level[];
  selectedLevelId: string | undefined;
  /** Every grade of every level (unfiltered). */
  grades: StatusRead<GradeLevel[]>;
  /** Every term of the selected cycle. */
  terms: StatusRead<Term[]>;
  /** Every group of the selected cycle. */
  groups: StatusRead<Group[]>;
  /** Every tabulador of the selected cycle. */
  plans: StatusRead<Pick<TuitionPlan, "levelId">[]>;
  /** Closed days of the selected cycle. */
  closedDays: StatusRead<number>;
}

export interface StepView extends StepDef {
  status: StepStatus;
  line: string | undefined;
  next: NextKind;
}

export interface SetupView {
  steps: StepView[];
  subtitle: string;
  cardTitle: string;
  /** All six measured steps are done for the selected cycle. */
  ready: boolean;
  /** No status read is still in flight: the opening step can be decided. */
  settled: boolean;
  initialStepId: StepId;
}

interface Mark {
  status: StepStatus;
  line?: string;
}

const LOADING_MARK: Mark = { status: "loading", line: "Revisando…" };
const UNKNOWN_MARK: Mark = { status: "unknown" };
const NO_CYCLE: Mark = { status: "pending", line: "Primero crea el ciclo escolar" };
const NO_LEVELS: Mark = { status: "pending", line: "Primero agrega un nivel" };

const done = (line: string): Mark => ({ status: "done", line });
const pending = (line: string): Mark => ({ status: "pending", line });

const plural = (n: number, one: string, many: string): string => `${n} ${n === 1 ? one : many}`;

/** "Sin grupos en Secundaria" / "Sin grupos en Primaria y 2 más". */
function missingLine(prefix: string, names: string[]): string {
  const rest = names.length - 1;
  return rest > 0 ? `${prefix} ${names[0]} y ${rest} más` : `${prefix} ${names[0]}`;
}

/** Folds a read into a mark: in flight and failed are the same for every step. */
function fromRead<T>(read: StatusRead<T>, decide: (value: T) => Mark): Mark {
  if (read.kind === "loading") return LOADING_MARK;
  if (read.kind === "failed") return UNKNOWN_MARK;
  return decide(read.value);
}

function profileMark(input: SetupInput): Mark {
  return fromRead(input.profile, (p) => (p.name.trim() !== "" ? done(p.name.trim()) : pending("Los comprobantes salen sin nombre")));
}

const statusLabel: Record<Cycle["status"], string> = { draft: "Borrador", active: "Activo", closed: "Cerrado" };

function cycleMark(input: SetupInput): Mark {
  const selected = input.cycles.find((c) => c.id === input.selectedCycleId);
  if (selected?.status === "active") return done(`${selected.name} · activo`);
  const active = input.cycles.find((c) => c.status === "active");
  if (selected === undefined || active === undefined) return pending("Sin ciclo activo");
  return pending(`${statusLabel[selected.status]} · en curso: ${active.name}`);
}

function levelsMark(input: SetupInput): Mark {
  if (input.levels.length === 0) return pending("Sin niveles");
  return fromRead(input.grades, (grades) => {
    const empty = input.levels.filter((l) => !grades.some((g) => g.levelId === l.id)).map((l) => l.name);
    if (empty.length > 0) return pending(missingLine("Faltan grados en", empty));
    const count = grades.filter((g) => input.levels.some((l) => l.id === g.levelId)).length;
    return done(`${plural(input.levels.length, "nivel", "niveles")} · ${plural(count, "grado", "grados")}`);
  });
}

/** Steps measured per level of the selected cycle need both a cycle and a level first. */
function prerequisite(input: SetupInput): Mark | undefined {
  if (input.selectedCycleId === undefined) return NO_CYCLE;
  if (input.levels.length === 0) return NO_LEVELS;
  return undefined;
}

function termsMark(input: SetupInput): Mark {
  return prerequisite(input) ?? fromRead(input.terms, (terms) => {
    const missing = input.levels.filter((l) => !terms.some((t) => t.levelId === l.id)).map((l) => l.name);
    if (missing.length > 0) return pending(missingLine("Sin periodos en", missing));
    return done(`${plural(input.levels.length, "nivel", "niveles")} con periodos`);
  });
}

/** The grades of the known levels, in level order then by ordinal ("1º de Primaria" first). */
function orderedGrades(levels: Level[], grades: GradeLevel[]): { grade: GradeLevel; level: Level }[] {
  return levels.flatMap((level) =>
    grades
      .filter((g) => g.levelId === level.id)
      .sort((a, b) => a.ordinal - b.ordinal)
      .map((grade) => ({ grade, level })),
  );
}

function groupsMark(input: SetupInput): Mark {
  const blocked = prerequisite(input);
  if (blocked !== undefined) return blocked;
  return fromRead(input.grades, (grades) => {
    const known = orderedGrades(input.levels, grades);
    if (known.length === 0) return pending("Primero agrega un grado");
    return fromRead(input.groups, (groups) => {
      const missing = known.filter(({ grade }) => !groups.some((g) => g.gradeLevelId === grade.id));
      if (missing.length > 0) return pending(missingLine("Sin grupos en", missing.map(({ grade, level }) => `${grade.name} de ${level.name}`)));
      return done(plural(groups.length, "grupo", "grupos"));
    });
  });
}

function tuitionMark(input: SetupInput): Mark {
  return prerequisite(input) ?? fromRead(input.plans, (plans) => {
    const missing = input.levels.filter((l) => !plans.some((p) => p.levelId === l.id)).map((l) => l.name);
    if (missing.length === input.levels.length) return pending("Ningún nivel tiene precio");
    if (missing.length > 0) return pending(missingLine("Sin precio en", missing));
    return done(`${plural(input.levels.length, "nivel", "niveles")} con precio`);
  });
}

const OPTIONAL_MARK: Mark = { status: "optional", line: "Opcional" };

function calendarMark(input: SetupInput): Mark {
  if (input.selectedCycleId === undefined) return OPTIONAL_MARK;
  return fromRead(input.closedDays, (n) => (n > 0 ? done(plural(n, "día cerrado", "días cerrados")) : OPTIONAL_MARK));
}

const MARKS: Record<StepId, (input: SetupInput) => Mark> = {
  profile: profileMark,
  cycle: cycleMark,
  levels: levelsMark,
  terms: termsMark,
  subjects: () => ({ status: "review", line: "Revisar" }),
  groups: groupsMark,
  tuition: tuitionMark,
  calendar: calendarMark,
};

/** Whether the step's own lime ("Capturar datos", "Nuevo ciclo", "Crear trimestres") is showing. */
function isEmptyHere(id: StepId, input: SetupInput): boolean {
  if (id === "profile") return input.profile.kind === "ready" && input.profile.value.name.trim() === "";
  if (id === "cycle") return input.cycles.length === 0;
  if (id !== "terms" || input.selectedLevelId === undefined || input.terms.kind !== "ready") return false;
  return !input.terms.value.some((t) => t.levelId === input.selectedLevelId);
}

function nextKind(def: StepDef, input: SetupInput): NextKind {
  if (def.number === STEPS.length) return "none";
  if (def.ownPrimary === "always") return "text";
  if (def.ownPrimary === "never") return "primary";
  return isEmptyHere(def.id, input) ? "text" : "primary";
}

/** "falta Colegiaturas" / "faltan Grupos y Colegiaturas" / "faltan A, B y 2 más". */
function gapsPhrase(titles: string[]): string | undefined {
  if (titles.length === 0) return undefined;
  if (titles.length === 1) return `falta ${titles[0]}`;
  const named = titles.slice(0, MAX_NAMED_GAPS);
  const rest = titles.length - named.length;
  const list = rest > 0 ? `${named.join(", ")} y ${rest} más` : named.join(" y ");
  return `faltan ${list}`;
}

function subtitle(input: SetupInput, steps: StepView[], ready: boolean): string {
  const selected = input.cycles.find((c) => c.id === input.selectedCycleId);
  const head = selected === undefined ? "Sin ciclo escolar" : `Ciclo ${selected.name}`;
  if (ready) return `${head} · listo para inscribir`;
  // The cycle's own state is already named in the head; it is not listed again as a gap.
  const draft = selected?.status === "draft" ? "borrador" : undefined;
  const gaps = steps.filter((s) => s.measurable && s.status === "pending" && s.id !== "cycle").map((s) => s.title);
  return [head, draft, gapsPhrase(gaps)].filter((part) => part !== undefined).join(" · ");
}

function isStepId(value: string | null): value is StepId {
  return STEPS.some((s) => s.id === value);
}

function initialStep(steps: StepView[], stored: string | null): StepId {
  if (isStepId(stored)) return stored;
  return steps.find((s) => s.status === "pending")?.id ?? FALLBACK_STEP;
}

/**
 * Every step's status and line for the selected cycle, the header, and the
 * step that opens. `storedStepId` is this session's last open step
 * (sessionStorage), passed in so the function stays pure.
 */
export function deriveSteps(input: SetupInput, storedStepId: string | null): SetupView {
  const steps = STEPS.map((def): StepView => {
    const mark = MARKS[def.id](input);
    return { ...def, status: mark.status, line: mark.line, next: nextKind(def, input) };
  });
  const ready = steps.filter((s) => s.measurable).every((s) => s.status === "done");
  return {
    steps,
    subtitle: subtitle(input, steps, ready),
    cardTitle: ready ? "Configuración" : "Puesta en marcha",
    ready,
    settled: steps.every((s) => s.status !== "loading"),
    initialStepId: initialStep(steps, storedStepId),
  };
}
