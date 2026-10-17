import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as api from "../../config/api";
import type { Cycle, Level } from "../../config/api";
import { useCatalogState } from "./useCatalogState";

vi.mock("../../config/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../config/api")>()),
  listCycles: vi.fn(), listLevels: vi.fn(), listGradeLevels: vi.fn(), listGroups: vi.fn(), listTerms: vi.fn(), createLevel: vi.fn(),
}));

const mocked = vi.mocked(api);
const CYCLE: Cycle = { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-24", endsOn: "2027-07-09" };
const PRIMARIA: Level = { id: "l1", name: "PRIMARIA", sepCode: "", enrollmentGranularity: "cycle" };
const SECUNDARIA: Level = { id: "l2", name: "SECUNDARIA", sepCode: "", enrollmentGranularity: "cycle" };

beforeEach(() => {
  mocked.listCycles.mockResolvedValue([CYCLE]);
  mocked.listGradeLevels.mockResolvedValue([]);
  mocked.listGroups.mockResolvedValue([]);
  mocked.listTerms.mockResolvedValue([]);
});
afterEach(() => vi.clearAllMocks());

async function boot() {
  const view = renderHook(() => useCatalogState(undefined, true));
  await waitFor(() => expect(view.result.current.load.loading).toBe(false));
  return view;
}

// ISSUE-T14-023: with one level, Materias and Grupos made you click it in every step.
describe("useCatalogState · a school with a single level", () => {
  it("has it chosen already, and loads its grades", async () => {
    mocked.listLevels.mockResolvedValue([PRIMARIA]);
    const view = await boot();

    await waitFor(() => expect(view.result.current.levels.selectedId).toBe("l1"));
    expect(mocked.listGradeLevels).toHaveBeenCalledWith(expect.any(String), "l1");
  });

  it("chooses nothing for the school when it has several", async () => {
    mocked.listLevels.mockResolvedValue([PRIMARIA, SECUNDARIA]);
    const view = await boot();

    expect(view.result.current.levels.selectedId).toBeUndefined();
  });

  it("chooses the level the moment the first one is created", async () => {
    mocked.listLevels.mockResolvedValueOnce([]).mockResolvedValue([PRIMARIA]);
    mocked.createLevel.mockResolvedValue(PRIMARIA);
    const view = await boot();
    expect(view.result.current.levels.selectedId).toBeUndefined();

    await act(() => view.result.current.levels.create({ name: "PRIMARIA", sepCode: "", enrollmentGranularity: "cycle" }));

    await waitFor(() => expect(view.result.current.levels.selectedId).toBe("l1"));
  });
});
