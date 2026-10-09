"use client";

import type { ReactElement } from "react";

import type { Level } from "../../../../config/api";
import type { TuitionPlan } from "../../../../finance/api";
import { TuitionPanel } from "../../TuitionPanel";
import type { CyclesSlice, PricingSlice } from "../useCatalogState";
import { StepFrame, type StepSlot } from "../StepFrame";
import { Prerequisite } from "./Prerequisite";

export interface StepTuitionProps {
  slot: StepSlot;
  cycles: CyclesSlice;
  levels: readonly Level[];
  pricing: PricingSlice;
  onPlansLoaded: (cycleId: string, plans: TuitionPlan[] | "failed") => void;
}

/**
 * Step 7 · Colegiaturas. While prices are captured the frame drops its chrome
 * and nothing else is drawn, but TuitionPanel keeps its exact place in the tree:
 * a remount would open "Editar" blank and save defaults over the plan (§3.3).
 */
export function StepTuition({ slot, cycles, levels, pricing, onPlansLoaded }: StepTuitionProps): ReactElement {
  const cycle = cycles.selected;
  return (
    <StepFrame {...slot} hidden={slot.hidden && !pricing.active} chrome={!pricing.active} ownHeading={cycle === undefined}>
      {cycle === undefined && <Prerequisite need="cycle" onGo={slot.onGo} />}
      {cycle !== undefined && (
        <TuitionPanel
          cycle={cycle}
          levels={levels}
          previousCycle={cycles.previous}
          pricingLevelId={pricing.levelId}
          onPrice={pricing.setLevelId}
          onPlansLoaded={onPlansLoaded}
        />
      )}
    </StepFrame>
  );
}
