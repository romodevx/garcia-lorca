"use client";

import { useMemo } from "react";

import type { CopyTermMonthsInput, CreateCycleInput, CreateGradeLevelInput, CreateGroupInput, CreateLevelInput, CreateTermInput, GradeLevel, Group, Term } from "../../../config/api";
import type { NameCorrections } from "../CorrectableName";
import type { CatalogState } from "../useCatalogState";
import type { SetupStatus } from "./useSetupStatus";

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
  /** T8 corrections, each re-reading the status it changes. Undefined without config:write. */
  gradeCorrections: NameCorrections | undefined;
  groupCorrections: NameCorrections | undefined;
  /** What «Estructura de la escuela» writes with (PLAN-redesign-estructura). */
  structure: StructureActions;
}

/**
 * The tree's writes: the SAME handlers and calls, for any level or grade (never
 * through the shared selection, M1), each applied to the status read at once
 * (H1) and then confirmed by a refresh. A group rename refreshes even when
 * refused: GROUP_RELABEL_PENDING renamed it in config already (H3).
 */
export interface StructureActions {
  createGrade: (input: CreateGradeLevelInput) => Promise<void>;
  createGroup: (input: CreateGroupInput) => Promise<void>;
  gradeCorrections: NameCorrections | undefined;
  groupCorrections: NameCorrections | undefined;
}

type Patch<T> = (change: (list: T[]) => T[]) => void;

/** Corrections that patch the tree's read on success; `refreshOnRefusal` re-reads on a refusal too. */
function patched<T extends { id: string; name: string }>(corrections: NameCorrections | undefined, patch: Patch<T>, refresh: () => void, refreshOnRefusal: boolean): NameCorrections | undefined {
  if (corrections === undefined) return undefined;
  const settle = async (work: Promise<void>, change: (list: T[]) => T[]): Promise<void> => {
    try {
      await work;
    } catch (cause) {
      if (refreshOnRefusal) refresh();
      throw cause;
    }
    patch(change);
    refresh();
  };
  return {
    rename: (id, name) => settle(corrections.rename(id, name), (list) => list.map((x) => (x.id === id ? { ...x, name } : x))),
    remove: (id) => settle(corrections.remove(id), (list) => list.filter((x) => x.id !== id)),
  };
}

function withRefresh(corrections: NameCorrections | undefined, refresh: () => void): NameCorrections | undefined {
  if (corrections === undefined) return undefined;
  return { rename: thenRefresh(corrections.rename, refresh), remove: thenRefresh(corrections.remove, refresh) };
}

function thenRefresh<Args extends unknown[], Result>(action: (...args: Args) => Promise<Result>, refresh: () => void): (...args: Args) => Promise<Result> {
  return (...args) =>
    action(...args).then((result) => {
      refresh();
      return result;
    });
}

export function useSetupActions(state: CatalogState, status: Pick<SetupStatus, "refresh" | "patchGrades" | "patchGroups">): SetupActions {
  const { refresh, patchGrades, patchGroups } = status;
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
      gradeCorrections: withRefresh(grades.corrections, () => refresh("grades")),
      groupCorrections: withRefresh(groups.corrections, () => refresh("groups")),
      structure: {
        createGrade: async (input) => {
          const created: GradeLevel = await grades.createIn(input);
          patchGrades((list) => [...list, created]);
          refresh("grades");
        },
        createGroup: async (input) => {
          const created: Group = await groups.createIn(input);
          patchGroups((list) => [...list, created]);
          refresh("groups");
        },
        gradeCorrections: patched<GradeLevel>(grades.corrections, patchGrades, () => refresh("grades"), false),
        groupCorrections: patched<Group>(groups.corrections, patchGroups, () => refresh("groups"), true),
      },
      // A restore brings the level back with its grades, terms and groups: every read is stale.
      restoredLevel: () => {
        reloadLevels();
        refresh("all");
      },
    }),
    [cycles.create, cycles.activate, createLevel, reloadLevels, grades.create, grades.createIn, grades.corrections, terms.create, terms.setMonths, terms.copyMonths, groups.create, groups.createIn, groups.corrections, refresh, patchGrades, patchGroups],
  );
}
