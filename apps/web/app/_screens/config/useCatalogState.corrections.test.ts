import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as api from "../../config/api";
import { ConfigError, type Cycle, type GradeLevel, type Group, type Level } from "../../config/api";
import { useCatalogState } from "./useCatalogState";

vi.mock("../../config/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../config/api")>()),
  listCycles: vi.fn(), listLevels: vi.fn(), listGradeLevels: vi.fn(), listGroups: vi.fn(), listTerms: vi.fn(),
  renameGroup: vi.fn(), deleteGroup: vi.fn(), renameGradeLevel: vi.fn(), deleteGradeLevel: vi.fn(),
  createGradeLevel: vi.fn(), createGroup: vi.fn(),
}));

const mocked = vi.mocked(api);
const CYCLE: Cycle = { id: "c1", name: "2026-2027", status: "active", startsOn: "2026-08-24", endsOn: "2027-07-09" };
const PRIMARIA: Level = { id: "l1", name: "PRIMARIA", sepCode: "", enrollmentGranularity: "cycle" };
const GRADE: GradeLevel = { id: "g1", levelId: "l1", name: "1° PRIMARIA", ordinal: 1 };
const A: Group = { id: "gr1", cycleId: "c1", gradeLevelId: "g1", name: "A", capacity: 30 };

beforeEach(() => {
  mocked.listCycles.mockResolvedValue([CYCLE]);
  mocked.listLevels.mockResolvedValue([PRIMARIA]);
  mocked.listGradeLevels.mockResolvedValue([GRADE]);
  mocked.listGroups.mockResolvedValue([A]);
  mocked.listTerms.mockResolvedValue([]);
});
afterEach(() => vi.clearAllMocks());

async function withGroups(canWriteConfig: boolean) {
  const view = renderHook(() => useCatalogState(undefined, canWriteConfig));
  await waitFor(() => expect(view.result.current.load.loading).toBe(false));
  act(() => view.result.current.levels.select("l1"));
  await waitFor(() => expect(view.result.current.grades.list).toHaveLength(1));
  act(() => view.result.current.grades.select("g1"));
  await waitFor(() => expect(view.result.current.groups.list).toHaveLength(1));
  return view;
}

describe("useCatalogState · corrections (config:write only)", () => {
  it("grades and groups hand out no corrections without config:write", async () => {
    const view = await withGroups(false);
    expect(view.result.current.grades.corrections).toBeUndefined();
    expect(view.result.current.groups.corrections).toBeUndefined();
  });

  it("with config:write they carry rename and remove", async () => {
    const view = await withGroups(true);
    expect(view.result.current.grades.corrections).toEqual({ rename: expect.any(Function), remove: expect.any(Function) });
    expect(view.result.current.groups.corrections).toEqual({ rename: expect.any(Function), remove: expect.any(Function) });
  });
});

describe("useCatalogState · a refused group rename", () => {
  it("re-reads the groups (the rename may have half-happened) and still rejects", async () => {
    const view = await withGroups(true);
    const pending = new ConfigError(503, "Guarda de nuevo para terminar.", "GROUP_RELABEL_PENDING");
    mocked.renameGroup.mockRejectedValueOnce(pending);
    mocked.listGroups.mockResolvedValueOnce([{ ...A, name: "B" }]);
    const calls = mocked.listGroups.mock.calls.length;

    await act(async () => {
      await expect(view.result.current.groups.corrections?.rename("gr1", "B")).rejects.toBe(pending);
    });
    await waitFor(() => expect(view.result.current.groups.list[0]?.name).toBe("B"));
    expect(mocked.listGroups).toHaveBeenCalledTimes(calls + 1);
    expect(mocked.listGroups).toHaveBeenLastCalledWith(expect.any(String), "c1", "g1");
  });
});

// PLAN-redesign-estructura M1: the tree creates in any level or grade, never
// through the selection the other steps share.
describe("useCatalogState · creating outside the selection", () => {
  it("a grade created in another level is returned and kept out of the selected level's list", async () => {
    const view = await withGroups(true);
    const other: GradeLevel = { id: "g9", levelId: "l2", name: "1° SECUNDARIA", ordinal: 1 };
    mocked.createGradeLevel.mockResolvedValue(other);
    const created = await act(() => view.result.current.grades.createIn({ levelId: "l2", name: "1° SECUNDARIA", ordinal: 1 }));
    expect(created).toEqual(other);
    expect(view.result.current.levels.selectedId).toBe("l1");
    expect(view.result.current.grades.list.map((g) => g.id)).toEqual(["g1"]);
  });

  it("a group created in another grade is returned and kept out of the selected grade's list", async () => {
    const view = await withGroups(true);
    const other: Group = { id: "gr9", cycleId: "c1", gradeLevelId: "g2", name: "A", capacity: 30 };
    mocked.createGroup.mockResolvedValue(other);
    const created = await act(() => view.result.current.groups.createIn({ cycleId: "c1", gradeLevelId: "g2", name: "A", capacity: 30 }));
    expect(created).toEqual(other);
    expect(view.result.current.grades.selectedId).toBe("g1");
    expect(view.result.current.groups.list.map((g) => g.id)).toEqual(["gr1"]);
  });
});
