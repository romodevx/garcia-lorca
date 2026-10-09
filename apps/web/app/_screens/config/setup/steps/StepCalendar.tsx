"use client";

import type { ReactElement } from "react";

import type { Cycle } from "../../../../config/api";
import { CalendarPanel } from "../../CalendarPanel";
import { StepFrame, type StepSlot } from "../StepFrame";
import { Prerequisite } from "./Prerequisite";

export interface StepCalendarProps {
  slot: StepSlot;
  cycle: Cycle | undefined;
  onLoaded: (cycleId: string, closedDays: number | "failed") => void;
}

/** Step 8 · Calendario: optional, never blocks. Its count comes from the panel's own read. */
export function StepCalendar({ slot, cycle, onLoaded }: StepCalendarProps): ReactElement {
  return (
    <StepFrame {...slot} ownHeading={cycle === undefined}>
      {cycle === undefined ? <Prerequisite need="cycle" onGo={slot.onGo} /> : <CalendarPanel cycle={cycle} onLoaded={onLoaded} />}
    </StepFrame>
  );
}
