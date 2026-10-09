"use client";

import type { ReactElement } from "react";

import type { Cycle, Level } from "../../../../config/api";
import { EmptyHint, ErrorAlert } from "../../parts";
import { LevelPeriodsEntry } from "../../periods/LevelPeriodsEntry";
import { TermsPanel } from "../../TermsPanel";
import { LevelGradeChips } from "../LevelGradeChips";
import type { LevelsSlice, TermsSlice } from "../useCatalogState";
import type { SetupActions } from "../useSetupActions";
import { StepFrame, type StepSlot } from "../StepFrame";
import { Prerequisite, needFor } from "./Prerequisite";

export interface StepTermsProps {
  slot: StepSlot;
  cycle: Cycle | undefined;
  levels: LevelsSlice;
  terms: TermsSlice;
  actions: SetupActions;
  onOpenLevelPeriods?: (levelId: string) => void;
}

/** Step 4 · Periodos: the level's terms and, beside them, the way to each grade's periods (#8). */
export function StepTerms({ slot, cycle, levels, terms, actions, onOpenLevelPeriods }: StepTermsProps): ReactElement {
  const need = needFor(cycle, levels.list);
  const level = levels.selected;
  // The level chips sit above the panel: the frame keeps the title on top either way.
  return (
    <StepFrame {...slot} ownHeading>
      {need !== undefined && <Prerequisite need={need} onGo={slot.onGo} />}
      {need === undefined && cycle !== undefined && (
        <>
          <LevelGradeChips levels={levels.list} selectedLevelId={levels.selectedId} onSelectLevel={levels.select} />
          {level === undefined ? <EmptyHint>Elige un nivel para ver sus periodos.</EmptyHint> : <LevelTerms cycle={cycle} level={level} levels={levels.list} terms={terms} actions={actions} />}
          {level !== undefined && onOpenLevelPeriods !== undefined && <LevelPeriodsEntry level={level} onOpen={onOpenLevelPeriods} />}
        </>
      )}
    </StepFrame>
  );
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
      onSetMonths={terms.setMonths}
      onCopyMonths={actions.copyMonths}
    />
  );
}
