import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { NONE, type FilterCatalog, type FilterableStudent } from "./studentFilter";
import { SEARCH_DEBOUNCE_MS, useStudentFilter } from "./useStudentFilter";

const catalog: FilterCatalog = {
  levels: [{ id: "pre", name: "Preescolar" }, { id: "pri", name: "Primaria" }],
  grades: [{ id: "pre-1", levelId: "pre", name: "1°", ordinal: 1 }],
  groups: [{ id: "pre-1-a", gradeLevelId: "pre-1", name: "A" }, { id: "pre-1-b", gradeLevelId: "pre-1", name: "B" }],
};

const kid = (id: string, lastName: string, levelId: string, gradeLevelId = "", groupId = ""): FilterableStudent =>
  ({ id, lastName, firstName: "X", levelId, gradeLevelId, groupId });

const roll = [
  kid("1", "Arce", "pre", "pre-1", "pre-1-a"),
  kid("2", "Bello", "pre", "pre-1", "pre-1-b"),
  kid("3", "Cano", "pre", "pre-1", "pre-1-b"),
  kid("4", "Duarte", "pri"),
];

const ids = (list: readonly FilterableStudent[]): string[] => list.map((s) => s.id);

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe("useStudentFilter", () => {
  it("starts from the given filter (a URL read back)", () => {
    const { result } = renderHook(() => useStudentFilter(roll, catalog, { levelId: "pre", gradeId: "pre-1", groupId: "pre-1-b", q: "" }));
    expect(ids(result.current.filtered)).toEqual(["2", "3"]);
    expect(result.current.active).toBe(true);
  });

  it("chains the chips: 1°B preescolar in three choices", () => {
    const { result } = renderHook(() => useStudentFilter(roll, catalog));
    act(() => result.current.chooseLevel("pre"));
    expect(result.current.facets.grades.map((g) => g.id)).toEqual(["pre-1"]);
    act(() => result.current.chooseGrade("pre-1"));
    expect(result.current.facets.groups.map((g) => g.label)).toEqual(["A", "B"]);
    act(() => result.current.chooseGroup("pre-1-b"));
    expect(ids(result.current.filtered)).toEqual(["2", "3"]);
  });

  it("applies the search a moment after typing stops", () => {
    const { result } = renderHook(() => useStudentFilter(roll, catalog));
    act(() => result.current.setQuery("cano"));
    expect(result.current.state.q).toBe("cano");
    expect(result.current.filtered).toHaveLength(4);
    act(() => { vi.advanceTimersByTime(SEARCH_DEBOUNCE_MS); });
    expect(ids(result.current.filtered)).toEqual(["3"]);
  });

  it("'select all' selects exactly the filtered pupils, and a second press unselects them", () => {
    const { result } = renderHook(() => useStudentFilter(roll, catalog));
    act(() => result.current.toggle("4"));
    act(() => result.current.chooseLevel("pre"));
    act(() => result.current.selectAll());
    expect([...result.current.selected].sort()).toEqual(["1", "2", "3"]);
    expect(result.current.allSelected).toBe(true);
    expect(ids(result.current.selectedItems)).toEqual(["1", "2", "3"]);

    act(() => result.current.selectAll());
    expect(result.current.selected.size).toBe(0);
  });

  it("drops from the selection whoever the filter hides", () => {
    const { result } = renderHook(() => useStudentFilter(roll, catalog));
    act(() => result.current.selectAll());
    act(() => result.current.chooseLevel("pri"));
    expect([...result.current.selected]).toEqual(["4"]);
  });

  it("toggles one pupil in and out", () => {
    const { result } = renderHook(() => useStudentFilter(roll, catalog));
    act(() => result.current.toggle("2"));
    expect(result.current.selected.has("2")).toBe(true);
    expect(result.current.allSelected).toBe(false);
    act(() => result.current.toggle("2"));
    expect(result.current.selected.has("2")).toBe(false);
  });

  it("'Limpiar' forgets chips and search at once", () => {
    const { result } = renderHook(() => useStudentFilter(roll, catalog, { levelId: NONE, gradeId: "", groupId: "", q: "zz" }));
    act(() => result.current.clear());
    expect(result.current.active).toBe(false);
    expect(result.current.filtered).toHaveLength(4);
  });

  it("is never 'all selected' over an empty list", () => {
    const { result } = renderHook(() => useStudentFilter([], catalog));
    expect(result.current.allSelected).toBe(false);
  });
});
