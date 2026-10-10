import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as api from "../../config/api";
import * as financeApi from "../../finance/api";
import { Catalog } from "./Catalog";

/*
 * The old view's traffic is today's traffic (PLAN-redesign-configuracion §7.6).
 * The setup view adds three UNFILTERED reads to mark its steps; the old view,
 * which never mounts that view, must never make one: every grade read names its
 * level, every term read its level, every group read its grade.
 */

vi.mock("../../config/api", () => ({
  listCycles: vi.fn(),
  createCycle: vi.fn(),
  activateCycle: vi.fn(),
  listLevels: vi.fn(),
  createLevel: vi.fn(),
  updateLevelAttendanceMode: vi.fn(),
  updateLevelGrading: vi.fn(),
  listGradeLevels: vi.fn(),
  createGradeLevel: vi.fn(),
  listSubjects: vi.fn(),
  createSubject: vi.fn(),
  listGroups: vi.fn(),
  createGroup: vi.fn(),
  listTerms: vi.fn(),
  createTerm: vi.fn(),
  setTermMonths: vi.fn(),
  copyTermMonths: vi.fn(),
  listNonWorkingPeriods: vi.fn(),
  createNonWorkingPeriod: vi.fn(),
  deleteNonWorkingPeriod: vi.fn(),
  listSchoolEvents: vi.fn(),
  createSchoolEvent: vi.fn(),
  deleteSchoolEvent: vi.fn(),
  getSchoolProfile: vi.fn(),
  saveSchoolProfile: vi.fn(),
  listLevelTrash: vi.fn(),
  restoreLevel: vi.fn(),
  ConfigError: class ConfigError extends Error {},
}));

vi.mock("../../finance/api", () => ({
  listTuitionPlans: vi.fn(),
  upsertTuitionPlan: vi.fn(),
  runBilling: vi.fn(),
}));

const mocked = vi.mocked(api);
const finance = vi.mocked(financeApi);

const ACTIVE = { id: "c1", name: "2026-2027", status: "active" as const, startsOn: "2026-08-01", endsOn: "2027-06-30" };
const DRAFT = { id: "c2", name: "2027-2028", status: "draft" as const, startsOn: "2027-08-01", endsOn: "2028-06-30" };
const PRIMARIA = { id: "l1", name: "PRIMARIA", sepCode: "", enrollmentGranularity: "cycle" as const };
const SECUNDARIA = { id: "l2", name: "SECUNDARIA", sepCode: "", enrollmentGranularity: "cycle" as const };
const GRADES_BY_LEVEL: Record<string, api.GradeLevel[]> = {
  l1: [
    { id: "g1", levelId: "l1", name: "1° PRIMARIA", ordinal: 1 },
    { id: "g2", levelId: "l1", name: "2° PRIMARIA", ordinal: 2 },
  ],
  l2: [{ id: "g9", levelId: "l2", name: "1° SECUNDARIA", ordinal: 1 }],
};

beforeEach(() => {
  mocked.listCycles.mockResolvedValue([]);
  mocked.listLevels.mockResolvedValue([]);
  mocked.listGradeLevels.mockImplementation(async (_: string, levelId?: string) => GRADES_BY_LEVEL[levelId ?? ""] ?? []);
  mocked.listNonWorkingPeriods.mockResolvedValue([]);
  mocked.listSchoolEvents.mockResolvedValue([]);
  mocked.getSchoolProfile.mockResolvedValue({
    profile: { name: "", legalName: "", rfc: "", cct: "", address: "", phone: "", legend: "" },
    found: false,
  });
  mocked.listSubjects.mockResolvedValue([]);
  mocked.listGroups.mockResolvedValue([]);
  mocked.listTerms.mockResolvedValue([]);
  mocked.listLevelTrash.mockResolvedValue([]);
  finance.listTuitionPlans.mockResolvedValue([]);
});

afterEach(() => vi.clearAllMocks());

const levelChip = (name: string): HTMLElement => screen.getByRole("button", { name: new RegExp(`^${name} · `) });

describe("Catalog (old view) — no unfiltered status reads", () => {
  it("never lists grades without a level, nor terms or groups without their filter", async () => {
    mocked.listCycles.mockResolvedValue([ACTIVE, DRAFT]);
    mocked.listLevels.mockResolvedValue([PRIMARIA, SECUNDARIA]);
    const user = userEvent.setup();
    render(<Catalog onBack={() => {}} canDeleteLevels onDeleteLevel={() => {}} onOpenLevelPeriods={() => {}} />);

    await user.click(await screen.findByRole("button", { name: /^PRIMARIA · / }));
    await user.click(await screen.findByRole("button", { name: "1. 1° PRIMARIA" }));
    await screen.findByRole("heading", { name: "Grupos" });
    await user.click(levelChip("SECUNDARIA"));
    await user.click(await screen.findByRole("button", { name: "1. 1° SECUNDARIA" }));
    await user.click(screen.getByRole("button", { name: "2027-2028 · Borrador" }));
    await waitFor(() => expect(mocked.listGroups).toHaveBeenCalledWith(expect.any(String), "c2", "g9"));
    await waitFor(() => expect(mocked.listTerms).toHaveBeenCalledWith(expect.any(String), "c2", "l2"));

    expect(mocked.listGradeLevels.mock.calls.length).toBeGreaterThan(0);
    expect(mocked.listGradeLevels.mock.calls.filter((args) => args[1] === undefined)).toEqual([]);
    expect(mocked.listTerms.mock.calls.filter((args) => args[2] === undefined)).toEqual([]);
    expect(mocked.listGroups.mock.calls.filter((args) => args[2] === undefined)).toEqual([]);
  });
});
