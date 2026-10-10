"use client";

import { createContext, useContext, useMemo, type ReactElement, type ReactNode } from "react";

import type { BtnKind } from "../../_design/Controls";

/**
 * How the catalog panels present themselves (PLAN-redesign-configuracion §5).
 *
 * The setup view wraps its steps in the provider; the old view has none, so the
 * default IS today's look. Panels read it for presentation only — no card around
 * a panel inside a step, which of their buttons stays lime (§1.3), and the
 * shorter labels of §1.8. Nothing here changes what a panel does or calls.
 */
export interface SetupPresentation {
  /** True inside a setup step. */
  inSetup: boolean;
  /** A setup step on a mid-size screen (§1.6): the months grid would not fit, so it opens as a list. */
  compact: boolean;
  /**
   * Set when the step frame already titles the step with its own h2, above the
   * chips: a panel's title then becomes a sub-heading, and is dropped when it
   * only repeats the step's name.
   */
  stepTitle?: string;
}

const OLD_VIEW: SetupPresentation = { inSetup: false, compact: false };

const SetupPresentationContext = createContext<SetupPresentation>(OLD_VIEW);

export function SetupPresentationProvider({ compact = false, children }: { compact?: boolean; children: ReactNode }): ReactElement {
  const value = useMemo(() => ({ inSetup: true, compact }), [compact]);
  return <SetupPresentationContext.Provider value={value}>{children}</SetupPresentationContext.Provider>;
}

/** Panels inside a step whose frame draws the h2 (§5 StepFrame): their titles drop a level. */
export function UnderStepTitle({ title, children }: { title: string; children: ReactNode }): ReactElement {
  const outer = useContext(SetupPresentationContext);
  const value = useMemo(() => ({ ...outer, stepTitle: title }), [outer, title]);
  return <SetupPresentationContext.Provider value={value}>{children}</SetupPresentationContext.Provider>;
}

export function useSetupPresentation(): SetupPresentation {
  return useContext(SetupPresentationContext);
}

/**
 * The kind of a panel action that is lime today but secondary inside a step,
 * where the step keeps a single lime (§1.3).
 */
export function useSecondaryInSetup(): BtnKind {
  return useSetupPresentation().inSetup ? "outline" : "primary";
}
