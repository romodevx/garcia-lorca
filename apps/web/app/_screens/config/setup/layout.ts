"use client";

import { useCallback, useSyncExternalStore } from "react";

/*
 * The setup view's breakpoints (PLAN-redesign-configuracion §1.6). The CSS in
 * globals.css (.setup-*) uses the same numbers; keep them in step.
 */

/** At or below this width the step list folds into the "Paso N de 8" bar. */
export const NARROW_MAX_PX = 899;

/** At or below this width the list narrows and the months grid of step 4 opens as "Lista". */
export const COMPACT_MAX_PX = 1199;

const hasMatchMedia = (): boolean => typeof window !== "undefined" && typeof window.matchMedia === "function";

/**
 * Whether the viewport is at most `px` wide. False where matchMedia does not
 * exist (the server, a test renderer): a layout question must never throw, and
 * the wide layout is the safe answer.
 */
export function useMaxWidth(px: number): boolean {
  const query = `(max-width: ${px}px)`;
  const subscribe = useCallback(
    (onChange: () => void): (() => void) => {
      if (!hasMatchMedia()) return () => {};
      const media = window.matchMedia(query);
      media.addEventListener("change", onChange);
      return () => media.removeEventListener("change", onChange);
    },
    [query],
  );
  return useSyncExternalStore(subscribe, () => hasMatchMedia() && window.matchMedia(query).matches, () => false);
}
