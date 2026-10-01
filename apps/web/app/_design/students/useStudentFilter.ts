"use client";

import { useCallback, useEffect, useMemo, useState } from "react";

import {
  EMPTY_FILTER,
  applyStudentFilter,
  chooseGrade,
  chooseGroup,
  chooseLevel,
  isFilterActive,
  studentFacets,
  type FilterCatalog,
  type FilterableStudent,
  type StudentFacets,
  type StudentFilterState,
} from "./studentFilter";

/** How long the search waits after the last key before it narrows the list. */
export const SEARCH_DEBOUNCE_MS = 150;

/** Everything a screen needs to narrow a list of pupils and pick from it. */
export interface StudentFilter<T extends FilterableStudent> {
  /** The chips chosen and the search as typed (what goes in the URL). */
  state: StudentFilterState;
  /** The pupils the filter keeps, surname first. */
  filtered: T[];
  facets: StudentFacets;
  /** Whether "Limpiar" has anything to clear. */
  active: boolean;
  setQuery: (q: string) => void;
  chooseLevel: (id: string) => void;
  chooseGrade: (id: string) => void;
  chooseGroup: (id: string) => void;
  clear: () => void;
  /** Ids picked; always a subset of `filtered`. */
  selected: ReadonlySet<string>;
  selectedItems: T[];
  /** Every filtered pupil is picked (and there is at least one). */
  allSelected: boolean;
  toggle: (id: string) => void;
  /** Picks exactly the filtered pupils; when they already are, unpicks them. */
  selectAll: () => void;
  clearSelection: () => void;
}

/**
 * The state of `StudentFilters` (T4), reusable wherever pupils are picked.
 * It knows nothing of the address bar: a screen that wants the URL to keep the
 * view reads `state` and writes it there.
 */
export function useStudentFilter<T extends FilterableStudent>(
  items: readonly T[],
  catalog: FilterCatalog,
  initial: StudentFilterState = EMPTY_FILTER,
): StudentFilter<T> {
  const [state, setState] = useState<StudentFilterState>(initial);
  const [appliedQ, setAppliedQ] = useState(initial.q);
  const [selected, setSelected] = useState<ReadonlySet<string>>(() => new Set());

  useEffect(() => {
    const t = setTimeout(() => setAppliedQ(state.q), SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(t);
  }, [state.q]);

  const applied = useMemo(() => ({ ...state, q: appliedQ }), [state, appliedQ]);
  const filtered = useMemo(() => applyStudentFilter(items, applied), [items, applied]);
  const facets = useMemo(() => studentFacets(items, catalog, applied), [items, catalog, applied]);

  // Whoever the filter hides leaves the selection: an action on "the selected"
  // must never reach a pupil the office cannot see.
  useEffect(() => {
    const visible = new Set(filtered.map((s) => s.id));
    setSelected((prev) => {
      const kept = [...prev].filter((id) => visible.has(id));
      return kept.length === prev.size ? prev : new Set(kept);
    });
  }, [filtered]);

  const selectedItems = useMemo(() => filtered.filter((s) => selected.has(s.id)), [filtered, selected]);
  const allSelected = filtered.length > 0 && selectedItems.length === filtered.length;

  const toggle = useCallback((id: string): void => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const selectAll = useCallback((): void => {
    setSelected(allSelected ? new Set() : new Set(filtered.map((s) => s.id)));
  }, [allSelected, filtered]);

  return {
    state,
    filtered,
    facets,
    active: isFilterActive(state),
    setQuery: useCallback((q: string) => setState((f) => ({ ...f, q })), []),
    chooseLevel: useCallback((id: string) => setState((f) => chooseLevel(f, id)), []),
    chooseGrade: useCallback((id: string) => setState((f) => chooseGrade(f, id)), []),
    chooseGroup: useCallback((id: string) => setState((f) => chooseGroup(f, id)), []),
    clear: useCallback(() => {
      setState(EMPTY_FILTER);
      setAppliedQ("");
    }, []),
    selected,
    selectedItems,
    allSelected,
    toggle,
    selectAll,
    clearSelection: useCallback(() => setSelected(new Set()), []),
  };
}
