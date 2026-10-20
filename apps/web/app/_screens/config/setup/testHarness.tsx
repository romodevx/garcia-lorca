import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, vi } from "vitest";

import * as configApi from "../../../config/api";
import type { Cycle, GradeLevel, Group, Level, LevelTrashItem, NonWorkingPeriod, SchoolEvent, Subject, Term } from "../../../config/api";
import { cycleMonths } from "../../../config/termMonths";
import * as financeApi from "../../../finance/api";
import type { TuitionPlan } from "../../../finance/api";
import type { CatalogProps } from "../Catalog";
import { CatalogSetup } from "./CatalogSetup";

/*
 * Test-only harness for the setup view. The test file mocks config/api and
 * finance/api; `installSchool` answers them from one in-memory school, and the
 * reads BRANCH ON THEIR ARGUMENTS (filtered or not), so a step mark can never
 * turn green just because a mock returns the same list to every caller.
 */

export const ACTIVE: Cycle = { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-01", endsOn: "2027-06-30" };
export const DRAFT: Cycle = { id: "c2", name: "2027-2028", status: "draft", startsOn: "2027-08-01", endsOn: "2028-06-30" };
export const PRIMARIA: Level = { id: "l1", name: "PRIMARIA", sepCode: "", enrollmentGranularity: "cycle" };
export const SECUNDARIA: Level = { id: "l2", name: "SECUNDARIA", sepCode: "", enrollmentGranularity: "cycle" };
export const G1: GradeLevel = { id: "g1", levelId: "l1", name: "1° PRIMARIA", ordinal: 1 };
export const G2: GradeLevel = { id: "g2", levelId: "l1", name: "2° PRIMARIA", ordinal: 2 };
export const G9: GradeLevel = { id: "g9", levelId: "l2", name: "1° SECUNDARIA", ordinal: 1 };
/** A level's one term, grading every month of ACTIVE: step 4 counts it done. */
export const T1: Term = { id: "t1", cycleId: "c1", levelId: "l1", name: "1.er trimestre", ordinal: 1, startsOn: "2026-08-01", endsOn: "2026-11-30", months: cycleMonths(ACTIVE.startsOn, ACTIVE.endsOn) };
export const T9: Term = { ...T1, id: "t9", levelId: "l2" };
export const GROUP_A: Group = { id: "gr1", cycleId: "c1", gradeLevelId: "g1", name: "A", capacity: 30 };
export const PLAN: TuitionPlan = {
  id: "p1", cycleId: "c1", levelId: "l1", enrollmentCents: 0, monthlyCents: 280000,
  months: 11, firstBilledMonth: 9, dueDay: 7, lateFeeCents: 0, active: true,
  policy: { dailyMode: false, lateFeeDailyCents: 0, surchargeCountsAllDays: true, earlyPaymentCents: 0, earlyPaymentDays: 0 },
  schedule: [], monthlyDiscountCents: 0, fees: [], enrollmentDueOn: "2026-06-30", enrollmentStartsOn: "",
  enrollmentConcepts: [{ name: "INSCRIPCIÓN", tiers: [{ effectiveFrom: "2025-08-31", effectiveTo: "", amountCents: 450000 }] }],
};
export const HOLIDAY: NonWorkingPeriod = { id: "d1", cycleId: "c1", label: "PATRIA", kind: "holiday", startsOn: "2026-09-16", endsOn: "2026-09-16" };
export const MATH: Subject = { id: "s1", gradeLevelId: "g1", name: "MATEMÁTICAS" };
export const TRASHED: LevelTrashItem = {
  batchId: "b1", levelId: "l7", levelName: "MATERNAL", deletedAt: "2026-09-27T17:00:00Z", restorableUntil: "2099-01-01T00:00:00Z",
  deletedByName: "Luis Beltrán", status: "deleted",
  counts: { gradeLevels: 0, groups: 0, subjects: 0, terms: 0, teachingAssignments: 0, students: 0, enrollments: 0, attendanceMarks: 0, monthlyGrades: 0, reportCards: 0, tuitionPlans: 0, uniformItems: 0, schoolEvents: 0 },
};

export interface FakeSchool {
  profileName: string;
  cycles: Cycle[];
  levels: Level[];
  grades: GradeLevel[];
  terms: Term[];
  groups: Group[];
  plans: TuitionPlan[];
  closedDays: NonWorkingPeriod[];
  events: SchoolEvent[];
  subjects: Subject[];
  trash: LevelTrashItem[];
}

/** A school with everything measured done, except Grupos (2° and SECUNDARIA have none) and SECUNDARIA's price. */
export function freshSchool(over: Partial<FakeSchool> = {}): FakeSchool {
  return {
    profileName: "GARCÍA LORCA",
    cycles: [ACTIVE],
    levels: [PRIMARIA, SECUNDARIA],
    grades: [G1, G2, G9],
    terms: [T1, T9],
    groups: [GROUP_A],
    plans: [PLAN],
    closedDays: [],
    events: [],
    subjects: [MATH],
    trash: [],
    ...over,
  };
}

const EMPTY_PROFILE = { name: "", legalName: "", rfc: "", cct: "", address: "", phone: "", legend: "", clabe: "" };
const RUN = { tuitionCreated: 3, enrollmentCreated: 0, lateFeesCreated: 0, feesCreated: 0, enrollmentMoved: 0, skipped: 0, levelsWithoutPlan: [] };

let seq = 0;
const nextId = (prefix: string): string => `${prefix}-new-${++seq}`;

function installReads(s: FakeSchool): void {
  const c = vi.mocked(configApi);
  c.listCycles.mockImplementation(async () => [...s.cycles]);
  c.listLevels.mockImplementation(async () => [...s.levels]);
  c.listGradeLevels.mockImplementation(async (_, levelId) => s.grades.filter((g) => levelId === undefined || g.levelId === levelId));
  c.listTerms.mockImplementation(async (_, cycleId, levelId) => s.terms.filter((t) => t.cycleId === cycleId && (levelId === undefined || t.levelId === levelId)));
  c.listGroups.mockImplementation(async (_, cycleId, gradeId) => s.groups.filter((g) => g.cycleId === cycleId && (gradeId === undefined || g.gradeLevelId === gradeId)));
  c.getSchoolProfile.mockImplementation(async () => ({ found: s.profileName !== "", profile: { ...EMPTY_PROFILE, name: s.profileName } }));
  c.listSubjects.mockImplementation(async () => [...s.subjects]);
  c.listNonWorkingPeriods.mockImplementation(async (_, cycleId) => s.closedDays.filter((d) => d.cycleId === cycleId));
  c.listSchoolEvents.mockImplementation(async (_, cycleId) => s.events.filter((e) => e.cycleId === cycleId));
  c.listLevelTrash.mockImplementation(async () => [...s.trash]);
  vi.mocked(financeApi).listTuitionPlans.mockImplementation(async (_, cycleId) => s.plans.filter((p) => p.cycleId === cycleId));
}

function installWrites(s: FakeSchool): void {
  const c = vi.mocked(configApi);
  const f = vi.mocked(financeApi);
  c.createCycle.mockImplementation(async (_, input) => { const made: Cycle = { id: nextId("c"), status: "draft", ...input }; s.cycles.push(made); return made; });
  c.activateCycle.mockImplementation(async (_, id) => ({ ...(s.cycles.find((x) => x.id === id) ?? ACTIVE), status: "active" }));
  c.createLevel.mockImplementation(async (_, input) => { const made: Level = { id: nextId("l"), ...input }; s.levels.push(made); return made; });
  c.updateLevelAttendanceMode.mockImplementation(async (_, id, attendanceMode) => ({ ...(s.levels.find((l) => l.id === id) ?? PRIMARIA), attendanceMode }));
  c.updateLevelGrading.mockImplementation(async (_, id, grading) => ({ ...(s.levels.find((l) => l.id === id) ?? PRIMARIA), grading }));
  c.createGradeLevel.mockImplementation(async (_, input) => { const made: GradeLevel = { id: nextId("g"), ...input }; s.grades.push(made); return made; });
  c.createTerm.mockImplementation(async (_, input) => { const made: Term = { id: nextId("t"), months: [], ...input }; s.terms.push(made); return made; });
  c.setTermMonths.mockImplementation(async (_, id, months) => {
    const saved: Term = { ...(s.terms.find((t) => t.id === id) ?? T1), months: [...months] };
    s.terms = s.terms.map((t) => (t.id === id ? saved : t));
    return saved;
  });
  c.copyTermMonths.mockImplementation(async () => []);
  c.createGroup.mockImplementation(async (_, input) => { const made: Group = { id: nextId("gr"), ...input }; s.groups.push(made); return made; });
  c.createSubjectInGrades.mockImplementation(async (_, name, ids) => ({ created: ids.map((gradeLevelId) => ({ id: nextId("s"), gradeLevelId, name })), existing: [], failed: [] }));
  c.createSubject.mockImplementation(async (_, input) => ({ id: nextId("s"), ...input }));
  c.renameSubject.mockImplementation(async (_, id, name) => ({ ...(s.subjects.find((x) => x.id === id) ?? MATH), name }));
  c.deleteSubject.mockResolvedValue(undefined);
  c.createNonWorkingPeriod.mockImplementation(async (_, input) => { const made: NonWorkingPeriod = { id: nextId("d"), endsOn: input.startsOn, ...input }; s.closedDays.push(made); return made; });
  c.deleteNonWorkingPeriod.mockImplementation(async (_, id) => { s.closedDays = s.closedDays.filter((d) => d.id !== id); });
  c.createSchoolEvent.mockImplementation(async (_, input) => { const made: SchoolEvent = { id: nextId("e"), description: "", endsOn: input.startsOn, ...input }; s.events.push(made); return made; });
  c.deleteSchoolEvent.mockResolvedValue(undefined);
  c.saveSchoolProfile.mockImplementation(async (_, profile) => { s.profileName = profile.name; return profile; });
  c.restoreLevel.mockResolvedValue({ restored: {}, skipped: [] });
  installCorrections(s);
  f.upsertTuitionPlan.mockImplementation(async (_, plan) => ({ ...PLAN, ...plan }));
  f.runBilling.mockResolvedValue(RUN);
}

/** T8: rename/delete grades and groups, and the levels' order. Writes change `s`. */
function installCorrections(s: FakeSchool): void {
  const c = vi.mocked(configApi);
  // Only the files that mock these five get them answered; the others keep the real ones.
  if (!vi.isMockFunction(c.reorderLevels)) return;
  c.reorderLevels.mockImplementation(async (_, ids) => {
    s.levels = ids.map((id, i) => ({ ...(s.levels.find((l) => l.id === id) ?? PRIMARIA), sortOrder: i + 1 }));
    return [...s.levels];
  });
  c.renameGradeLevel.mockImplementation(async (_, id, name) => {
    const saved: GradeLevel = { ...(s.grades.find((g) => g.id === id) ?? G1), name };
    s.grades = s.grades.map((g) => (g.id === id ? saved : g));
    return saved;
  });
  c.deleteGradeLevel.mockImplementation(async (_, id) => { s.grades = s.grades.filter((g) => g.id !== id); });
  c.renameGroup.mockImplementation(async (_, id, name) => {
    const saved: Group = { ...(s.groups.find((g) => g.id === id) ?? GROUP_A), name };
    s.groups = s.groups.map((g) => (g.id === id ? saved : g));
    return saved;
  });
  c.deleteGroup.mockImplementation(async (_, id) => { s.groups = s.groups.filter((g) => g.id !== id); });
}

/** Answers every config and finance call from `s`; writes change `s`, so a later read sees them. */
export function installSchool(s: FakeSchool): FakeSchool {
  installReads(s);
  installWrites(s);
  return s;
}

export type User = ReturnType<typeof userEvent.setup>;

export function renderSetup(props: Partial<CatalogProps> = {}): { user: User; props: CatalogProps } {
  // The owner by default: every step writable and the prices readable (CC-311 tests pass less).
  const all: CatalogProps = { onBack: vi.fn(), canWriteConfig: true, canReadTuition: true, ...props };
  const user = userEvent.setup();
  render(<CatalogSetup {...all} />);
  return { user, props: all };
}

export const stepsNav = (): HTMLElement => screen.getByRole("navigation", { name: /Puesta en marcha|Configuración/ });
export const stepButton = (title: string): HTMLElement => within(stepsNav()).getByRole("button", { name: new RegExp(`^${title}`) });

/** Every status read has answered: no step still says "Revisando…". */
export async function settled(): Promise<void> {
  await screen.findByRole("navigation", { name: /Puesta en marcha|Configuración/ });
  await waitFor(() => expect(stepsNav()).not.toHaveTextContent("Revisando…"));
  await waitFor(() => expect(document.querySelector("[aria-current]")).not.toBeNull());
}

export async function openStep(user: User, title: string): Promise<HTMLElement> {
  await user.click(stepButton(title));
  return stepRegion(title);
}

/** The open step's frame, found through its button's aria-current and the frame's data-step. */
export function stepRegion(title: string): HTMLElement {
  const id = STEP_IDS[title];
  const region = document.querySelector<HTMLElement>(`[data-step="${id}"]`);
  if (region === null) throw new Error(`no frame for ${title}`);
  return region;
}

const STEP_IDS: Record<string, string> = {
  "Datos del colegio": "profile",
  "Ciclo escolar": "cycle",
  "Niveles y grados": "levels",
  "Estructura de la escuela": "levels",
  Periodos: "terms",
  Materias: "subjects",
  Grupos: "groups",
  Colegiaturas: "tuition",
  Calendario: "calendar",
  Papelera: "trash",
};

/** A Btn's look is its inline background: lime is the only `var(--primary)` one. */
export const isLime = (el: HTMLElement): boolean => el.style.background === "var(--primary)";
