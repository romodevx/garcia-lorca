import { screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { OPEN_STEP_KEY } from "./useOpenStep";
import { freshSchool, installSchool, openStep, renderSetup, settled, stepsNav } from "./testHarness";

/*
 * module_school_structure ON (PLAN-redesign-estructura): «Estructura de la
 * escuela» is step 3 of 7 and «Grupos» is gone. With the flag OFF every other
 * CatalogSetup test keeps the eight steps.
 */

vi.mock("../../../config/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../../config/api")>()),
  listCycles: vi.fn(), createCycle: vi.fn(), activateCycle: vi.fn(), listLevels: vi.fn(), createLevel: vi.fn(),
  updateLevelAttendanceMode: vi.fn(), updateLevelGrading: vi.fn(), listGradeLevels: vi.fn(), createGradeLevel: vi.fn(),
  listSubjects: vi.fn(), createSubject: vi.fn(), createSubjectInGrades: vi.fn(), renameSubject: vi.fn(), deleteSubject: vi.fn(),
  listGroups: vi.fn(), createGroup: vi.fn(), listTerms: vi.fn(), createTerm: vi.fn(), setTermMonths: vi.fn(), copyTermMonths: vi.fn(),
  listNonWorkingPeriods: vi.fn(), createNonWorkingPeriod: vi.fn(), deleteNonWorkingPeriod: vi.fn(),
  listSchoolEvents: vi.fn(), createSchoolEvent: vi.fn(), deleteSchoolEvent: vi.fn(),
  getSchoolProfile: vi.fn(), saveSchoolProfile: vi.fn(), listLevelTrash: vi.fn(), restoreLevel: vi.fn(),
  reorderLevels: vi.fn(), renameGradeLevel: vi.fn(), deleteGradeLevel: vi.fn(), renameGroup: vi.fn(), deleteGroup: vi.fn(),
}));
vi.mock("../../../finance/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../../finance/api")>()),
  listTuitionPlans: vi.fn(), upsertTuitionPlan: vi.fn(), runBilling: vi.fn(),
}));

beforeEach(() => {
  window.sessionStorage.clear();
  installSchool(freshSchool());
});
afterEach(() => vi.clearAllMocks());

describe("CatalogSetup — Estructura de la escuela (flag ON)", () => {
  it("lists seven steps, with Estructura de la escuela third and no Grupos", async () => {
    renderSetup({ structureView: true });
    await settled();
    expect(within(stepsNav()).getByRole("button", { name: /^Estructura de la escuela/ })).toBeInTheDocument();
    expect(within(stepsNav()).queryByRole("button", { name: /^Grupos/ })).toBeNull();
  });

  it("numbers the merged step 3 of 7, and Periodos steps back to it", async () => {
    const { user } = renderSetup({ structureView: true });
    await settled();
    const step = await openStep(user, "Estructura de la escuela");
    expect(within(step).getByText("Paso 3 de 7")).toBeInTheDocument();
    const terms = await openStep(user, "Periodos");
    await user.click(within(terms).getByRole("button", { name: /Anterior: Estructura de la escuela/ }));
    await waitFor(() => expect(screen.getByRole("heading", { level: 2, name: "Estructura de la escuela" })).toBeVisible());
  });

  it("opens the merged step for a session that last had Grupos open", async () => {
    window.sessionStorage.setItem(OPEN_STEP_KEY, "groups");
    renderSetup({ structureView: true });
    await settled();
    expect(await screen.findByRole("heading", { level: 2, name: "Estructura de la escuela" })).toBeVisible();
  });
});
