"use client";

import { useCallback, useState } from "react";

import type { OpenTarget } from "./SetupStepList";
import { STEPS, type StepId } from "./setupSteps";

/** sessionStorage key of the step this session last had open (§1.4). */
export const OPEN_STEP_KEY = "config-setup-step";

const isStepId = (value: string | null): value is StepId => STEPS.some((s) => s.id === value);

/** The step this session last had open, if any. Storage may be missing or refuse: then there is none. */
export function readStoredStep(): StepId | undefined {
  try {
    const stored = window.sessionStorage.getItem(OPEN_STEP_KEY);
    return isStepId(stored) ? stored : undefined;
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
function announcementFor(id: OpenTarget): string {
  const step = STEPS.find((s) => s.id === id);
  return step === undefined ? "Papelera" : `Paso ${step.number} de ${STEPS.length}, ${step.title}`;
}

export interface OpenStep {
  openId: OpenTarget | undefined;
  open: (id: OpenTarget) => void;
  /** Bumped on every user-opened step: the view moves focus to its h2 then. */
  focusRequest: number;
  announcement: string;
}

/**
 * Which step is open. A step this session already opened reopens at once;
 * otherwise the step is decided ONCE, when the status reads have settled, and
 * never moves on its own after that or after the user picked one (§1.4).
 */
export function useOpenStep(initialStepId: StepId, settled: boolean): OpenStep {
  const [openId, setOpenId] = useState<OpenTarget | undefined>(readStoredStep);
  const [focusRequest, setFocusRequest] = useState(0);
  const [announcement, setAnnouncement] = useState("");

  // Decided while rendering (React's "adjusting state" pattern), not in an
  // effect: the list never paints a frame with the status known and no step open.
  // Not stored: until the user picks one, the next visit recomputes it.
  if (openId === undefined && settled) setOpenId(initialStepId);

  const open = useCallback((id: OpenTarget): void => {
    setOpenId(id);
    storeStep(id);
    setFocusRequest((n) => n + 1);
    setAnnouncement(announcementFor(id));
  }, []);

  return { openId, open, focusRequest, announcement };
}
