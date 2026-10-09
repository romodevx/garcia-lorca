"use client";

import type { ReactElement } from "react";

import type { SchoolProfile } from "../../../../config/api";
import { SchoolProfilePanel } from "../../SchoolProfilePanel";
import { StepFrame, type StepSlot } from "../StepFrame";

/** Step 1 · Datos del colegio: the panel as it is, reporting its profile to the step list. */
export function StepProfile({ slot, onLoaded }: { slot: StepSlot; onLoaded: (profile: SchoolProfile | "failed") => void }): ReactElement {
  return (
    <StepFrame {...slot}>
      <SchoolProfilePanel onLoaded={onLoaded} />
    </StepFrame>
  );
}
