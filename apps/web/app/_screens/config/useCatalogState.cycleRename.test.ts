import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as api from "../../config/api";
import type { Cycle } from "../../config/api";
import { useCatalogState } from "./useCatalogState";

vi.mock("../../config/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../config/api")>()),
  listCycles: vi.fn(), listLevels: vi.fn(), listTerms: vi.fn(), renameCycle: vi.fn(),
}));

const mocked = vi.mocked(api);
const DRAFT: Cycle = { id: "c2", name: "NUEVO CICLO", status: "draft", startsOn: "2027-08-23", endsOn: "2028-07-07" };

beforeEach(() => {
  mocked.listCycles.mockResolvedValue([DRAFT]);
  mocked.listLevels.mockResolvedValue([]);
  mocked.listTerms.mockResolvedValue([]);
});
afterEach(() => vi.clearAllMocks());

async function loaded(canWriteConfig: boolean) {
  const view = renderHook(() => useCatalogState(undefined, canWriteConfig));
  await waitFor(() => expect(view.result.current.load.loading).toBe(false));
  return view;
}

describe("useCatalogState · renaming a cycle (QA CC-103, config:write only)", () => {
  it("hands out no rename without config:write", async () => {
    const view = await loaded(false);
    expect(view.result.current.cycles.rename).toBeUndefined();
  });

  it("renames through the API and updates the list in place, without a reload", async () => {
    mocked.renameCycle.mockResolvedValue({ ...DRAFT, name: "2027-2028" });
    const view = await loaded(true);

    await act(() => view.result.current.cycles.rename?.("c2", "2027-2028") ?? Promise.resolve());

    expect(mocked.renameCycle).toHaveBeenCalledWith(expect.any(String), "c2", "2027-2028");
    expect(view.result.current.cycles.list.map((c) => c.name)).toEqual(["2027-2028"]);
    expect(mocked.listCycles).toHaveBeenCalledTimes(1);
  });
});
