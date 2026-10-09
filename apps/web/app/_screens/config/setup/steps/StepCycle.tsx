"use client";

import type { ReactElement } from "react";

import { CycleBar } from "../../CycleBar";
import type { CyclesSlice } from "../useCatalogState";
import type { SetupActions } from "../useSetupActions";
import { StepFrame, type StepSlot } from "../StepFrame";

/** Step 2 · Ciclo escolar: choosing a cycle re-targets the whole screen, as today (A7). */
export function StepCycle({ slot, cycles, actions }: { slot: StepSlot; cycles: CyclesSlice; actions: SetupActions }): ReactElement {
  return (
    <StepFrame {...slot}>
      <CycleBar cycles={cycles.list} selectedCycleId={cycles.selectedId} onSelect={cycles.select} onCreate={actions.createCycle} onActivate={actions.activateCycle} />
    </StepFrame>
  );
}
