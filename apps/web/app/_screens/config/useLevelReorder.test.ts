import { act, renderHook } from "@testing-library/react";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as api from "../../config/api";
import { ConfigError, type Level } from "../../config/api";
import { useLevelReorder } from "./useLevelReorder";

vi.mock("../../config/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../config/api")>()),
  reorderLevels: vi.fn(),
}));

const reorderLevels = vi.mocked(api).reorderLevels;

const level = (id: string): Level => ({ id, name: id.toUpperCase(), sepCode: "", enrollmentGranularity: "cycle" });
const A = level("a");
const B = level("b");
const C = level("c");

interface Deferred {
  ids: readonly string[];
  resolve: (saved: Level[]) => void;
  reject: (cause: Error) => void;
}

/** Each request waits until the test answers it, in whatever order the test picks. */
let pending: Deferred[] = [];
beforeEach(() => {
  pending = [];
  reorderLevels.mockImplementation((_, ids) => new Promise<Level[]>((resolve, reject) => { pending.push({ ids, resolve, reject }); }));
});
afterEach(() => vi.clearAllMocks());

const byIds = (ids: string[]): Level[] => ids.map((id) => [A, B, C].find((l) => l.id === id) as Level);

function setup() {
  const view = renderHook(() => {
    const [levels, setLevels] = useState<Level[]>([]);
    const order = useLevelReorder(setLevels);
    return { levels, ...order };
  });
  act(() => view.result.current.adopt([A, B, C]));
  const ids = (): string[] => view.result.current.levels.map((l) => l.id);
  return { view, ids };
}

async function flush(): Promise<void> {
  await act(async () => { await Promise.resolve(); await Promise.resolve(); });
}

describe("useLevelReorder", () => {
  it("serializes quick moves: the second is not sent until the first is answered", async () => {
    const { view, ids } = setup();
    let first!: Promise<void>;
    let second!: Promise<void>;
    act(() => { first = view.result.current.reorder(["b", "a", "c"]); });
    act(() => { second = view.result.current.reorder(["b", "c", "a"]); });
    await flush();
    expect(ids()).toEqual(["b", "c", "a"]);
    expect(reorderLevels).toHaveBeenCalledTimes(1);

    // The first answer arrives after the second move: it must not pull the list back.
    await act(async () => { pending[0]?.resolve(byIds(["b", "a", "c"])); await first; });
    expect(ids()).toEqual(["b", "c", "a"]);
    expect(reorderLevels).toHaveBeenCalledTimes(2);
    expect(reorderLevels).toHaveBeenLastCalledWith(expect.any(String), ["b", "c", "a"]);

    await act(async () => { pending[1]?.resolve(byIds(["b", "c", "a"])); await second; });
    expect(ids()).toEqual(["b", "c", "a"]);
  });

  it("a refused move rolls back to the order the server last confirmed, not an optimistic one", async () => {
    const { view, ids } = setup();
    let first!: Promise<void>;
    act(() => { first = view.result.current.reorder(["b", "a", "c"]); });
    await flush();
    await act(async () => { pending[0]?.resolve(byIds(["b", "a", "c"])); await first; });

    let second!: Promise<void>;
    act(() => { second = view.result.current.reorder(["b", "c", "a"]); });
    let third!: Promise<void>;
    act(() => { third = view.result.current.reorder(["c", "b", "a"]); });
    await flush();
    const refusal = new ConfigError(400, "La lista no coincide.", "LEVEL_ORDER_MISMATCH");
    await act(async () => {
      pending[1]?.reject(refusal);
      await expect(second).rejects.toBe(refusal);
      await third;
    });
    expect(ids()).toEqual(["b", "a", "c"]);
    // The move built on the refused one is dropped, never sent.
    expect(reorderLevels).toHaveBeenCalledTimes(2);
  });

  it("a first move refused goes back to the order first loaded", async () => {
    const { view, ids } = setup();
    let first!: Promise<void>;
    act(() => { first = view.result.current.reorder(["a", "c", "b"]); });
    await flush();
    const refusal = new Error("network");
    await act(async () => {
      pending[0]?.reject(refusal);
      await expect(first).rejects.toBe(refusal);
    });
    expect(ids()).toEqual(["a", "b", "c"]);
  });
});
