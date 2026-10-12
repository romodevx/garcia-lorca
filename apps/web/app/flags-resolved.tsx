"use client";

import { createContext, useContext, type ReactElement, type ReactNode } from "react";

/**
 * Whether /api/v1/features has answered. Before that, the flags are the OFF
 * fallback, and a reader that must agree with the pre-paint look (the home
 * variant, the skin) falls back to the remembered look instead (review I1).
 *
 * Defaults to true: outside AppFlagsProvider (tests, the design demo) the
 * flags given are the answer.
 */
const FlagsResolvedContext = createContext(true);

export function FlagsResolvedProvider({ resolved, children }: { resolved: boolean; children: ReactNode }): ReactElement {
  return <FlagsResolvedContext.Provider value={resolved}>{children}</FlagsResolvedContext.Provider>;
}

export function useFlagsResolved(): boolean {
  return useContext(FlagsResolvedContext);
}
