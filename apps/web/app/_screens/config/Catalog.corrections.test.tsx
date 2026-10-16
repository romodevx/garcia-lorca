import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as api from "../../config/api";
import type { GradeLevel, Level } from "../../config/api";
import * as financeApi from "../../finance/api";
import { Catalog } from "./Catalog";

/*
 * T8 in the old view (module_config_setup OFF): the same panels, the same
 * config:write gate. Self-contained mocks: nothing here may import setup/.
 */

vi.mock("../../config/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../config/api")>()),
  listCycles: vi.fn(), listLevels: vi.fn(), listGradeLevels: vi.fn(), listSubjects: vi.fn(), listGroups: vi.fn(),
  listTerms: vi.fn(), listNonWorkingPeriods: vi.fn(), listSchoolEvents: vi.fn(), getSchoolProfile: vi.fn(),
  listLevelTrash: vi.fn(), reorderLevels: vi.fn(),
}));
vi.mock("../../finance/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../finance/api")>()),
  listTuitionPlans: vi.fn(),
}));

const mocked = vi.mocked(api);
const PRIMARIA: Level = { id: "l1", name: "PRIMARIA", sepCode: "", enrollmentGranularity: "cycle" };
const SECUNDARIA: Level = { id: "l2", name: "SECUNDARIA", sepCode: "", enrollmentGranularity: "cycle" };
const G1: GradeLevel = { id: "g1", levelId: "l1", name: "1° PRIMARIA", ordinal: 1 };
const G2: GradeLevel = { id: "g2", levelId: "l1", name: "2° PRIMARIA", ordinal: 2 };

// No cycle: the old page's heavy cycle panels stay out; levels and grades are what is under test.
beforeEach(() => {
  mocked.listCycles.mockResolvedValue([]);
  mocked.listLevels.mockResolvedValue([PRIMARIA, SECUNDARIA]);
  mocked.listGradeLevels.mockImplementation(async (_, levelId) => [G1, G2].filter((g) => levelId === undefined || g.levelId === levelId));
  mocked.listSubjects.mockResolvedValue([]);
  mocked.getSchoolProfile.mockResolvedValue({ profile: { name: "", legalName: "", rfc: "", cct: "", address: "", phone: "", legend: "" }, found: false });
  mocked.listLevelTrash.mockResolvedValue([]);
  mocked.reorderLevels.mockImplementation(async (_, ids) => ids.map((id) => (id === "l1" ? PRIMARIA : SECUNDARIA)));
  vi.mocked(financeApi).listTuitionPlans.mockResolvedValue([]);
});
afterEach(() => vi.clearAllMocks());

async function pickPrimaria(canWriteConfig: boolean): Promise<ReturnType<typeof userEvent.setup>> {
  const user = userEvent.setup();
  render(<Catalog onBack={vi.fn()} canWriteConfig={canWriteConfig} />);
  await user.click(await screen.findByRole("button", { name: "PRIMARIA · por ciclo" }));
  await screen.findByRole("button", { name: "1. 1° PRIMARIA" });
  return user;
}

describe("Catalog · T8 corrections", () => {
  it("config:write gets the level order and the grade's rename/delete", async () => {
    const user = await pickPrimaria(true);
    await user.click(screen.getByRole("button", { name: "Subir SECUNDARIA" }));
    await waitFor(() => expect(mocked.reorderLevels).toHaveBeenCalledWith(expect.any(String), ["l2", "l1"]));
    expect(screen.getByRole("button", { name: "Renombrar grado 1° PRIMARIA" })).toBeInTheDocument();
    expect(within(screen.getByRole("list", { name: "Grados de PRIMARIA" })).getByRole("button", { name: "Borrar grado 2° PRIMARIA" })).toBeInTheDocument();
  });

  it("without config:write, none of it", async () => {
    await pickPrimaria(false);
    expect(screen.queryByRole("button", { name: /^Subir|^Bajar|^Renombrar grado|^Borrar grado/ })).toBeNull();
  });
});
