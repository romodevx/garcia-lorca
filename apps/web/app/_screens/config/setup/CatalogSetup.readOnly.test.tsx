import { screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as financeApi from "../../../finance/api";
import { freshSchool, installSchool, openStep, renderSetup, settled, stepsNav } from "./testHarness";

vi.mock("../../../config/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../../config/api")>()),
  listCycles: vi.fn(), createCycle: vi.fn(), activateCycle: vi.fn(), listLevels: vi.fn(), createLevel: vi.fn(),
  updateLevelAttendanceMode: vi.fn(), updateLevelGrading: vi.fn(), listGradeLevels: vi.fn(), createGradeLevel: vi.fn(),
  listSubjects: vi.fn(), createSubject: vi.fn(), createSubjectInGrades: vi.fn(), renameSubject: vi.fn(), deleteSubject: vi.fn(),
  listGroups: vi.fn(), createGroup: vi.fn(), listTerms: vi.fn(), createTerm: vi.fn(), setTermMonths: vi.fn(), copyTermMonths: vi.fn(),
  listNonWorkingPeriods: vi.fn(), createNonWorkingPeriod: vi.fn(), deleteNonWorkingPeriod: vi.fn(),
  listSchoolEvents: vi.fn(), createSchoolEvent: vi.fn(), deleteSchoolEvent: vi.fn(),
  getSchoolProfile: vi.fn(), saveSchoolProfile: vi.fn(), listLevelTrash: vi.fn(), restoreLevel: vi.fn(),
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

/** Secretaría: config:read only — no config:write, no finance:read (CC-311). */
const readOnly = { canWriteConfig: false, canReadTuition: false } as const;

describe("CatalogSetup — read-only role (CC-311)", () => {
  it("never asks for the tuition plans the role cannot read", async () => {
    renderSetup(readOnly);
    await settled();
    expect(vi.mocked(financeApi).listTuitionPlans).not.toHaveBeenCalled();
  });

  it("says plainly, in Colegiaturas, who sees the prices", async () => {
    const { user } = renderSetup(readOnly);
    await settled();
    const step = await openStep(user, "Colegiaturas");
    expect(within(step).getByText("Las colegiaturas las ve quien administra Finanzas.")).toBeInTheDocument();
  });

  it("offers no Nuevo nivel", async () => {
    const { user } = renderSetup(readOnly);
    await settled();
    const step = await openStep(user, "Niveles y grados");
    expect(within(step).queryByRole("button", { name: "Nuevo nivel" })).not.toBeInTheDocument();
  });

  it("titles the list Configuración, not Puesta en marcha: setting up is not this role's job", async () => {
    renderSetup(readOnly);
    await settled();
    expect(stepsNav()).toHaveAccessibleName("Configuración");
    expect(screen.queryByText("Puesta en marcha")).not.toBeInTheDocument();
  });

  it("still offers Nuevo nivel and reads the prices with config:write and finance:read", async () => {
    const { user } = renderSetup({ canWriteConfig: true, canReadTuition: true });
    await settled();
    expect(vi.mocked(financeApi).listTuitionPlans).toHaveBeenCalled();
    const step = await openStep(user, "Niveles y grados");
    expect(within(step).getByRole("button", { name: "Nuevo nivel" })).toBeInTheDocument();
  });
});
