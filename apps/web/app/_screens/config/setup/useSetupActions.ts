"use client";

import { useMemo } from "react";

import type { CopyTermMonthsInput, CreateCycleInput, CreateGradeLevelInput, CreateGroupInput, CreateLevelInput, CreateTermInput, Term } from "../../../config/api";
import type { CatalogState } from "./useCatalogState";
import type { RefreshKind } from "./useSetupStatus";

/**
 * The catalog's own handlers, each followed by the status read it invalidates
 * (PLAN §4.2). The call and its arguments are untouched: `.then` only adds the
 * refresh after the handler has done exactly what it does in the old view.
 */
export interface SetupActions {
  createCycle: (input: CreateCycleInput) => Promise<void>;
  activateCycle: (id: string) => Promise<void>;
  createLevel: (input: CreateLevelInput) => Promise<void>;
  createGrade: (input: CreateGradeLevelInput) => Promise<void>;
  createTerm: (input: CreateTermInput) => Promise<void>;
  copyMonths: (input: CopyTermMonthsInput) => Promise<Term[]>;
  createGroup: (input: CreateGroupInput) => Promise<void>;
  /** A level came back from the Papelera. */
  restoredLevel: () => void;
}

function thenRefresh<Args extends unknown[], Result>(action: (...args: Args) => Promise<Result>, refresh: () => void): (...args: Args) => Promise<Result> {
  return (...args) =>
    action(...args).then((result) => {
      refresh();
      return result;
    });
}

export function useSetupActions(state: CatalogState, refresh: (kind: RefreshKind) => void): SetupActions {
  const { cycles, grades, terms, groups } = state;
  const { create: createLevel, reload: reloadLevels } = state.levels;
  return useMemo(
    () => ({
      createCycle: thenRefresh(cycles.create, () => refresh("all")),
      activateCycle: thenRefresh(cycles.activate, () => refresh("all")),
      createLevel: thenRefresh(createLevel, () => refresh("grades")),
      createGrade: thenRefresh(grades.create, () => refresh("grades")),
      createTerm: thenRefresh(terms.create, () => refresh("terms")),
      copyMonths: thenRefresh(terms.copyMonths, () => refresh("terms")),
      createGroup: thenRefresh(groups.create, () => refresh("groups")),
      restoredLevel: () => {
        reloadLevels();
        refresh("grades");
      },
    }),
    [cycles.create, cycles.activate, createLevel, reloadLevels, grades.create, terms.create, terms.copyMonths, groups.create, refresh],
  );
}
