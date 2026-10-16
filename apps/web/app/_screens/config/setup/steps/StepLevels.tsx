"use client";

import type { ReactElement } from "react";

import { GradeLevelsPanel } from "../../GradeLevelsPanel";
import { LevelSettings, LevelsPanel } from "../../LevelsPanel";
import type { Group } from "../../../../config/api";
import type { GradesSlice, LevelsSlice } from "../../useCatalogState";
import type { StatusRead } from "../setupSteps";
import type { SetupActions } from "../useSetupActions";
import { StepFrame, type StepSlot } from "../StepFrame";
import { GradeGroups } from "./GradeGroups";

export interface StepLevelsProps {
  slot: StepSlot;
  levels: LevelsSlice;
  grades: GradesSlice;
  /** Every group of the cycle (the status read): the picked grade's groups. */
  cycleGroups: StatusRead<Group[]>;
  actions: SetupActions;
  /** config:write: reorder levels (T8). Grade corrections come in `actions`. */
  canWrite?: boolean;
  /** Given only to the owner (levels:delete), exactly as the old view does (A37). */
  onDeleteLevel?: (levelId: string) => void;
}

/**
 * Step 3 · Niveles y grados. The step's own work first — the level chips with
 * Nuevo nivel, and the selected level's grades — then that level's settings,
 * which are edited far less often (N6). The old view keeps them under the chips.
 */
export function StepLevels({ slot, levels, grades, cycleGroups, actions, canWrite = false, onDeleteLevel }: StepLevelsProps): ReactElement {
  const selected = levels.selected;
  return (
    <StepFrame {...slot}>
      <div className="setup-pair">
        <LevelsPanel
          levels={levels.list}
          selectedLevelId={levels.selectedId}
          onSelect={levels.select}
          onCreate={actions.createLevel}
          onSetAttendanceMode={levels.setAttendanceMode}
          onSetGrading={levels.setGrading}
          onDelete={onDeleteLevel}
          onReorder={canWrite ? levels.reorder : undefined}
          showLevelSettings={false}
        />
        {selected !== undefined && (
          <div>
            <GradeLevelsPanel key={selected.id} level={selected} gradeLevels={grades.list} selectedGradeLevelId={grades.selectedId} onSelect={grades.select} onCreate={actions.createGrade} corrections={actions.gradeCorrections} />
            {grades.selected !== undefined && <GradeGroups grade={grades.selected} cycleGroups={cycleGroups} onGo={slot.onGo} />}
          </div>
        )}
        {selected !== undefined && (
          <div className="setup-pair-full">
            <LevelSettings level={selected} onSetAttendanceMode={levels.setAttendanceMode} onSetGrading={levels.setGrading} onDelete={onDeleteLevel} />
          </div>
        )}
      </div>
    </StepFrame>
  );
}
