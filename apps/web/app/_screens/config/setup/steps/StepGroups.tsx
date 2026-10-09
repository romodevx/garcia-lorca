"use client";

import type { ReactElement } from "react";

import type { Cycle, Group } from "../../../../config/api";
import { GroupsPanel } from "../../GroupsPanel";
import { EmptyHint } from "../../parts";
import { LevelGradeChips } from "../LevelGradeChips";
import type { StatusRead } from "../setupSteps";
import type { GradesSlice, GroupsSlice, LevelsSlice } from "../useCatalogState";
import type { SetupActions } from "../useSetupActions";
import { StepFrame, type StepSlot } from "../StepFrame";
import { Prerequisite, needFor } from "./Prerequisite";

export interface StepGroupsProps {
  slot: StepSlot;
  cycle: Cycle | undefined;
  levels: LevelsSlice;
  grades: GradesSlice;
  groups: GroupsSlice;
  /** Every group of the cycle (the status read): the "1 grupo" pills. */
  cycleGroups: StatusRead<Group[]>;
  actions: SetupActions;
}

/** "sin grupos" / "1 grupo" / "3 grupos", once the cycle's groups are known. */
function groupPill(groups: StatusRead<Group[]>): ((gradeId: string) => string) | undefined {
  if (groups.kind !== "ready") return undefined;
  return (gradeId) => {
    const count = groups.value.filter((g) => g.gradeLevelId === gradeId).length;
    if (count === 0) return "sin grupos";
    return count === 1 ? "1 grupo" : `${count} grupos`;
  };
}

/** Step 6 · Grupos: level and grade chips (with each grade's count) above the inline row. */
export function StepGroups({ slot, cycle, levels, grades, groups, cycleGroups, actions }: StepGroupsProps): ReactElement {
  const need = needFor(cycle, levels.list);
  const grade = grades.selected;
  const noGrades = levels.selected !== undefined && grades.list.length === 0;
  return (
    <StepFrame {...slot} ownHeading={need !== undefined || grade === undefined}>
      {need !== undefined && <Prerequisite need={need} onGo={slot.onGo} />}
      {need === undefined && (
        <LevelGradeChips
          levels={levels.list}
          selectedLevelId={levels.selectedId}
          onSelectLevel={levels.select}
          grades={grades.list}
          selectedGradeId={grades.selectedId}
          onSelectGrade={grades.select}
          gradePill={groupPill(cycleGroups)}
        />
      )}
      {need === undefined && noGrades && <Prerequisite need="grades" onGo={slot.onGo} />}
      {need === undefined && !noGrades && grade === undefined && <EmptyHint>Elige un grado para ver sus grupos.</EmptyHint>}
      {cycle !== undefined && grade !== undefined && (
        <GroupsPanel key={`${cycle.id}:${grade.id}`} cycle={cycle} gradeLevel={grade} groups={groups.list} onCreate={actions.createGroup} />
      )}
    </StepFrame>
  );
}
