"use client";

import { useCallback, useState } from "react";

import type { OpenTarget } from "./SetupStepList";
import { STEPS, type StepDef, type StepId } from "./setupSteps";

/** sessionStorage key of the step this session last had open (§1.4). */
export const OPEN_STEP_KEY = "config-setup-step";

const isStepId = (steps: readonly StepDef[], value: string | null): value is StepId => steps.some((s) => s.id === value);

/**
 * The step this session last had open, if any, in `steps`. A stored «Grupos»
 * opens the merged «Estructura de la escuela» (module_school_structure).
 * Storage may be missing or refuse: then there is none.
 */
export function readStoredStep(steps: readonly StepDef[] = STEPS): StepId | undefined {
  try {
    const stored = window.sessionStorage.getItem(OPEN_STEP_KEY);
    if (stored === "groups" && !isStepId(steps, stored)) return "levels";
    return isStepId(steps, stored) ? stored : undefined;
  } catch {
    return undefined;
  }
}

function storeStep(id: OpenTarget): void {
  if (id === "trash") return;
  try {
    window.sessionStorage.setItem(OPEN_STEP_KEY, id);
  } catch {
    // Private mode or a full quota: the step just is not remembered.
  }
}

/** What the live region says when a step opens (§1.7). */
function announcementFor(steps: readonly StepDef[], id: OpenTarget): string {
  const step = steps.find((s) => s.id === id);
  return step === undefined ? "Papelera" : `Paso ${step.number} de ${steps.length}, ${step.title}`;
}

export interface OpenStep {
  openId: OpenTarget | undefined;
  open: (id: OpenTarget) => void;
  /** Bumped on every user-opened step: the view moves focus to its h2 then. */
  focusRequest: number;
  announcement: string;
}

/**
 * Which step is open. A step this session already opened (`storedStepId`, read
 * once by the caller with `readStoredStep`) reopens at once;
 * otherwise the step is decided ONCE, when the status reads have settled, and
 * never moves on its own after that or after the user picked one (§1.4).
 */
export function useOpenStep(initialStepId: StepId, settled: boolean, storedStepId: StepId | undefined, steps: readonly StepDef[] = STEPS): OpenStep {
  const [openId, setOpenId] = useState<OpenTarget | undefined>(storedStepId);
  const [focusRequest, setFocusRequest] = useState(0);
  const [announcement, setAnnouncement] = useState("");

  // Decided while rendering (React's "adjusting state" pattern), not in an
  // effect: the list never paints a frame with the status known and no step open.
  // Not stored: until the user picks one, the next visit recomputes it.
  if (openId === undefined && settled) setOpenId(initialStepId);

  const open = useCallback((wanted: OpenTarget): void => {
    // «Grupos» lives inside «Estructura de la escuela» when the list merges them.
    const id = wanted === "groups" && !isStepId(steps, wanted) ? "levels" : wanted;
    setOpenId(id);
    storeStep(id);
    setFocusRequest((n) => n + 1);
    setAnnouncement(announcementFor(steps, id));
  }, [steps]);

  return { openId, open, focusRequest, announcement };
}
