"use client";

import type { ReactElement } from "react";

import type { Cycle } from "../../../../config/api";
import { EmptyHint } from "../../parts";
import { SubjectsPanel } from "../../SubjectsPanel";
import { LevelGradeChips } from "../LevelGradeChips";
import type { GradesSlice, LevelsSlice } from "../../useCatalogState";
import { StepFrame, type StepSlot } from "../StepFrame";
import { Prerequisite, needFor } from "./Prerequisite";

export interface StepSubjectsProps {
  slot: StepSlot;
  cycle: Cycle | undefined;
  levels: LevelsSlice;
  grades: GradesSlice;
  /** A subject changed: the step list re-reads its Materias count (ISSUE-CC-107). */
  onSubjectsChanged?: () => void;
}

/**
 * Step 5 · Materias: only the level chips go above the panel. The panel keeps
 * its own "En qué grados" picker, which decides where a subject is created (A49).
 */
export function StepSubjects({ slot, cycle, levels, grades, onSubjectsChanged }: StepSubjectsProps): ReactElement {
  const need = needFor(cycle, levels.list);
  const level = levels.selected;
  return (
    <StepFrame {...slot} ownHeading>
      {need !== undefined && <Prerequisite need={need} onGo={slot.onGo} />}
      {need === undefined && <LevelGradeChips levels={levels.list} selectedLevelId={levels.selectedId} onSelectLevel={levels.select} />}
      {need === undefined && level === undefined && <EmptyHint>Elige un nivel para ver sus materias.</EmptyHint>}
      {need === undefined && level !== undefined && (
        <SubjectsPanel key={`s:${level.id}`} level={level} gradeLevels={grades.list.filter((g) => g.levelId === level.id)} selectedGradeLevelId={grades.selectedId} onChanged={onSubjectsChanged} />
      )}
    </StepFrame>
  );
}
