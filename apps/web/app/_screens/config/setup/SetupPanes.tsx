"use client";

import type { ReactElement } from "react";

import { LevelTrashPanel } from "../levelDeletion/LevelTrashPanel";
import type { OpenTarget } from "./SetupStepList";
import { StepFrame, type StepSlot } from "./StepFrame";
import type { StepId, StepView } from "./setupSteps";
import { StepCalendar } from "./steps/StepCalendar";
import { StepCycle } from "./steps/StepCycle";
import { StepGroups } from "./steps/StepGroups";
import { StepLevels } from "./steps/StepLevels";
import { StepProfile } from "./steps/StepProfile";
import { StepSubjects } from "./steps/StepSubjects";
import { StepTerms } from "./steps/StepTerms";
import { StepTuition } from "./steps/StepTuition";
import type { CatalogState } from "../useCatalogState";
import type { SetupActions } from "./useSetupActions";
import type { SetupStatus } from "./useSetupStatus";

export interface SetupPanesProps {
  steps: readonly StepView[];
  openId: OpenTarget | undefined;
  onOpen: (id: OpenTarget) => void;
  state: CatalogState;
  status: SetupStatus;
  actions: SetupActions;
  canDeleteLevels: boolean;
  onDeleteLevel?: (levelId: string) => void;
  onOpenLevelPeriods?: (levelId: string) => void;
}

/**
 * The right column: EVERY step, mounted, the closed ones `hidden` (§3.1) — so
 * drafts, reads and undo windows behave exactly as on the old long page. While
 * prices take the screen the other steps unmount, as they do today, and each
 * child keeps a fixed slot (`{cond && …}`), so TuitionPanel never moves (§3.3).
 */
export function SetupPanes({ steps, openId, onOpen, state, status, actions, canDeleteLevels, onDeleteLevel, onOpenLevelPeriods }: SetupPanesProps): ReactElement {
  const pricing = state.pricing.active;
  const cycle = state.cycles.selected;
  const slot = (id: StepId): StepSlot => ({ step: stepOf(steps, id), hidden: openId !== id, onGo: onOpen });
  return (
    <>
      {!pricing && <StepProfile slot={slot("profile")} onLoaded={status.onProfileLoaded} />}
      {!pricing && <StepCycle slot={slot("cycle")} cycles={state.cycles} actions={actions} />}
      {!pricing && <StepLevels slot={slot("levels")} levels={state.levels} grades={state.grades} cycleGroups={status.groups} actions={actions} onDeleteLevel={canDeleteLevels ? onDeleteLevel : undefined} />}
      {!pricing && <StepTerms slot={slot("terms")} cycle={cycle} levels={state.levels} terms={state.terms} actions={actions} onOpenLevelPeriods={onOpenLevelPeriods} />}
      {!pricing && <StepSubjects slot={slot("subjects")} cycle={cycle} levels={state.levels} grades={state.grades} />}
      {!pricing && <StepGroups slot={slot("groups")} cycle={cycle} levels={state.levels} grades={state.grades} groups={state.groups} cycleGroups={status.groups} actions={actions} />}
      <StepTuition slot={slot("tuition")} cycles={state.cycles} levels={state.levels.list} pricing={state.pricing} onPlansLoaded={status.onPlansLoaded} />
      {!pricing && <StepCalendar slot={slot("calendar")} cycle={cycle} onLoaded={status.onCalendarLoaded} />}
      {!pricing && canDeleteLevels && (
        <StepFrame tool hidden={openId !== "trash"} onGo={onOpen}>
          <LevelTrashPanel onRestored={actions.restoredLevel} />
        </StepFrame>
      )}
    </>
  );
}

function stepOf(steps: readonly StepView[], id: StepId): StepView {
  const found = steps.find((s) => s.id === id);
  if (found === undefined) throw new Error(`setup step "${id}" is not in STEPS`);
  return found;
}
