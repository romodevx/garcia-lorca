"use client";

import { useMemo } from "react";

import type { CopyTermMonthsInput, CreateCycleInput, CreateGradeLevelInput, CreateGroupInput, CreateLevelInput, CreateTermInput, Term } from "../../../config/api";
import type { CatalogState } from "../useCatalogState";
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
  /** A tapped month: it can be the one that completes the step, or the one that undoes it. */
  setMonths: (termId: string, months: readonly string[]) => Promise<Term>;
  copyMonths: (input: CopyTermMonthsInput) => Promise<Term[]>;
  createGroup: (input: CreateGroupInput) => Promise<void>;
  /** A level came back from the Papelera. */
  restoredLevel: () => void;
  /** T8 corrections: each re-reads the status it changes. */
  renameGrade: (id: string, name: string) => Promise<void>;
  deleteGrade: (id: string) => Promise<void>;
  renameGroup: (id: string, name: string) => Promise<void>;
  deleteGroup: (id: string) => Promise<void>;
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
      // Creating selects the new cycle, and the status reads follow the selected cycle's
      // key on their own. A refresh here would still hold the OLD cycle: wasted reads.
      createCycle: cycles.create,
      // Grades do not depend on the cycle; its terms and groups are re-read.
      activateCycle: thenRefresh(cycles.activate, () => {
        refresh("terms");
        refresh("groups");
      }),
      createLevel: thenRefresh(createLevel, () => refresh("grades")),
      createGrade: thenRefresh(grades.create, () => refresh("grades")),
      createTerm: thenRefresh(terms.create, () => refresh("terms")),
      setMonths: thenRefresh(terms.setMonths, () => refresh("terms")),
      copyMonths: thenRefresh(terms.copyMonths, () => refresh("terms")),
      createGroup: thenRefresh(groups.create, () => refresh("groups")),
      renameGrade: thenRefresh(grades.rename, () => refresh("grades")),
      deleteGrade: thenRefresh(grades.remove, () => refresh("grades")),
      renameGroup: thenRefresh(groups.rename, () => refresh("groups")),
      deleteGroup: thenRefresh(groups.remove, () => refresh("groups")),
      // A restore brings the level back with its grades, terms and groups: every read is stale.
      restoredLevel: () => {
        reloadLevels();
        refresh("all");
      },
    }),
    [cycles.create, cycles.activate, createLevel, reloadLevels, grades.create, grades.rename, grades.remove, terms.create, terms.setMonths, terms.copyMonths, groups.create, groups.rename, groups.remove, refresh],
  );
}
