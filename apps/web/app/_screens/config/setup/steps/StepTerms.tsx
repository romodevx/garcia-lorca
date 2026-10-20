"use client";

import { useEffect, useMemo, type ReactElement } from "react";

import type { Cycle, Level, Term } from "../../../../config/api";
import { cycleMonths } from "../../../../config/termMonths";
import { EmptyHint, ErrorAlert } from "../../parts";
import { LevelPeriodsEntry } from "../../periods/LevelPeriodsEntry";
import { TermsPanel } from "../../TermsPanel";
import { LevelGradeChips } from "../LevelGradeChips";
import type { LevelsSlice, TermsSlice } from "../../useCatalogState";
import type { SetupActions } from "../useSetupActions";
import { StepFrame, type StepSlot } from "../StepFrame";
import type { StatusRead } from "../setupSteps";
import { Prerequisite, needFor } from "./Prerequisite";
import { levelTermsLine, levelToOpen } from "./termsCoverage";

export interface StepTermsProps {
  slot: StepSlot;
  cycle: Cycle | undefined;
  levels: LevelsSlice;
  terms: TermsSlice;
  actions: SetupActions;
  onOpenLevelPeriods?: (levelId: string) => void;
  /**
   * module_school_structure: every term of the cycle. With it each level chip
   * says how far its periods go, and the open step picks the first level still
   * to do instead of asking for one. Absent: the step behaves as before.
   */
  cycleTerms?: StatusRead<Term[]>;
}

/** Step 4 · Periodos: the level's terms and, beside them, the way to each grade's periods (#8). */
export function StepTerms({ slot, cycle, levels, terms, actions, onOpenLevelPeriods, cycleTerms }: StepTermsProps): ReactElement {
  const need = needFor(cycle, levels.list);
  const level = levels.selected;
  const levelPill = useLevelCoverage(slot.hidden, cycle, levels, cycleTerms);
  // The level chips sit above the panel: the frame keeps the title on top either way.
  return (
    <StepFrame {...slot} ownHeading>
      {need !== undefined && <Prerequisite need={need} onGo={slot.onGo} />}
      {need === undefined && cycle !== undefined && (
        <>
          <LevelGradeChips levels={levels.list} selectedLevelId={levels.selectedId} onSelectLevel={levels.select} levelPill={levelPill} />
          {level === undefined ? <EmptyHint>Elige un nivel para ver sus periodos.</EmptyHint> : <LevelTerms cycle={cycle} level={level} levels={levels.list} terms={terms} actions={actions} />}
          {level !== undefined && onOpenLevelPeriods !== undefined && <LevelPeriodsEntry level={level} onOpen={onOpenLevelPeriods} />}
        </>
      )}
    </StepFrame>
  );
}

/** The chip line per level and, once the step is open with nothing picked, the level to start on. */
function useLevelCoverage(hidden: boolean, cycle: Cycle | undefined, levels: LevelsSlice, cycleTerms: StatusRead<Term[]> | undefined): ((levelId: string) => string | undefined) | undefined {
  const months = useMemo(() => (cycle === undefined ? [] : cycleMonths(cycle.startsOn, cycle.endsOn)), [cycle]);
  const ready = cycleTerms?.kind === "ready" ? cycleTerms.value : undefined;
  const { list, selectedId, select } = levels;
  useEffect(() => {
    if (hidden || selectedId !== undefined || ready === undefined) return;
    const id = levelToOpen(list, ready, months);
    if (id !== undefined) select(id);
  }, [hidden, selectedId, ready, list, months, select]);
  if (ready === undefined) return undefined;
  return (levelId) => levelTermsLine(ready.filter((t) => t.levelId === levelId), months);
}

function LevelTerms({ cycle, level, levels, terms, actions }: { cycle: Cycle; level: Level; levels: readonly Level[]; terms: TermsSlice; actions: SetupActions }): ReactElement | null {
  if (terms.load.kind === "loading") return <EmptyHint>Cargando periodos…</EmptyHint>;
  if (terms.load.kind === "failed") return <ErrorAlert message="No se pudieron cargar los periodos de este nivel." />;
  if (terms.load.kind !== "ready") return null;
  return (
    <TermsPanel
      key={`${cycle.id}:${level.id}`}
      cycle={cycle}
      level={level}
      levels={levels}
      terms={terms.load.terms}
      onCreate={actions.createTerm}
      onSetMonths={actions.setMonths}
      onCopyMonths={actions.copyMonths}
    />
  );
}
